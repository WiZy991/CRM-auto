package httpx

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/security"
	"github.com/autoimport/crm/internal/service"
)

// refreshCookieName — cookie с refresh-токеном.
//
// Refresh лежит именно в cookie с флагом HttpOnly, а не в localStorage:
// при XSS скрипт не может прочитать HttpOnly-cookie, тогда как содержимое
// localStorage выгружается одной строкой. Access-токен, наоборот, хранится
// в памяти вкладки и не сохраняется на диск.
const refreshCookieName = "ai_refresh"

// refreshCookiePath ограничивает область действия cookie: она уходит только
// на эндпоинты обновления и выхода, а не на каждый запрос к API.
const refreshCookiePath = "/api/v1/auth"

// AuthHandler — обработчики аутентификации.
type AuthHandler struct {
	auth          *service.Auth
	secureCookies bool
}

func NewAuthHandler(auth *service.Auth, secureCookies bool) *AuthHandler {
	return &AuthHandler{auth: auth, secureCookies: secureCookies}
}

// --- Запросы и ответы -------------------------------------------------------

type registerRequest struct {
	Role     string `json:"role"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type verifyRequest struct {
	Channel string `json:"channel"`
	Code    string `json:"code"`
}

type resendRequest struct {
	Channel string `json:"channel"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type updateProfileRequest struct {
	FullName string  `json:"full_name"`
	Passport *string `json:"passport"`
	Address  *string `json:"address"`
}

type deleteAccountRequest struct {
	Password string `json:"password"`
}

// userResponse — публичное представление пользователя.
//
// Отдельный тип вместо domain.User: так в ответ физически не может попасть
// хеш пароля, счётчик неудачных входов или адрес последнего входа.
type userResponse struct {
	ID            uuid.UUID `json:"id"`
	Role          string    `json:"role"`
	RoleTitle     string    `json:"role_title"`
	Status        string    `json:"status"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	FullName      string    `json:"full_name"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
	EmailVerified bool      `json:"email_verified"`
	PhoneVerified bool      `json:"phone_verified"`
	Passport      string    `json:"passport,omitempty"`
	Address       string    `json:"address,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:            u.ID,
		Role:          u.Role.String(),
		RoleTitle:     u.Role.Title(),
		Status:        string(u.Status),
		Email:         u.Email,
		Phone:         u.Phone,
		FullName:      u.FullName,
		AvatarURL:     u.AvatarURL,
		EmailVerified: u.EmailVerified(),
		PhoneVerified: u.PhoneVerified(),
		CreatedAt:     u.CreatedAt,
	}
}

type authResponse struct {
	User            userResponse `json:"user"`
	AccessToken     string       `json:"access_token"`
	AccessExpiresAt time.Time    `json:"access_expires_at"`
	CSRFToken       string       `json:"csrf_token"`
}

// --- Обработчики ------------------------------------------------------------

// Register — POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	role, err := domain.ParseRole(req.Role)
	if err != nil {
		Error(w, r, apierr.Validation(map[string]string{
			"role": "допустимые значения: client, dealer, seller",
		}))
		return
	}

	result, err := h.auth.Register(r.Context(), service.RegisterInput{
		Role:     role,
		Email:    req.Email,
		Phone:    req.Phone,
		Password: req.Password,
		FullName: req.FullName,
	}, h.meta(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	csrfToken := h.setSessionCookies(w, result.Tokens)
	JSON(w, http.StatusCreated, authResponse{
		User:            toUserResponse(result.User),
		AccessToken:     result.Tokens.AccessToken,
		AccessExpiresAt: result.Tokens.AccessExpiresAt,
		CSRFToken:       csrfToken,
	})
}

// Login — POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	result, err := h.auth.Login(r.Context(), service.LoginInput{
		Login:    req.Login,
		Password: req.Password,
	}, h.meta(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	csrfToken := h.setSessionCookies(w, result.Tokens)
	JSON(w, http.StatusOK, authResponse{
		User:            toUserResponse(result.User),
		AccessToken:     result.Tokens.AccessToken,
		AccessExpiresAt: result.Tokens.AccessExpiresAt,
		CSRFToken:       csrfToken,
	})
}

// LoginPrecheck — GET /api/v1/auth/precheck?login=...
//
// Сообщает интерфейсу, нужно ли показывать капчу. Признак существования
// аккаунта не раскрывается: для неизвестного логина ответ такой же, как
// для существующего без неудачных попыток.
func (h *AuthHandler) LoginPrecheck(w http.ResponseWriter, r *http.Request) {
	login := strings.TrimSpace(r.URL.Query().Get("login"))
	if login == "" || len(login) > 200 {
		JSON(w, http.StatusOK, map[string]bool{"captcha_required": false})
		return
	}

	JSON(w, http.StatusOK, map[string]bool{
		"captcha_required": h.auth.CaptchaRequired(r.Context(), login),
	})
}

// Refresh — POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || cookie.Value == "" {
		Error(w, r, apierr.Unauthorized("Сессия не найдена"))
		return
	}

	tokens, err := h.auth.Refresh(r.Context(), cookie.Value, h.meta(r))
	if err != nil {
		// Любая неудача обновления означает, что сессия больше не годится:
		// cookie снимается, чтобы клиент не пытался бесконечно.
		h.clearSessionCookies(w)
		Error(w, r, err)
		return
	}

	user, err := h.auth.CurrentUser(r.Context(), tokens.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}

	csrfToken := h.setSessionCookies(w, *tokens)
	JSON(w, http.StatusOK, authResponse{
		User:            toUserResponse(user),
		AccessToken:     tokens.AccessToken,
		AccessExpiresAt: tokens.AccessExpiresAt,
		CSRFToken:       csrfToken,
	})
}

// Logout — POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	refreshToken := ""
	if cookie, err := r.Cookie(refreshCookieName); err == nil {
		refreshToken = cookie.Value
	}

	actor := ActorFrom(r.Context())
	if err := h.auth.Logout(r.Context(), refreshToken, actor.UserID, h.meta(r)); err != nil {
		Error(w, r, err)
		return
	}

	h.clearSessionCookies(w)
	NoContent(w)
}

// LogoutAll — POST /api/v1/auth/logout-all
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	actor := ActorFrom(r.Context())
	if err := h.auth.LogoutEverywhere(r.Context(), actor.UserID, h.meta(r)); err != nil {
		Error(w, r, err)
		return
	}

	h.clearSessionCookies(w)
	NoContent(w)
}

// Me — GET /api/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	actor := ActorFrom(r.Context())

	user, err := h.auth.CurrentUser(r.Context(), actor.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}
	resp := toUserResponse(user)
	secrets, err := h.auth.ProfileSecrets(r.Context(), actor.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}
	if secrets != nil {
		resp.Passport = secrets.Passport
		resp.Address = secrets.Address
	}
	JSON(w, http.StatusOK, map[string]any{"user": resp})
}

// UpdateProfile — PATCH /api/v1/auth/profile
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	var req updateProfileRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	user, err := h.auth.UpdateProfile(r.Context(), actor.UserID, service.UpdateProfileInput{
		FullName: req.FullName,
		Passport: req.Passport,
		Address:  req.Address,
	}, h.meta(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	resp := toUserResponse(user)
	secrets, err := h.auth.ProfileSecrets(r.Context(), actor.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}
	if secrets != nil {
		resp.Passport = secrets.Passport
		resp.Address = secrets.Address
	}
	JSON(w, http.StatusOK, map[string]any{"user": resp})
}

// DeleteAccount — POST /api/v1/auth/delete
func (h *AuthHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	var req deleteAccountRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	if err := h.auth.DeleteAccount(r.Context(), actor.UserID, req.Password, h.meta(r)); err != nil {
		Error(w, r, err)
		return
	}

	h.clearSessionCookies(w)
	NoContent(w)
}

// SendCode — POST /api/v1/auth/verify/resend
func (h *AuthHandler) SendCode(w http.ResponseWriter, r *http.Request) {
	var req resendRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	channel := domain.VerifyChannel(strings.ToLower(strings.TrimSpace(req.Channel)))

	if err := h.auth.SendVerificationCode(r.Context(), actor.UserID, channel, h.meta(r)); err != nil {
		Error(w, r, err)
		return
	}
	JSON(w, http.StatusAccepted, map[string]string{
		"status": "sent", "channel": string(channel),
	})
}

// Verify — POST /api/v1/auth/verify
func (h *AuthHandler) Verify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	channel := domain.VerifyChannel(strings.ToLower(strings.TrimSpace(req.Channel)))

	user, err := h.auth.ConfirmVerificationCode(r.Context(), actor.UserID, channel, req.Code, h.meta(r))
	if err != nil {
		Error(w, r, err)
		return
	}

	// Новый access сразу с обновлёнными ev/pv — без отдельного /refresh,
	// который при сбое гасил сессию на клиенте сразу после успешного confirm.
	tokens, err := h.auth.ReissueAccess(r.Context(), actor.UserID, actor.SessionID)
	if err != nil {
		Error(w, r, err)
		return
	}

	csrfToken := ""
	if cookie, err := r.Cookie(CSRFCookieName); err == nil {
		csrfToken = cookie.Value
	}

	JSON(w, http.StatusOK, authResponse{
		User:            toUserResponse(user),
		AccessToken:     tokens.AccessToken,
		AccessExpiresAt: tokens.AccessExpiresAt,
		CSRFToken:       csrfToken,
	})
}

// Sessions — GET /api/v1/auth/sessions
func (h *AuthHandler) Sessions(w http.ResponseWriter, r *http.Request) {
	actor := ActorFrom(r.Context())

	sessions, err := h.auth.ActiveSessions(r.Context(), actor.UserID)
	if err != nil {
		Error(w, r, err)
		return
	}

	type sessionResponse struct {
		ID         uuid.UUID  `json:"id"`
		Device     string     `json:"device"`
		IP         string     `json:"ip,omitempty"`
		Current    bool       `json:"current"`
		CreatedAt  time.Time  `json:"created_at"`
		LastUsedAt *time.Time `json:"last_used_at,omitempty"`
		ExpiresAt  time.Time  `json:"expires_at"`
	}

	items := make([]sessionResponse, 0, len(sessions))
	for _, sess := range sessions {
		ip := ""
		if sess.IP != nil {
			ip = *sess.IP
		}
		items = append(items, sessionResponse{
			ID:         sess.ID,
			Device:     sess.DeviceLabel,
			IP:         ip,
			Current:    sess.ID == actor.SessionID,
			CreatedAt:  sess.CreatedAt,
			LastUsedAt: sess.LastUsedAt,
			ExpiresAt:  sess.ExpiresAt,
		})
	}
	JSON(w, http.StatusOK, map[string]any{"items": items})
}

// RevokeSession — DELETE /api/v1/auth/sessions/{id}
func (h *AuthHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chiURLParam(r, "id"))
	if err != nil {
		Error(w, r, apierr.BadRequest("Некорректный идентификатор сессии"))
		return
	}

	actor := ActorFrom(r.Context())
	if err := h.auth.RevokeSession(r.Context(), actor.UserID, sessionID); err != nil {
		Error(w, r, err)
		return
	}
	NoContent(w)
}

// ChangePassword — POST /api/v1/auth/password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		Error(w, r, err)
		return
	}

	actor := ActorFrom(r.Context())
	if err := h.auth.ChangePassword(r.Context(), actor.UserID, req.OldPassword, req.NewPassword, h.meta(r)); err != nil {
		Error(w, r, err)
		return
	}

	h.clearSessionCookies(w)
	JSON(w, http.StatusOK, map[string]string{
		"status": "password_changed",
		"note":   "Все сессии завершены, войдите заново",
	})
}

// --- Работа с cookie --------------------------------------------------------

// setSessionCookies выставляет refresh-токен и токен защиты от подделки
// запроса. Возвращает значение CSRF-токена для клиента.
func (h *AuthHandler) setSessionCookies(w http.ResponseWriter, tokens service.TokenPair) string {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    tokens.RefreshToken,
		Path:     refreshCookiePath,
		Expires:  tokens.RefreshExpires,
		MaxAge:   int(time.Until(tokens.RefreshExpires).Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookies,
		// Strict, а не Lax: cookie нужна только на собственных запросах
		// обновления сессии, переходов по внешним ссылкам с ней не бывает.
		SameSite: http.SameSiteStrictMode,
	})

	csrfToken, err := security.RandomToken(24)
	if err != nil {
		// Провал генератора случайных чисел означает серьёзную проблему
		// среды; сессия при этом остаётся рабочей, но без CSRF-токена
		// небезопасные запросы будут отклонены.
		return ""
	}

	http.SetCookie(w, &http.Cookie{
		Name:  CSRFCookieName,
		Value: csrfToken,
		Path:  "/",
		// HttpOnly не ставится намеренно: схема double submit требует, чтобы
		// собственный скрипт мог прочитать значение и продублировать его в
		// заголовке. Чужому сайту это недоступно из-за политики одного
		// источника.
		HttpOnly: false,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
		Expires:  tokens.RefreshExpires,
		MaxAge:   int(time.Until(tokens.RefreshExpires).Seconds()),
	})

	return csrfToken
}

func (h *AuthHandler) clearSessionCookies(w http.ResponseWriter) {
	for _, cookie := range []struct{ name, path string }{
		{refreshCookieName, refreshCookiePath},
		{CSRFCookieName, "/"},
	} {
		http.SetCookie(w, &http.Cookie{
			Name:     cookie.name,
			Value:    "",
			Path:     cookie.path,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			HttpOnly: cookie.name == refreshCookieName,
			Secure:   h.secureCookies,
			SameSite: http.SameSiteStrictMode,
		})
	}
}

func (h *AuthHandler) meta(r *http.Request) service.RequestMeta {
	return requestMeta(r)
}

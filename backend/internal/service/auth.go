// Package service содержит сценарии приложения.
//
// Слой между транспортом и хранилищем. Здесь принимаются решения («можно ли
// этому пользователю сменить этап сделки»), а транспорт занимается только
// разбором запроса, а хранилище — только SQL.
package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/config"
	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/notify"
	"github.com/autoimport/crm/internal/pkg/security"
	"github.com/autoimport/crm/internal/store"
)

// Auth реализует регистрацию, вход, обновление сессии и подтверждение
// контактов.
type Auth struct {
	users        *store.Users
	sessions     *store.Sessions
	verification *store.Verification
	securityLog  *store.SecurityLog

	hasher  *security.Hasher
	tokens  *security.TokenIssuer
	revoker *SessionRevoker

	mailer notify.Mailer
	sms    notify.SMSSender

	cfg config.Auth
	log *slog.Logger

	appName   string
	publicURL string

	cipher  *security.Cipher
	dealers *store.Dealers
	sellers *store.Sellers

	// skipVerify — development: регистрация сразу active, коды не шлём.
	skipVerify bool
}

// AuthDeps — зависимости сервиса аутентификации.
type AuthDeps struct {
	Users        *store.Users
	Sessions     *store.Sessions
	Verification *store.Verification
	SecurityLog  *store.SecurityLog
	Hasher       *security.Hasher
	Tokens       *security.TokenIssuer
	Revoker      *SessionRevoker
	Mailer       notify.Mailer
	SMS          notify.SMSSender
	Config       config.Auth
	Log          *slog.Logger
	AppName      string
	PublicURL    string
	Cipher       *security.Cipher
	Dealers      *store.Dealers
	Sellers      *store.Sellers
	SkipVerify   bool
}

func NewAuth(deps AuthDeps) *Auth {
	return &Auth{
		users:        deps.Users,
		sessions:     deps.Sessions,
		verification: deps.Verification,
		securityLog:  deps.SecurityLog,
		hasher:       deps.Hasher,
		tokens:       deps.Tokens,
		revoker:      deps.Revoker,
		mailer:       deps.Mailer,
		sms:          deps.SMS,
		cfg:          deps.Config,
		log:          deps.Log,
		appName:      deps.AppName,
		publicURL:    deps.PublicURL,
		cipher:       deps.Cipher,
		dealers:      deps.Dealers,
		sellers:      deps.Sellers,
		skipVerify:   deps.SkipVerify,
	}
}

// RequestMeta — сведения о запросе для журналов и сессий.
type RequestMeta struct {
	IP        string
	UserAgent string
	RequestID string
}

// TokenPair — выданные токены.
type TokenPair struct {
	AccessToken     string
	AccessExpiresAt time.Time
	RefreshToken    string
	RefreshExpires  time.Time
	SessionID       uuid.UUID
	UserID          uuid.UUID
}

// --- Регистрация ------------------------------------------------------------

// RegisterInput — данные регистрации.
type RegisterInput struct {
	Role     domain.Role
	Email    string
	Phone    string
	Password string
	FullName string
}

// RegisterResult — результат регистрации.
type RegisterResult struct {
	User   *domain.User
	Tokens TokenPair
}

// Register создаёт учётную запись и отправляет коды подтверждения.
//
// Пользователь получает токены сразу, но со статусом pending: он может
// зайти в кабинет и подтвердить контакты, однако действия, порождающие
// обязательства (заявка, объявление, переписка), закрыты до подтверждения.
// Это компромисс между безопасностью и тем, чтобы не терять человека на
// пустом экране «проверьте почту».
func (a *Auth) Register(ctx context.Context, input RegisterInput, meta RequestMeta) (*RegisterResult, error) {
	if !input.Role.Registrable() {
		return nil, apierr.BadRequest("Регистрация с этой ролью недоступна")
	}

	email := normalizeEmail(input.Email)
	phone, err := normalizePhone(input.Phone)
	if err != nil {
		return nil, apierr.Validation(map[string]string{"phone": err.Error()})
	}

	if err := security.ValidatePassword(input.Password); err != nil {
		return nil, apierr.Validation(map[string]string{"password": err.Error()})
	}

	fullName := strings.TrimSpace(input.FullName)
	if len([]rune(fullName)) < 2 {
		return nil, apierr.Validation(map[string]string{"full_name": "укажите имя не короче двух символов"})
	}

	hash, err := a.hasher.Hash(input.Password)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	record, err := a.users.Create(ctx, store.CreateUserParams{
		Role:            input.Role,
		Email:           email,
		Phone:           phone,
		PasswordHash:    hash,
		FullName:        fullName,
		ImmediateActive: a.skipVerify,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrEmailTaken):
			return nil, apierr.Conflict("Этот адрес электронной почты уже зарегистрирован")
		case errors.Is(err, store.ErrPhoneTaken):
			return nil, apierr.Conflict("Этот номер телефона уже зарегистрирован")
		default:
			return nil, apierr.Internal(err)
		}
	}

	a.recordSecurity(ctx, store.SecurityEvent{
		Kind:      store.EventRegistration,
		UserID:    &record.ID,
		IP:        meta.IP,
		UserAgent: meta.UserAgent,
		Details:   map[string]any{"role": record.Role.String()},
	})

	if !a.skipVerify {
		// Коды отправляются после создания записи и вне транзакции: ошибка
		// отправки письма не должна откатывать регистрацию, пользователь
		// сможет запросить код повторно.
		if err := a.sendCode(ctx, record, domain.ChannelEmail, meta); err != nil {
			a.log.ErrorContext(ctx, "не удалось отправить код на email",
				slog.String("error", err.Error()))
		}
		if err := a.sendCode(ctx, record, domain.ChannelPhone, meta); err != nil {
			a.log.ErrorContext(ctx, "не удалось отправить код по SMS",
				slog.String("error", err.Error()))
		}
	}

	tokens, err := a.issueTokens(ctx, record, meta)
	if err != nil {
		return nil, err
	}

	if record.Role == domain.RoleDealer && a.dealers != nil {
		if err := a.dealers.EnsureDefault(ctx, record.ID, record.FullName); err != nil {
			a.log.ErrorContext(ctx, "не удалось создать профиль дилера",
				slog.String("error", err.Error()))
		}
	}
	if record.Role == domain.RoleSeller && a.sellers != nil {
		if err := a.sellers.EnsureDefault(ctx, record.ID, record.FullName); err != nil {
			a.log.ErrorContext(ctx, "не удалось создать карточку продавца",
				slog.String("error", err.Error()))
		}
	}

	user := record.User
	return &RegisterResult{User: &user, Tokens: *tokens}, nil
}

// --- Вход -------------------------------------------------------------------

// LoginInput — данные входа.
type LoginInput struct {
	Login    string // email или телефон
	Password string
}

// LoginResult — результат входа.
type LoginResult struct {
	User   *domain.User
	Tokens TokenPair
}

// Login проверяет пароль и выдаёт токены.
//
// Порядок проверок важен для безопасности:
//
//  1. пользователь ищется по email или телефону;
//  2. если его нет, пароль всё равно «проверяется» против фиктивного хеша,
//     чтобы время ответа не отличалось и нельзя было перебором выяснить,
//     какие адреса зарегистрированы;
//  3. блокировка аккаунта проверяется до сравнения пароля;
//  4. при неудаче счётчик растёт и включается экспоненциальная задержка.
func (a *Auth) Login(ctx context.Context, input LoginInput, meta RequestMeta) (*LoginResult, error) {
	login := strings.TrimSpace(input.Login)
	if login == "" || input.Password == "" {
		return nil, apierr.InvalidCredentials()
	}

	record, lookupErr := a.findByLogin(ctx, login)
	if lookupErr != nil && !errors.Is(lookupErr, store.ErrNotFound) {
		return nil, apierr.Internal(lookupErr)
	}

	if record == nil {
		// Сравнение с заранее посчитанным хешем: аккаунта нет, но время
		// ответа сопоставимо с обычной проверкой пароля.
		_, _ = a.hasher.Verify(input.Password, dummyPasswordHash)
		a.recordSecurity(ctx, store.SecurityEvent{
			Kind:      store.EventLoginFailed,
			Severity:  2,
			IP:        meta.IP,
			UserAgent: meta.UserAgent,
			Details:   map[string]any{"reason": "unknown_login"},
		})
		return nil, apierr.InvalidCredentials()
	}

	now := time.Now()
	if record.IsLocked(now) {
		wait := record.LockedUntil.Sub(now).Round(time.Second)
		return nil, apierr.AccountLocked(fmt.Sprintf(
			"Вход заблокирован из-за неудачных попыток. Повторите через %s", humanWait(wait)))
	}

	if !record.Status.CanSignIn() {
		return nil, apierr.Forbidden("Доступ к учётной записи приостановлен")
	}

	ok, err := a.hasher.Verify(input.Password, record.PasswordHash)
	if err != nil {
		return nil, apierr.Internal(fmt.Errorf("проверка пароля: %w", err))
	}

	if !ok {
		a.handleFailedLogin(ctx, record, meta)
		return nil, apierr.InvalidCredentials()
	}

	if err := a.users.RegisterSuccessfulLogin(ctx, record.ID, meta.IP); err != nil {
		a.log.ErrorContext(ctx, "не удалось обновить данные о входе", slog.String("error", err.Error()))
	}

	if a.skipVerify && record.Status == domain.StatusPending {
		activated, actErr := a.users.ActivatePending(ctx, record.ID)
		if actErr != nil {
			a.log.ErrorContext(ctx, "не удалось активировать pending-учётку",
				slog.String("error", actErr.Error()))
		} else {
			record = activated
		}
		if record.Role == domain.RoleDealer && a.dealers != nil {
			if err := a.dealers.EnsureDefault(ctx, record.ID, record.FullName); err != nil {
				a.log.ErrorContext(ctx, "не удалось создать профиль дилера при входе",
					slog.String("error", err.Error()))
			}
		}
		if record.Role == domain.RoleSeller && a.sellers != nil {
			if err := a.sellers.EnsureDefault(ctx, record.ID, record.FullName); err != nil {
				a.log.ErrorContext(ctx, "не удалось создать карточку продавца при входе",
					slog.String("error", err.Error()))
			}
		}
	}

	// Если параметры argon2 усилены в конфиге, пароль перехешируется
	// прозрачно для пользователя — единственный момент, когда он известен.
	if a.hasher.NeedsRehash(record.PasswordHash) {
		if newHash, hashErr := a.hasher.Hash(input.Password); hashErr == nil {
			if updErr := a.users.UpdatePasswordHash(ctx, record.ID, newHash); updErr != nil {
				a.log.WarnContext(ctx, "не удалось обновить хеш пароля", slog.String("error", updErr.Error()))
			}
		}
	}

	a.recordSecurity(ctx, store.SecurityEvent{
		Kind:      store.EventLoginSuccess,
		UserID:    &record.ID,
		IP:        meta.IP,
		UserAgent: meta.UserAgent,
	})

	tokens, err := a.issueTokens(ctx, record, meta)
	if err != nil {
		return nil, err
	}

	user := record.User
	return &LoginResult{User: &user, Tokens: *tokens}, nil
}

// dummyPasswordHash — заранее посчитанный хеш для выравнивания времени
// ответа при несуществующем логине.
const dummyPasswordHash = "$argon2id$v=19$m=65536,t=3,p=2$" +
	"c29tZS1zYWx0LXZhbHVl$Zm9yLXRpbWluZy1lcXVhbGl6YXRpb24tb25seS0xMjM0"

func (a *Auth) handleFailedLogin(ctx context.Context, record *store.UserRecord, meta RequestMeta) {
	nextCount := record.FailedLoginCount + 1
	lockFor := domain.LockoutDuration(nextCount, a.cfg.LockoutBase, a.cfg.LockoutMax)

	if _, err := a.users.RegisterFailedLogin(ctx, record.ID, lockFor); err != nil {
		a.log.ErrorContext(ctx, "не удалось учесть неудачный вход", slog.String("error", err.Error()))
	}

	details := map[string]any{"failed_count": nextCount}
	kind := store.EventLoginFailed
	severity := 2

	if lockFor > 0 {
		kind = store.EventAccountLocked
		severity = 3
		details["locked_for_seconds"] = int(lockFor.Seconds())
	}

	a.recordSecurity(ctx, store.SecurityEvent{
		Kind:      kind,
		Severity:  severity,
		UserID:    &record.ID,
		IP:        meta.IP,
		UserAgent: meta.UserAgent,
		Details:   details,
	})
}

// CaptchaRequired сообщает, нужно ли требовать проверку «не робот».
//
// Порог берётся из числа неудачных попыток конкретного аккаунта: показывать
// капчу всем на первой же попытке — плохой опыт, не показывать никогда —
// приглашение к перебору.
func (a *Auth) CaptchaRequired(ctx context.Context, login string) bool {
	if a.cfg.CaptchaAfterFails <= 0 {
		return false
	}
	record, err := a.findByLogin(ctx, strings.TrimSpace(login))
	if err != nil || record == nil {
		return false
	}
	return record.FailedLoginCount >= a.cfg.CaptchaAfterFails
}

func (a *Auth) findByLogin(ctx context.Context, login string) (*store.UserRecord, error) {
	if strings.Contains(login, "@") {
		record, err := a.users.ByEmail(ctx, login)
		if errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		return record, err
	}

	record, err := a.users.ByPhone(ctx, login)
	if errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return record, err
}

// --- Обновление и завершение сессии -----------------------------------------

// Refresh обменивает refresh-токен на новую пару токенов.
func (a *Auth) Refresh(ctx context.Context, refreshToken string, meta RequestMeta) (*TokenPair, error) {
	if refreshToken == "" {
		return nil, apierr.Unauthorized("Отсутствует токен обновления")
	}

	newToken, err := security.RandomToken(32)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	expiresAt := time.Now().Add(a.cfg.RefreshTokenTTL)

	result, err := a.sessions.Rotate(ctx,
		security.HashToken(refreshToken), security.HashToken(newToken), expiresAt,
		meta.UserAgent, meta.IP, deviceLabel(meta.UserAgent))

	switch {
	case errors.Is(err, store.ErrSessionReused):
		// Токеном воспользовались дважды. Вся семья погашена в базе,
		// осталось закрыть доступ уже выпущенным access-токенам.
		a.recordSecurity(ctx, store.SecurityEvent{
			Kind:      store.EventTokenReuse,
			Severity:  4,
			IP:        meta.IP,
			UserAgent: meta.UserAgent,
			Details:   map[string]any{"action": "family_revoked"},
		})
		a.log.WarnContext(ctx, "обнаружено повторное использование refresh-токена")
		return nil, apierr.TokenReused()

	case errors.Is(err, store.ErrNotFound):
		return nil, apierr.Unauthorized("Сессия не найдена или истекла")

	case err != nil:
		return nil, apierr.Internal(err)
	}

	if err := a.revoker.Revoke(ctx, result.OldID); err != nil {
		a.log.WarnContext(ctx, "не удалось отозвать предыдущую сессию в кеше",
			slog.String("error", err.Error()))
	}

	record, err := a.users.ByID(ctx, result.Session.UserID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	if !record.Status.CanSignIn() {
		return nil, apierr.Forbidden("Доступ к учётной записи приостановлен")
	}

	access, accessExpires, err := a.tokens.IssueAccess(
		record.ID, result.Session.ID, record.Role.String(),
		record.EmailVerified(), record.PhoneVerified())
	if err != nil {
		return nil, apierr.Internal(err)
	}

	return &TokenPair{
		AccessToken:     access,
		AccessExpiresAt: accessExpires,
		RefreshToken:    newToken,
		RefreshExpires:  result.Session.ExpiresAt,
		SessionID:       result.Session.ID,
		UserID:          record.ID,
	}, nil
}

// Logout завершает текущую сессию.
func (a *Auth) Logout(ctx context.Context, refreshToken string, actorID uuid.UUID, meta RequestMeta) error {
	if refreshToken == "" {
		return nil
	}

	sessionID, err := a.sessions.RevokeByTokenHash(ctx, security.HashToken(refreshToken), "logout")
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return apierr.Internal(err)
	}

	if sessionID != uuid.Nil {
		if revokeErr := a.revoker.Revoke(ctx, sessionID); revokeErr != nil {
			a.log.WarnContext(ctx, "не удалось отозвать сессию в кеше",
				slog.String("error", revokeErr.Error()))
		}
	}

	var userID *uuid.UUID
	if actorID != uuid.Nil {
		userID = &actorID
	}
	a.recordSecurity(ctx, store.SecurityEvent{
		Kind: store.EventLogout, UserID: userID, IP: meta.IP, UserAgent: meta.UserAgent,
	})
	return nil
}

// LogoutEverywhere завершает все сессии пользователя.
func (a *Auth) LogoutEverywhere(ctx context.Context, userID uuid.UUID, meta RequestMeta) error {
	ids, err := a.sessions.RevokeAllForUser(ctx, userID, "logout_all")
	if err != nil {
		return apierr.Internal(err)
	}
	if err := a.revoker.RevokeMany(ctx, ids); err != nil {
		a.log.WarnContext(ctx, "не удалось отозвать сессии в кеше", slog.String("error", err.Error()))
	}

	a.recordSecurity(ctx, store.SecurityEvent{
		Kind: store.EventLogoutAll, UserID: &userID, IP: meta.IP, UserAgent: meta.UserAgent,
		Details: map[string]any{"sessions": len(ids)},
	})
	return nil
}

// ActiveSessions возвращает список активных устройств.
func (a *Auth) ActiveSessions(ctx context.Context, userID uuid.UUID) ([]store.Session, error) {
	sessions, err := a.sessions.ListActiveForUser(ctx, userID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return sessions, nil
}

// RevokeSession завершает конкретную сессию пользователя.
func (a *Auth) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	sessions, err := a.sessions.ListActiveForUser(ctx, userID)
	if err != nil {
		return apierr.Internal(err)
	}

	// Проверка владения: пользователь может завершать только свои сессии.
	found := false
	for _, sess := range sessions {
		if sess.ID == sessionID {
			found = true
			break
		}
	}
	if !found {
		return apierr.NotFound("Сессия")
	}

	if err := a.sessions.Revoke(ctx, sessionID, "revoked_by_user"); err != nil {
		return apierr.Internal(err)
	}
	if err := a.revoker.Revoke(ctx, sessionID); err != nil {
		a.log.WarnContext(ctx, "не удалось отозвать сессию в кеше", slog.String("error", err.Error()))
	}
	return nil
}

// --- Подтверждение контактов ------------------------------------------------

// SendVerificationCode отправляет код подтверждения.
func (a *Auth) SendVerificationCode(ctx context.Context, userID uuid.UUID, channel domain.VerifyChannel, meta RequestMeta) error {
	if !channel.Valid() {
		return apierr.BadRequest("Неизвестный канал подтверждения")
	}

	record, err := a.users.ByID(ctx, userID)
	if err != nil {
		return apierr.Internal(err)
	}

	if channel == domain.ChannelEmail && record.EmailVerified() {
		return apierr.Conflict("Адрес электронной почты уже подтверждён")
	}
	if channel == domain.ChannelPhone && record.PhoneVerified() {
		return apierr.Conflict("Номер телефона уже подтверждён")
	}

	// Пауза между отправками: без неё кнопка «отправить снова» превращается
	// в инструмент рассылки за счёт платформы.
	if issuedAt, exists, err := a.verification.ActiveCodeIssuedAt(ctx, userID, channel); err == nil && exists {
		const minInterval = 60 * time.Second
		if elapsed := time.Since(issuedAt); elapsed < minInterval {
			wait := (minInterval - elapsed).Round(time.Second)
			return apierr.TooManyRequests(fmt.Sprintf(
				"Повторная отправка возможна через %s", humanWait(wait)))
		}
	}

	return a.sendCode(ctx, record, channel, meta)
}

func (a *Auth) sendCode(ctx context.Context, record *store.UserRecord, channel domain.VerifyChannel, meta RequestMeta) error {
	code, err := security.NumericCode(6)
	if err != nil {
		return fmt.Errorf("генерация кода: %w", err)
	}

	destination := record.Email
	if channel == domain.ChannelPhone {
		destination = record.Phone
	}

	if err := a.verification.Issue(ctx, store.CreateCodeParams{
		UserID:      record.ID,
		Channel:     channel,
		Destination: destination,
		CodeHash:    hashVerificationCode(code, record.ID),
		ExpiresAt:   time.Now().Add(a.cfg.VerificationCodeTTL),
		MaxAttempts: a.cfg.VerificationMaxAttempts,
		CreatedIP:   meta.IP,
	}); err != nil {
		return fmt.Errorf("сохранение кода: %w", err)
	}

	minutes := int(a.cfg.VerificationCodeTTL.Minutes())

	if channel == domain.ChannelEmail {
		err = a.mailer.Send(ctx, notify.Message{
			To:      destination,
			Subject: fmt.Sprintf("%s: код подтверждения %s", a.appName, code),
			Text: fmt.Sprintf(
				"Здравствуйте, %s!\n\n"+
					"Код подтверждения адреса электронной почты: %s\n"+
					"Код действует %d минут.\n\n"+
					"Если вы не запрашивали код, просто проигнорируйте это письмо.\n\n"+
					"%s",
				record.FullName, code, minutes, a.publicURL),
		})
	} else {
		err = a.sms.Send(ctx, destination, fmt.Sprintf(
			"%s: код подтверждения %s. Действует %d мин.", a.appName, code, minutes))
	}
	if err != nil {
		return fmt.Errorf("отправка кода: %w", err)
	}

	a.recordSecurity(ctx, store.SecurityEvent{
		Kind:      store.EventVerifyCodeSent,
		UserID:    &record.ID,
		IP:        meta.IP,
		UserAgent: meta.UserAgent,
		Details:   map[string]any{"channel": string(channel)},
	})
	return nil
}

// ConfirmVerificationCode проверяет код и подтверждает контакт.
func (a *Auth) ConfirmVerificationCode(
	ctx context.Context,
	userID uuid.UUID,
	channel domain.VerifyChannel,
	code string,
	meta RequestMeta,
) (*domain.User, error) {
	if !channel.Valid() {
		return nil, apierr.BadRequest("Неизвестный канал подтверждения")
	}

	code = strings.TrimSpace(code)
	if len(code) < 4 || len(code) > 10 {
		return nil, apierr.Validation(map[string]string{"code": "код должен состоять из шести цифр"})
	}

	expected := hashVerificationCode(code, userID)
	attemptsLeft, err := a.verification.Consume(ctx, userID, channel, func(stored string) bool {
		return subtle.ConstantTimeCompare([]byte(stored), []byte(expected)) == 1
	})

	switch {
	case errors.Is(err, store.ErrCodeNotFound):
		return nil, apierr.NotFound("Активный код подтверждения")

	case errors.Is(err, store.ErrCodeExpired):
		return nil, apierr.BadRequest("Срок действия кода истёк, запросите новый")

	case errors.Is(err, store.ErrCodeAttempts):
		a.recordSecurity(ctx, store.SecurityEvent{
			Kind: store.EventVerifyCodeFailed, Severity: 3, UserID: &userID,
			IP: meta.IP, UserAgent: meta.UserAgent,
			Details: map[string]any{"reason": "attempts_exceeded", "channel": string(channel)},
		})
		return nil, apierr.TooManyRequests("Превышено число попыток. Запросите новый код")

	case errors.Is(err, store.ErrCodeMismatch):
		a.recordSecurity(ctx, store.SecurityEvent{
			Kind: store.EventVerifyCodeFailed, Severity: 2, UserID: &userID,
			IP: meta.IP, UserAgent: meta.UserAgent,
			Details: map[string]any{"channel": string(channel), "attempts_left": attemptsLeft},
		})
		return nil, apierr.Validation(map[string]string{
			"code": fmt.Sprintf("код неверен, осталось попыток: %d", max(attemptsLeft, 0)),
		})

	case err != nil:
		return nil, apierr.Internal(err)
	}

	record, err := a.users.MarkVerified(ctx, userID, channel)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	a.recordSecurity(ctx, store.SecurityEvent{
		Kind: store.EventVerifySuccess, UserID: &userID, IP: meta.IP, UserAgent: meta.UserAgent,
		Details: map[string]any{"channel": string(channel)},
	})

	user := record.User
	return &user, nil
}

// hashVerificationCode хеширует код вместе с идентификатором пользователя.
//
// Идентификатор в роли соли решает конкретную задачу: одинаковые коды у
// разных пользователей дают разные хеши, поэтому по совпадению хешей нельзя
// определить, что кому-то пришёл тот же код.
func hashVerificationCode(code string, userID uuid.UUID) string {
	sum := sha256.Sum256([]byte(code + ":" + userID.String()))
	return hex.EncodeToString(sum[:])
}

// --- Смена пароля -----------------------------------------------------------

// ChangePassword меняет пароль и завершает остальные сессии.
func (a *Auth) ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string, meta RequestMeta) error {
	record, err := a.users.ByID(ctx, userID)
	if err != nil {
		return apierr.Internal(err)
	}

	ok, err := a.hasher.Verify(oldPassword, record.PasswordHash)
	if err != nil {
		return apierr.Internal(err)
	}
	if !ok {
		return apierr.Validation(map[string]string{"old_password": "текущий пароль указан неверно"})
	}

	if err := security.ValidatePassword(newPassword); err != nil {
		return apierr.Validation(map[string]string{"new_password": err.Error()})
	}
	if oldPassword == newPassword {
		return apierr.Validation(map[string]string{"new_password": "новый пароль совпадает с текущим"})
	}

	hash, err := a.hasher.Hash(newPassword)
	if err != nil {
		return apierr.Internal(err)
	}
	if err := a.users.UpdatePasswordHash(ctx, userID, hash); err != nil {
		return apierr.Internal(err)
	}

	// Смена пароля завершает все сессии: если пароль меняют из-за
	// подозрения на взлом, чужая сессия должна перестать работать.
	ids, err := a.sessions.RevokeAllForUser(ctx, userID, "password_changed")
	if err != nil {
		a.log.ErrorContext(ctx, "не удалось отозвать сессии после смены пароля",
			slog.String("error", err.Error()))
	} else if err := a.revoker.RevokeMany(ctx, ids); err != nil {
		a.log.WarnContext(ctx, "не удалось отозвать сессии в кеше", slog.String("error", err.Error()))
	}

	a.recordSecurity(ctx, store.SecurityEvent{
		Kind: store.EventPasswordChanged, Severity: 2, UserID: &userID,
		IP: meta.IP, UserAgent: meta.UserAgent,
	})
	return nil
}

// --- Общие вспомогательные методы -------------------------------------------

func (a *Auth) issueTokens(ctx context.Context, record *store.UserRecord, meta RequestMeta) (*TokenPair, error) {
	refreshToken, err := security.RandomToken(32)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	session, err := a.sessions.Create(ctx, store.CreateSessionParams{
		UserID:      record.ID,
		TokenHash:   security.HashToken(refreshToken),
		UserAgent:   meta.UserAgent,
		IP:          meta.IP,
		DeviceLabel: deviceLabel(meta.UserAgent),
		ExpiresAt:   time.Now().Add(a.cfg.RefreshTokenTTL),
	})
	if err != nil {
		return nil, apierr.Internal(err)
	}

	access, accessExpires, err := a.tokens.IssueAccess(
		record.ID, session.ID, record.Role.String(),
		record.EmailVerified(), record.PhoneVerified())
	if err != nil {
		return nil, apierr.Internal(err)
	}

	return &TokenPair{
		AccessToken:     access,
		AccessExpiresAt: accessExpires,
		RefreshToken:    refreshToken,
		RefreshExpires:  session.ExpiresAt,
		SessionID:       session.ID,
		UserID:          record.ID,
	}, nil
}

// CurrentUser возвращает профиль по идентификатору.
func (a *Auth) CurrentUser(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	record, err := a.users.ByID(ctx, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Пользователь")
		}
		return nil, apierr.Internal(err)
	}
	user := record.User
	return &user, nil
}

// ProfileSecrets — расшифрованные персональные поля, только для владельца.
type ProfileSecrets struct {
	Passport string
	Address  string
}

// ProfileSecrets возвращает паспорт и адрес владельца. Чужому аккаунту
// этот метод не вызывается: обработчик берёт идентификатор из токена.
func (a *Auth) ProfileSecrets(ctx context.Context, userID uuid.UUID) (*ProfileSecrets, error) {
	if a.cipher == nil {
		return &ProfileSecrets{}, nil
	}
	raw, err := a.users.EncryptedPII(ctx, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Пользователь")
		}
		return nil, apierr.Internal(err)
	}
	passport, err := a.cipher.Decrypt(raw.Passport)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	address, err := a.cipher.Decrypt(raw.Address)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return &ProfileSecrets{Passport: passport, Address: address}, nil
}

// UpdateProfileInput — поля, которые пользователь правит сам.
type UpdateProfileInput struct {
	FullName string
	Passport *string
	Address  *string
}

// UpdateProfile сохраняет имя и шифрует паспорт/адрес на уровне приложения.
func (a *Auth) UpdateProfile(ctx context.Context, userID uuid.UUID, input UpdateProfileInput, meta RequestMeta) (*domain.User, error) {
	fullName := strings.TrimSpace(input.FullName)
	if fullName != "" && len([]rune(fullName)) < 2 {
		return nil, apierr.Validation(map[string]string{"full_name": "укажите имя не короче двух символов"})
	}

	record, err := a.users.UpdateProfile(ctx, userID, store.UpdateProfileParams{FullName: fullName})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Пользователь")
		}
		return nil, apierr.Internal(err)
	}

	if input.Passport != nil || input.Address != nil {
		if a.cipher == nil {
			return nil, apierr.Internal(errors.New("шифровальщик персональных данных не настроен"))
		}
		current, err := a.users.EncryptedPII(ctx, userID)
		if err != nil {
			return nil, apierr.Internal(err)
		}
		passport := current.Passport
		address := current.Address
		if input.Passport != nil {
			passport, err = a.cipher.Encrypt(strings.TrimSpace(*input.Passport))
			if err != nil {
				return nil, apierr.Internal(err)
			}
		}
		if input.Address != nil {
			address, err = a.cipher.Encrypt(strings.TrimSpace(*input.Address))
			if err != nil {
				return nil, apierr.Internal(err)
			}
		}
		if err := a.users.SetEncryptedPII(ctx, userID, passport, address); err != nil {
			return nil, apierr.Internal(err)
		}
	}

	a.recordSecurity(ctx, store.SecurityEvent{
		Kind: store.EventProfileUpdated, UserID: &userID,
		IP: meta.IP, UserAgent: meta.UserAgent,
	})

	user := record.User
	return &user, nil
}

// DeleteAccount обезличивает учётную запись после проверки пароля.
func (a *Auth) DeleteAccount(ctx context.Context, userID uuid.UUID, password string, meta RequestMeta) error {
	record, err := a.users.ByID(ctx, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Пользователь")
		}
		return apierr.Internal(err)
	}
	ok, err := a.hasher.Verify(password, record.PasswordHash)
	if err != nil {
		return apierr.Internal(err)
	}
	if !ok {
		return apierr.InvalidCredentials()
	}

	if err := a.LogoutEverywhere(ctx, userID, meta); err != nil {
		return err
	}
	if err := a.users.SoftDelete(ctx, userID); err != nil {
		return apierr.Internal(err)
	}
	a.recordSecurity(ctx, store.SecurityEvent{
		Kind: store.EventAccountDeleted, UserID: &userID,
		IP: meta.IP, UserAgent: meta.UserAgent, Severity: 3,
	})
	return nil
}

func (a *Auth) recordSecurity(ctx context.Context, event store.SecurityEvent) {
	// Журнал безопасности не должен ронять основной сценарий: если запись
	// не удалась, это ошибка эксплуатации, а не причина отказать
	// пользователю во входе.
	if err := a.securityLog.RecordEvent(context.WithoutCancel(ctx), event); err != nil {
		a.log.ErrorContext(ctx, "не удалось записать событие безопасности",
			slog.String("kind", event.Kind),
			slog.String("error", err.Error()))
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// normalizePhone приводит телефон к формату +7XXXXXXXXXX.
func normalizePhone(raw string) (string, error) {
	digits := make([]rune, 0, len(raw))
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}

	switch {
	case len(digits) == 11 && (digits[0] == '7' || digits[0] == '8'):
		return "+7" + string(digits[1:]), nil
	case len(digits) == 10:
		return "+7" + string(digits), nil
	case len(digits) >= 10 && len(digits) <= 15:
		// Зарубежные номера продавцов из Китая и Японии сохраняются как есть.
		return "+" + string(digits), nil
	default:
		return "", errors.New("укажите номер телефона в формате +7 900 000-00-00")
	}
}

// deviceLabel формирует человекочитаемое имя устройства для списка сессий.
//
// Полный User-Agent в интерфейсе бесполезен, а хранить его целиком в
// названии устройства — лишние персональные данные на экране.
func deviceLabel(userAgent string) string {
	ua := strings.ToLower(userAgent)

	platform := "Неизвестное устройство"
	switch {
	case strings.Contains(ua, "android"):
		platform = "Android"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"):
		platform = "iOS"
	case strings.Contains(ua, "windows"):
		platform = "Windows"
	case strings.Contains(ua, "mac os"), strings.Contains(ua, "macintosh"):
		platform = "macOS"
	case strings.Contains(ua, "linux"):
		platform = "Linux"
	}

	browser := ""
	switch {
	case strings.Contains(ua, "yabrowser"):
		browser = "Яндекс.Браузер"
	case strings.Contains(ua, "edg/"):
		browser = "Edge"
	case strings.Contains(ua, "opr/"), strings.Contains(ua, "opera"):
		browser = "Opera"
	case strings.Contains(ua, "firefox"):
		browser = "Firefox"
	case strings.Contains(ua, "chrome"):
		browser = "Chrome"
	case strings.Contains(ua, "safari"):
		browser = "Safari"
	}

	if browser == "" {
		return platform
	}
	return platform + ", " + browser
}

func humanWait(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d с", int(d.Seconds())+1)
	case d < time.Hour:
		return fmt.Sprintf("%d мин", int(d.Minutes())+1)
	default:
		return fmt.Sprintf("%d ч", int(d.Hours())+1)
	}
}

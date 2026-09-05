package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessClaims — полезная нагрузка короткоживущего access-токена.
//
// В токен кладётся минимум: идентификатор пользователя, роль и признаки
// подтверждения контактов. Ничего, что может измениться и стать опасно
// устаревшим (например, статус блокировки), в токене не хранится — такие
// проверки делаются по базе.
type AccessClaims struct {
	UserID        uuid.UUID `json:"sub_id"`
	Role          string    `json:"role"`
	SessionID     uuid.UUID `json:"sid"`
	EmailVerified bool      `json:"ev"`
	PhoneVerified bool      `json:"pv"`
	jwt.RegisteredClaims
}

// TokenIssuer выпускает и проверяет access-токены.
type TokenIssuer struct {
	secret    []byte
	issuer    string
	accessTTL time.Duration
}

func NewTokenIssuer(secret []byte, issuer string, accessTTL time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: secret, issuer: issuer, accessTTL: accessTTL}
}

// AccessTTL возвращает срок жизни access-токена.
func (t *TokenIssuer) AccessTTL() time.Duration { return t.accessTTL }

// IssueAccess выпускает access-токен для пользователя.
func (t *TokenIssuer) IssueAccess(userID, sessionID uuid.UUID, role string, emailVerified, phoneVerified bool) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(t.accessTTL)

	claims := AccessClaims{
		UserID:        userID,
		Role:          role,
		SessionID:     sessionID,
		EmailVerified: emailVerified,
		PhoneVerified: phoneVerified,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    t.issuer,
			Subject:   userID.String(),
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("подпись access-токена: %w", err)
	}
	return signed, expiresAt, nil
}

// ErrTokenExpired отделяет истёкший токен от повреждённого: интерфейс на
// первый случай молча обновляет сессию, а на второй разлогинивает.
var (
	ErrTokenExpired = errors.New("срок действия токена истёк")
	ErrTokenInvalid = errors.New("токен недействителен")
)

// ParseAccess проверяет подпись и срок действия токена.
//
// Алгоритм подписи задаётся явно через WithValidMethods: иначе возможна
// классическая атака с подменой alg на none или на асимметричный алгоритм.
func (t *TokenIssuer) ParseAccess(raw string) (*AccessClaims, error) {
	claims := &AccessClaims{}

	parsed, err := jwt.ParseWithClaims(raw, claims,
		func(token *jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(t.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(5*time.Second),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}
	if !parsed.Valid {
		return nil, ErrTokenInvalid
	}
	if claims.UserID == uuid.Nil {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

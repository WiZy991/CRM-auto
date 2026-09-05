package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// SessionRevoker — список отозванных сессий в Redis.
//
// Зачем нужен отдельный список, если refresh-сессии и так лежат в базе:
// access-токен проверяется по подписи, без обращения к базе, иначе каждый
// запрос к API стоил бы дополнительный SELECT. Но выход из аккаунта,
// блокировка пользователя и обнаружение кражи токена должны действовать
// немедленно, а не через 15 минут.
//
// Компромисс: идентификаторы отозванных сессий кладутся в Redis на срок
// жизни access-токена. Проверка — один GET, память — только по реально
// отозванным сессиям.
type SessionRevoker struct {
	rdb            redis.UniversalClient
	accessTokenTTL time.Duration
}

func NewSessionRevoker(rdb redis.UniversalClient, accessTokenTTL time.Duration) *SessionRevoker {
	if accessTokenTTL <= 0 {
		accessTokenTTL = 15 * time.Minute
	}
	return &SessionRevoker{rdb: rdb, accessTokenTTL: accessTokenTTL}
}

func revokedKey(sessionID uuid.UUID) string {
	return "session:revoked:" + sessionID.String()
}

// Revoke добавляет сессию в список отозванных.
func (r *SessionRevoker) Revoke(ctx context.Context, sessionID uuid.UUID) error {
	if sessionID == uuid.Nil {
		return nil
	}
	// TTL с запасом: access-токен, выпущенный за миг до отзыва, должен
	// перестать работать до истечения записи в списке.
	ttl := r.accessTokenTTL + time.Minute
	if err := r.rdb.Set(ctx, revokedKey(sessionID), "1", ttl).Err(); err != nil {
		return fmt.Errorf("отметка сессии как отозванной: %w", err)
	}
	return nil
}

// RevokeMany отзывает несколько сессий одним конвейером.
func (r *SessionRevoker) RevokeMany(ctx context.Context, sessionIDs []uuid.UUID) error {
	if len(sessionIDs) == 0 {
		return nil
	}

	ttl := r.accessTokenTTL + time.Minute
	pipe := r.rdb.Pipeline()
	for _, id := range sessionIDs {
		if id == uuid.Nil {
			continue
		}
		pipe.Set(ctx, revokedKey(id), "1", ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("отметка сессий как отозванных: %w", err)
	}
	return nil
}

// IsSessionRevoked проверяет сессию.
//
// При недоступности Redis возвращается false: иначе сбой кеша полностью
// закрыл бы доступ всем пользователям. Риск ограничен — окно составляет
// не больше срока жизни access-токена.
func (r *SessionRevoker) IsSessionRevoked(ctx context.Context, sessionID uuid.UUID) bool {
	if sessionID == uuid.Nil {
		return false
	}
	count, err := r.rdb.Exists(ctx, revokedKey(sessionID)).Result()
	if err != nil {
		return false
	}
	return count > 0
}

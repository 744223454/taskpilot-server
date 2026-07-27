// Package cache provides Redis client helpers.
package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/redis/go-redis/v9"
)

const refreshSessionKeyPrefix = "taskpilot:auth:refresh_session:"

var rotateRefreshSessionScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
  return {0}
end

local expires_at = tonumber(redis.call("HGET", KEYS[1], "expires_at"))
if not expires_at or expires_at <= tonumber(ARGV[3]) then
  redis.call("DEL", KEYS[1])
  return {0}
end

local current_hash = redis.call("HGET", KEYS[1], "token_hash")
if not current_hash or current_hash ~= ARGV[1] then
  redis.call("DEL", KEYS[1])
  return {-1}
end

redis.call("HSET", KEYS[1], "token_hash", ARGV[2])
return {
  1,
  redis.call("HGET", KEYS[1], "user_id"),
  redis.call("HGET", KEYS[1], "email"),
  redis.call("HGET", KEYS[1], "nickname"),
  redis.call("HGET", KEYS[1], "avatar_url"),
  tostring(expires_at)
}
`)

var revokeRefreshSessionScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
  return 0
end

local current_hash = redis.call("HGET", KEYS[1], "token_hash")
redis.call("DEL", KEYS[1])
if not current_hash or current_hash ~= ARGV[1] then
  return -1
end
return 1
`)

type RefreshSessionStore struct {
	client *redis.Client
}

func NewRedis(host, password string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     host,
		Password: password,
		DB:       0,
	})
}

func NewRefreshSessionStore(client *redis.Client) *RefreshSessionStore {
	return &RefreshSessionStore{client: client}
}

func (s *RefreshSessionStore) Create(ctx context.Context, session jwtauth.RefreshSession, tokenHash string) error {
	if s == nil || s.client == nil {
		return errors.New("redis client is not configured")
	}

	key := refreshSessionKey(session.ID)
	_, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, key, map[string]any{
			"user_id":    session.UserID,
			"email":      session.Email,
			"nickname":   session.Nickname,
			"avatar_url": optionalString(session.AvatarURL),
			"token_hash": tokenHash,
			"expires_at": session.ExpiresAt.Unix(),
		})
		pipe.ExpireAt(ctx, key, session.ExpiresAt)
		return nil
	})
	if err != nil {
		return fmt.Errorf("create refresh session: %w", err)
	}
	return nil
}

func (s *RefreshSessionStore) Rotate(ctx context.Context, sessionID, currentHash, replacementHash string, now time.Time) (jwtauth.RefreshSession, error) {
	if s == nil || s.client == nil {
		return jwtauth.RefreshSession{}, errors.New("redis client is not configured")
	}

	result, err := rotateRefreshSessionScript.Run(
		ctx,
		s.client,
		[]string{refreshSessionKey(sessionID)},
		currentHash,
		replacementHash,
		now.Unix(),
	).Slice()
	if err != nil {
		return jwtauth.RefreshSession{}, fmt.Errorf("rotate refresh session: %w", err)
	}
	if len(result) == 0 {
		return jwtauth.RefreshSession{}, jwtauth.ErrRefreshSessionNotFound
	}

	status, err := redisInt64(result[0])
	if err != nil {
		return jwtauth.RefreshSession{}, fmt.Errorf("decode refresh rotation status: %w", err)
	}
	switch status {
	case 0:
		return jwtauth.RefreshSession{}, jwtauth.ErrRefreshSessionNotFound
	case -1:
		return jwtauth.RefreshSession{}, jwtauth.ErrRefreshTokenReused
	case 1:
		if len(result) != 6 {
			return jwtauth.RefreshSession{}, fmt.Errorf("decode refresh rotation result: unexpected field count %d", len(result))
		}
	default:
		return jwtauth.RefreshSession{}, fmt.Errorf("decode refresh rotation status: unexpected value %d", status)
	}

	userID, err := redisInt64(result[1])
	if err != nil {
		return jwtauth.RefreshSession{}, fmt.Errorf("decode refresh session user id: %w", err)
	}
	expiresAtUnix, err := redisInt64(result[5])
	if err != nil {
		return jwtauth.RefreshSession{}, fmt.Errorf("decode refresh session expiry: %w", err)
	}
	return jwtauth.RefreshSession{
		ID:        sessionID,
		UserID:    userID,
		Email:     redisString(result[2]),
		Nickname:  redisString(result[3]),
		AvatarURL: optionalStringPointer(redisString(result[4])),
		ExpiresAt: time.Unix(expiresAtUnix, 0).UTC(),
	}, nil
}

func (s *RefreshSessionStore) Revoke(ctx context.Context, sessionID, tokenHash string) error {
	if s == nil || s.client == nil {
		return errors.New("redis client is not configured")
	}

	result, err := revokeRefreshSessionScript.Run(
		ctx,
		s.client,
		[]string{refreshSessionKey(sessionID)},
		tokenHash,
	).Int64()
	if err != nil {
		return fmt.Errorf("revoke refresh session: %w", err)
	}
	switch result {
	case 1:
		return nil
	case 0:
		return jwtauth.ErrRefreshSessionNotFound
	case -1:
		return jwtauth.ErrRefreshTokenReused
	default:
		return fmt.Errorf("decode refresh revocation status: unexpected value %d", result)
	}
}

func refreshSessionKey(sessionID string) string {
	return refreshSessionKeyPrefix + sessionID
}

func redisInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported Redis value type %T", value)
	}
}

func redisString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

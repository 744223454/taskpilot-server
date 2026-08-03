package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var authRateLimitScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", now - window)
local count = redis.call("ZCARD", KEYS[1])
if count >= limit then
  local oldest = redis.call("ZRANGE", KEYS[1], 0, 0, "WITHSCORES")
  local retry_after = window
  if #oldest == 2 then
    retry_after = math.max(1, tonumber(oldest[2]) + window - now)
  end
  return {0, retry_after}
end
redis.call("ZADD", KEYS[1], now, ARGV[4])
redis.call("PEXPIRE", KEYS[1], window)
return {1, 0}
`)

type AuthRateLimiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, time.Duration, error)
}

type RedisAuthRateLimiter struct {
	client *redis.Client
}

func NewAuthRateLimiter(client *redis.Client) *RedisAuthRateLimiter {
	return &RedisAuthRateLimiter{client: client}
}

func (limiter *RedisAuthRateLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, time.Duration, error) {
	if limiter == nil || limiter.client == nil {
		return false, 0, errors.New("redis client is not configured")
	}
	if key == "" || limit <= 0 || window <= 0 {
		return false, 0, errors.New("rate limiter is misconfigured")
	}

	memberBytes := make([]byte, 16)
	if _, err := rand.Read(memberBytes); err != nil {
		return false, 0, fmt.Errorf("generate rate limit member: %w", err)
	}
	now := time.Now().UnixMilli()
	result, err := authRateLimitScript.Run(
		ctx,
		limiter.client,
		[]string{"taskpilot:auth:rate:" + key},
		now,
		window.Milliseconds(),
		limit,
		hex.EncodeToString(memberBytes),
	).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("apply auth rate limit: %w", err)
	}
	if len(result) != 2 {
		return false, 0, fmt.Errorf("decode auth rate limit result: unexpected field count %d", len(result))
	}
	return result[0] == 1, time.Duration(result[1]) * time.Millisecond, nil
}

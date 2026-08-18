package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var aiChatAcquireScript = redis.NewScript(`
local rate_key = KEYS[1]
local lock_key = KEYS[2]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local owner = ARGV[4]
local lock_ttl = tonumber(ARGV[5])

if redis.call("EXISTS", lock_key) == 1 then
  return {0, 1}
end
redis.call("ZREMRANGEBYSCORE", rate_key, "-inf", now - window)
local count = redis.call("ZCARD", rate_key)
if count >= limit then
  local oldest = redis.call("ZRANGE", rate_key, 0, 0, "WITHSCORES")
  local retry_after = window
  if #oldest == 2 then
    retry_after = math.max(1, tonumber(oldest[2]) + window - now)
  end
  return {0, retry_after}
end
redis.call("ZADD", rate_key, now, owner)
redis.call("PEXPIRE", rate_key, window)
redis.call("SET", lock_key, owner, "PX", lock_ttl)
return {1, 0}
`)

var aiChatReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type AIChatGuard interface {
	Acquire(ctx context.Context, userID int64, owner string, limit int64, window, lockTTL time.Duration) (bool, time.Duration, error)
	Release(ctx context.Context, userID int64, owner string) error
}

type RedisAIChatGuard struct {
	client *redis.Client
}

func NewAIChatGuard(client *redis.Client) *RedisAIChatGuard {
	return &RedisAIChatGuard{client: client}
}

func (guard *RedisAIChatGuard) Acquire(ctx context.Context, userID int64, owner string, limit int64, window, lockTTL time.Duration) (bool, time.Duration, error) {
	if guard == nil || guard.client == nil {
		return false, 0, errors.New("redis client is not configured")
	}
	if userID <= 0 || owner == "" || limit <= 0 || window <= 0 || lockTTL <= 0 {
		return false, 0, errors.New("AI chat guard is misconfigured")
	}
	userKey := strconv.FormatInt(userID, 10)
	result, err := aiChatAcquireScript.Run(ctx, guard.client, []string{
		"taskpilot:ai:chat:rate:" + userKey,
		"taskpilot:ai:chat:lock:" + userKey,
	}, time.Now().UnixMilli(), window.Milliseconds(), limit, owner, lockTTL.Milliseconds()).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("acquire AI chat guard: %w", err)
	}
	if len(result) != 2 {
		return false, 0, fmt.Errorf("decode AI chat guard result: unexpected field count %d", len(result))
	}
	return result[0] == 1, time.Duration(result[1]) * time.Millisecond, nil
}

func (guard *RedisAIChatGuard) Release(ctx context.Context, userID int64, owner string) error {
	if guard == nil || guard.client == nil || userID <= 0 || owner == "" {
		return nil
	}
	key := "taskpilot:ai:chat:lock:" + strconv.FormatInt(userID, 10)
	if err := aiChatReleaseScript.Run(ctx, guard.client, []string{key}, owner).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("release AI chat guard: %w", err)
	}
	return nil
}

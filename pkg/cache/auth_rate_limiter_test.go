package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisAuthRateLimiterEnforcesSlidingWindow(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()

	limiter := NewAuthRateLimiter(NewRedis(redisServer.Addr(), ""))
	for attempt := 1; attempt <= 2; attempt++ {
		allowed, _, err := limiter.Allow(context.Background(), "login:test", 2, time.Minute)
		if err != nil || !allowed {
			t.Fatalf("attempt %d = %v, %v; want allowed", attempt, allowed, err)
		}
	}
	allowed, retryAfter, err := limiter.Allow(context.Background(), "login:test", 2, time.Minute)
	if err != nil || allowed || retryAfter <= 0 {
		t.Fatalf("limited attempt = %v, %v, %v", allowed, retryAfter, err)
	}

	allowed, _, err = limiter.Allow(context.Background(), "login:other", 2, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("independent key = %v, %v; want allowed", allowed, err)
	}
}

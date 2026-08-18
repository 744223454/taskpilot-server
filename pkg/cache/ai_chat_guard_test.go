package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestAIChatGuardEnforcesConcurrencyAndOwnerRelease(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := NewRedis(redisServer.Addr(), "")
	guard := NewAIChatGuard(client)

	allowed, _, err := guard.Acquire(context.Background(), 7, "owner-one", 10, time.Minute, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("first Acquire() = %v, %v", allowed, err)
	}
	allowed, _, err = guard.Acquire(context.Background(), 7, "owner-two", 10, time.Minute, time.Minute)
	if err != nil || allowed {
		t.Fatalf("concurrent Acquire() = %v, %v", allowed, err)
	}
	if err := guard.Release(context.Background(), 7, "owner-two"); err != nil {
		t.Fatalf("wrong owner Release() error = %v", err)
	}
	allowed, _, _ = guard.Acquire(context.Background(), 7, "owner-three", 10, time.Minute, time.Minute)
	if allowed {
		t.Fatal("wrong owner released active lock")
	}
	if err := guard.Release(context.Background(), 7, "owner-one"); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	allowed, _, err = guard.Acquire(context.Background(), 7, "owner-four", 10, time.Minute, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("Acquire() after release = %v, %v", allowed, err)
	}
}

func TestAIChatGuardEnforcesRateLimit(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := NewRedis(redisServer.Addr(), "")
	guard := NewAIChatGuard(client)

	for index := 0; index < 2; index++ {
		owner := "owner-" + string(rune('a'+index))
		allowed, _, err := guard.Acquire(context.Background(), 9, owner, 2, time.Minute, time.Minute)
		if err != nil || !allowed {
			t.Fatalf("Acquire(%d) = %v, %v", index, allowed, err)
		}
		if err := guard.Release(context.Background(), 9, owner); err != nil {
			t.Fatalf("Release(%d) error = %v", index, err)
		}
	}
	allowed, retryAfter, err := guard.Acquire(context.Background(), 9, "owner-c", 2, time.Minute, time.Minute)
	if err != nil || allowed || retryAfter <= 0 {
		t.Fatalf("rate-limited Acquire() = %v, %v, %v", allowed, retryAfter, err)
	}
}

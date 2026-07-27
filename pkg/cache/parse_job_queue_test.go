package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisParseJobQueuePublishReadAckAndHeartbeat(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer redisServer.Close()

	queue := NewParseJobQueue(NewRedis(redisServer.Addr(), ""), "taskpilot:test:parse_jobs", "test-workers")
	ctx := context.Background()
	if err := queue.EnsureGroup(ctx); err != nil {
		t.Fatalf("EnsureGroup() error = %v", err)
	}
	if err := queue.EnsureGroup(ctx); err != nil {
		t.Fatalf("second EnsureGroup() error = %v", err)
	}
	messageID, err := queue.Publish(ctx, 42)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	messages, err := queue.Read(ctx, "consumer-1", 1, time.Millisecond)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(messages) != 1 || messages[0].ID != messageID || messages[0].JobID != 42 {
		t.Fatalf("messages = %#v", messages)
	}
	if err := queue.Ack(ctx, messageID); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if err := queue.Heartbeat(ctx, "consumer-1", 30*time.Second); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	healthy, err := queue.WorkerHealthy(ctx)
	if err != nil || !healthy {
		t.Fatalf("WorkerHealthy() = %v, %v", healthy, err)
	}
	redisServer.FastForward(31 * time.Second)
	healthy, err = queue.WorkerHealthy(ctx)
	if err != nil || healthy {
		t.Fatalf("WorkerHealthy() after expiry = %v, %v", healthy, err)
	}
}

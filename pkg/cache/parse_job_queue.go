package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const workerHeartbeatKey = "taskpilot:worker:heartbeat"

type ParseJobMessage struct {
	ID    string
	JobID int64
}

type ParseJobQueue interface {
	EnsureGroup(ctx context.Context) error
	Publish(ctx context.Context, jobID int64) (string, error)
	Read(ctx context.Context, consumer string, count int64, block time.Duration) ([]ParseJobMessage, error)
	ClaimStale(ctx context.Context, consumer string, minIdle time.Duration, count int64) ([]ParseJobMessage, error)
	Ack(ctx context.Context, messageID string) error
	TrimBefore(ctx context.Context, cutoff time.Time) error
	Heartbeat(ctx context.Context, consumer string, ttl time.Duration) error
	WorkerHealthy(ctx context.Context) (bool, error)
}

type RedisParseJobQueue struct {
	client    *redis.Client
	streamKey string
	group     string
}

func NewParseJobQueue(client *redis.Client, streamKey, group string) *RedisParseJobQueue {
	return &RedisParseJobQueue{client: client, streamKey: streamKey, group: group}
}

func (q *RedisParseJobQueue) EnsureGroup(ctx context.Context) error {
	if err := q.requireClient(); err != nil {
		return err
	}
	if err := q.client.XGroupCreateMkStream(ctx, q.streamKey, q.group, "0").Err(); err != nil && !isBusyGroupError(err) {
		return fmt.Errorf("create parse job consumer group: %w", err)
	}
	return nil
}

func (q *RedisParseJobQueue) Publish(ctx context.Context, jobID int64) (string, error) {
	if err := q.requireClient(); err != nil {
		return "", err
	}
	messageID, err := q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: q.streamKey,
		Values: map[string]any{"job_id": jobID},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("publish parse job: %w", err)
	}
	return messageID, nil
}

func (q *RedisParseJobQueue) Read(ctx context.Context, consumer string, count int64, block time.Duration) ([]ParseJobMessage, error) {
	if err := q.requireClient(); err != nil {
		return nil, err
	}
	streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    q.group,
		Consumer: consumer,
		Streams:  []string{q.streamKey, ">"},
		Count:    count,
		Block:    block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read parse jobs: %w", err)
	}
	return decodeParseJobStreams(streams), nil
}

func (q *RedisParseJobQueue) ClaimStale(ctx context.Context, consumer string, minIdle time.Duration, count int64) ([]ParseJobMessage, error) {
	if err := q.requireClient(); err != nil {
		return nil, err
	}
	messages, _, err := q.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   q.streamKey,
		Group:    q.group,
		Consumer: consumer,
		MinIdle:  minIdle,
		Start:    "0-0",
		Count:    count,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim stale parse jobs: %w", err)
	}
	return decodeParseJobMessages(messages), nil
}

func (q *RedisParseJobQueue) Ack(ctx context.Context, messageID string) error {
	if err := q.requireClient(); err != nil {
		return err
	}
	if err := q.client.XAck(ctx, q.streamKey, q.group, messageID).Err(); err != nil {
		return fmt.Errorf("ack parse job message: %w", err)
	}
	return nil
}

func (q *RedisParseJobQueue) TrimBefore(ctx context.Context, cutoff time.Time) error {
	if err := q.requireClient(); err != nil {
		return err
	}
	minID := strconv.FormatInt(cutoff.UnixMilli(), 10) + "-0"
	if err := q.client.XTrimMinIDApprox(ctx, q.streamKey, minID, 1000).Err(); err != nil {
		return fmt.Errorf("trim parse job stream: %w", err)
	}
	return nil
}

func (q *RedisParseJobQueue) Heartbeat(ctx context.Context, consumer string, ttl time.Duration) error {
	if err := q.requireClient(); err != nil {
		return err
	}
	if err := q.client.Set(ctx, workerHeartbeatKey, consumer, ttl).Err(); err != nil {
		return fmt.Errorf("write worker heartbeat: %w", err)
	}
	return nil
}

func (q *RedisParseJobQueue) WorkerHealthy(ctx context.Context) (bool, error) {
	if err := q.requireClient(); err != nil {
		return false, err
	}
	exists, err := q.client.Exists(ctx, workerHeartbeatKey).Result()
	if err != nil {
		return false, fmt.Errorf("check worker heartbeat: %w", err)
	}
	return exists > 0, nil
}

func (q *RedisParseJobQueue) requireClient() error {
	if q == nil || q.client == nil {
		return errors.New("redis client is not configured")
	}
	if q.streamKey == "" || q.group == "" {
		return errors.New("parse job queue is not configured")
	}
	return nil
}

func decodeParseJobStreams(streams []redis.XStream) []ParseJobMessage {
	messages := make([]ParseJobMessage, 0)
	for _, stream := range streams {
		messages = append(messages, decodeParseJobMessages(stream.Messages)...)
	}
	return messages
}

func decodeParseJobMessages(messages []redis.XMessage) []ParseJobMessage {
	decoded := make([]ParseJobMessage, 0, len(messages))
	for _, message := range messages {
		jobID, _ := redisInt64(message.Values["job_id"])
		decoded = append(decoded, ParseJobMessage{ID: message.ID, JobID: jobID})
	}
	return decoded
}

func isBusyGroupError(err error) bool {
	return err != nil && len(err.Error()) >= len("BUSYGROUP") && err.Error()[:len("BUSYGROUP")] == "BUSYGROUP"
}

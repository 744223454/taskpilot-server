package parsejob

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/model/documentmodel"
	"github.com/744223454/taskpilot-server/model/parsejobmodel"
	"github.com/744223454/taskpilot-server/model/parseresultmodel"
	"github.com/744223454/taskpilot-server/pkg/ai"
	cachepkg "github.com/744223454/taskpilot-server/pkg/cache"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const reconciliationBatchSize = 100

type Worker struct {
	db                *gorm.DB
	queue             cachepkg.ParseJobQueue
	parser            ai.Parser
	logger            *slog.Logger
	consumer          string
	concurrency       int
	blockTimeout      time.Duration
	reconcileInterval time.Duration
	pendingGrace      time.Duration
	leaseTimeout      time.Duration
	maxRecoveries     int32
	streamRetention   time.Duration
	heartbeatInterval time.Duration
	heartbeatTTL      time.Duration
	shutdownGrace     time.Duration
	processSlots      chan struct{}
}

func New(svcCtx *svc.ServiceContext) (*Worker, error) {
	if svcCtx == nil || svcCtx.DB == nil {
		return nil, errors.New("worker database is not configured")
	}
	if svcCtx.ParseJobs == nil {
		return nil, errors.New("worker parse job queue is not configured")
	}
	if svcCtx.Parser == nil {
		return nil, errors.New("worker AI parser is not configured")
	}
	logger := svcCtx.Logger
	if logger == nil {
		logger = slog.Default()
	}
	config := svcCtx.Config.Worker
	return &Worker{
		db:                svcCtx.DB,
		queue:             svcCtx.ParseJobs,
		parser:            svcCtx.Parser,
		logger:            logger,
		consumer:          newConsumerName(),
		concurrency:       config.Concurrency,
		blockTimeout:      time.Duration(config.BlockTimeout) * time.Second,
		reconcileInterval: time.Duration(config.ReconcileInterval) * time.Second,
		pendingGrace:      time.Duration(config.PendingGrace) * time.Second,
		leaseTimeout:      time.Duration(config.LeaseTimeout) * time.Second,
		maxRecoveries:     config.MaxRecoveries,
		streamRetention:   time.Duration(config.StreamRetention) * time.Second,
		heartbeatInterval: time.Duration(config.HeartbeatInterval) * time.Second,
		heartbeatTTL:      time.Duration(config.HeartbeatTTL) * time.Second,
		shutdownGrace:     time.Duration(config.ShutdownGrace) * time.Second,
		processSlots:      make(chan struct{}, config.Concurrency),
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.ensureGroup(ctx); err != nil {
		return err
	}

	workContext, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()

	var waitGroup sync.WaitGroup
	for range w.concurrency {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			w.consumeLoop(ctx, workContext)
		}()
	}
	for _, loop := range []func(context.Context, context.Context){
		w.reclaimLoop,
		w.reconcileLoop,
		w.heartbeatLoop,
		w.trimLoop,
	} {
		waitGroup.Add(1)
		go func(run func(context.Context, context.Context)) {
			defer waitGroup.Done()
			run(ctx, workContext)
		}(loop)
	}

	w.logger.Info("parse worker started", "consumer", w.consumer, "concurrency", w.concurrency)
	<-ctx.Done()
	w.logger.Info("parse worker stopping", "consumer", w.consumer)

	done := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(done)
	}()

	timer := time.NewTimer(w.shutdownGrace)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		cancelWork()
		w.logger.Warn("parse worker graceful shutdown timed out", "consumer", w.consumer)
		return nil
	}
}

func (w *Worker) ProcessJob(ctx context.Context, jobID int64) error {
	_, err := w.processJob(ctx, jobID)
	return err
}

func (w *Worker) ensureGroup(ctx context.Context) error {
	backoff := time.Second
	for {
		if err := w.queue.EnsureGroup(ctx); err == nil {
			return nil
		} else {
			w.logger.ErrorContext(ctx, "ensure parse worker group failed", "error", err)
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func (w *Worker) consumeLoop(runContext, workContext context.Context) {
	backoff := time.Second
	for runContext.Err() == nil {
		messages, err := w.queue.Read(runContext, w.consumer, 1, w.blockTimeout)
		if err != nil {
			w.logger.ErrorContext(runContext, "read parse job queue failed", "error", err)
			_ = w.queue.EnsureGroup(runContext)
			if !waitWithContext(runContext, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = time.Second
		for _, message := range messages {
			w.handleMessage(runContext, workContext, message)
		}
	}
}

func (w *Worker) reclaimLoop(runContext, workContext context.Context) {
	ticker := time.NewTicker(w.reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-runContext.Done():
			return
		case <-ticker.C:
			if err := w.recoverStaleJobs(runContext); err != nil {
				w.logger.ErrorContext(runContext, "recover stale parse jobs failed", "error", err)
			}
			messages, err := w.queue.ClaimStale(runContext, w.consumer, w.leaseTimeout, reconciliationBatchSize)
			if err != nil {
				w.logger.ErrorContext(runContext, "claim stale parse messages failed", "error", err)
				continue
			}
			for _, message := range messages {
				w.handleMessage(runContext, workContext, message)
			}
		}
	}
}

func (w *Worker) reconcileLoop(runContext, _ context.Context) {
	if err := w.reconcilePendingJobs(runContext); err != nil {
		w.logger.ErrorContext(runContext, "initial parse job reconciliation failed", "error", err)
	}
	ticker := time.NewTicker(w.reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-runContext.Done():
			return
		case <-ticker.C:
			if err := w.reconcilePendingJobs(runContext); err != nil {
				w.logger.ErrorContext(runContext, "parse job reconciliation failed", "error", err)
			}
		}
	}
}

func (w *Worker) heartbeatLoop(runContext, _ context.Context) {
	w.writeHeartbeat(runContext)
	ticker := time.NewTicker(w.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-runContext.Done():
			return
		case <-ticker.C:
			w.writeHeartbeat(runContext)
		}
	}
}

func (w *Worker) trimLoop(runContext, _ context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-runContext.Done():
			return
		case <-ticker.C:
			if err := w.queue.TrimBefore(runContext, time.Now().Add(-w.streamRetention)); err != nil {
				w.logger.ErrorContext(runContext, "trim parse job stream failed", "error", err)
			}
		}
	}
}

func (w *Worker) writeHeartbeat(ctx context.Context) {
	if err := w.queue.Heartbeat(ctx, w.consumer, w.heartbeatTTL); err != nil {
		w.logger.ErrorContext(ctx, "write parse worker heartbeat failed", "error", err)
	}
}

func (w *Worker) handleMessage(runContext, workContext context.Context, message cachepkg.ParseJobMessage) {
	if !w.acquireProcessSlot(runContext) {
		return
	}
	defer func() { <-w.processSlots }()

	if message.JobID <= 0 {
		w.logger.ErrorContext(workContext, "invalid parse job message", "message_id", message.ID)
		w.ack(workContext, message.ID)
		return
	}

	jobContext, cancel := context.WithTimeout(workContext, w.leaseTimeout)
	defer cancel()
	terminal, err := w.processJob(jobContext, message.JobID)
	if err != nil {
		w.logger.ErrorContext(jobContext, "parse job processing failed", "job_id", message.JobID, "error", err)
	}
	if terminal {
		w.ack(jobContext, message.ID)
	}
}

func (w *Worker) processJob(ctx context.Context, jobID int64) (bool, error) {
	job, claimed, err := w.claimJob(ctx, jobID)
	if err != nil {
		return false, err
	}
	if !claimed {
		return job.Status == "success" || job.Status == "failed", nil
	}

	document, err := gorm.G[documentmodel.Document](w.db).
		Where("id = ? AND user_id = ?", job.DocumentID, job.UserID).
		First(ctx)
	if err != nil {
		processingErr := fmt.Errorf("load parse job document: %w", err)
		return w.failJob(ctx, job.ID, "source document is unavailable", processingErr)
	}
	content := document.TextInput
	if content == nil {
		content = document.RawText
	}
	if content == nil || strings.TrimSpace(*content) == "" {
		processingErr := errors.New("source document text is empty")
		return w.failJob(ctx, job.ID, "source document text is empty", processingErr)
	}

	parsed, err := w.parser.Parse(ctx, *content)
	if err != nil {
		return w.failJob(ctx, job.ID, ai.PublicErrorMessage(err), fmt.Errorf("parse document with AI: %w", err))
	}
	if err := w.completeJob(ctx, job, parsed); err != nil {
		return false, err
	}
	return true, nil
}

func (w *Worker) claimJob(ctx context.Context, jobID int64) (parsejobmodel.ParseJob, bool, error) {
	now := time.Now()
	rowsAffected, err := gorm.G[parsejobmodel.ParseJob](w.db).
		Where("id = ? AND status = ?", jobID, "pending").
		Set(clause.Assignments(map[string]any{
			"status":        "processing",
			"started_at":    now,
			"finished_at":   nil,
			"error_message": nil,
			"updated_at":    now,
		})).
		Update(ctx)
	if err != nil {
		return parsejobmodel.ParseJob{}, false, fmt.Errorf("claim parse job: %w", err)
	}
	job, err := gorm.G[parsejobmodel.ParseJob](w.db).Where("id = ?", jobID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return job, false, nil
	}
	if err != nil {
		return job, false, fmt.Errorf("load claimed parse job: %w", err)
	}
	return job, rowsAffected == 1, nil
}

func (w *Worker) completeJob(ctx context.Context, job parsejobmodel.ParseJob, parsed ai.ParsedDocument) error {
	deliverables, err := json.Marshal(parsed.Deliverables)
	if err != nil {
		return fmt.Errorf("encode parse result deliverables: %w", err)
	}
	keyRequirements, err := json.Marshal(parsed.KeyRequirements)
	if err != nil {
		return fmt.Errorf("encode parse result requirements: %w", err)
	}
	riskWarnings, err := json.Marshal(parsed.RiskWarnings)
	if err != nil {
		return fmt.Errorf("encode parse result risks: %w", err)
	}
	generatedTasks, err := json.Marshal(parsed.GeneratedTasks)
	if err != nil {
		return fmt.Errorf("encode parse result tasks: %w", err)
	}

	model := parsed.Model
	return w.db.Transaction(func(tx *gorm.DB) error {
		result := parseresultmodel.ParseResult{
			UserID:          job.UserID,
			DocumentID:      job.DocumentID,
			ParseJobID:      job.ID,
			Title:           parsed.Title,
			Summary:         parsed.Summary,
			Deadline:        parsed.Deadline,
			Deliverables:    deliverables,
			KeyRequirements: keyRequirements,
			RiskWarnings:    riskWarnings,
			GeneratedTasks:  generatedTasks,
			AIModel:         &model,
			Version:         1,
		}
		if err := gorm.G[parseresultmodel.ParseResult](tx).Create(ctx, &result); err != nil {
			return fmt.Errorf("create parse result: %w", err)
		}
		now := time.Now()
		rowsAffected, err := gorm.G[parsejobmodel.ParseJob](tx).
			Where("id = ? AND status = ?", job.ID, "processing").
			Set(clause.Assignments(map[string]any{
				"status":        "success",
				"error_message": nil,
				"finished_at":   now,
				"updated_at":    now,
			})).
			Update(ctx)
		if err != nil {
			return fmt.Errorf("complete parse job: %w", err)
		}
		if rowsAffected != 1 {
			return errors.New("complete parse job: processing state was lost")
		}
		return nil
	})
}

func (w *Worker) failJob(ctx context.Context, jobID int64, message string, processingErr error) (bool, error) {
	now := time.Now()
	rowsAffected, err := gorm.G[parsejobmodel.ParseJob](w.db).
		Where("id = ? AND status = ?", jobID, "processing").
		Set(clause.Assignments(map[string]any{
			"status":        "failed",
			"error_message": message,
			"finished_at":   now,
			"updated_at":    now,
		})).
		Update(ctx)
	if err != nil {
		return false, fmt.Errorf("mark parse job failed after %v: %w", processingErr, err)
	}
	if rowsAffected != 1 {
		return false, fmt.Errorf("mark parse job failed after %v: processing state was lost", processingErr)
	}
	return true, processingErr
}

func (w *Worker) recoverStaleJobs(ctx context.Context) error {
	cutoff := time.Now().Add(-w.leaseTimeout)
	jobs, err := gorm.G[parsejobmodel.ParseJob](w.db).
		Where("status = ? AND started_at < ?", "processing", cutoff).
		Order("started_at ASC, id ASC").
		Limit(reconciliationBatchSize).
		Find(ctx)
	if err != nil {
		return fmt.Errorf("list stale parse jobs: %w", err)
	}

	for _, job := range jobs {
		now := time.Now()
		if job.RetryCount >= w.maxRecoveries {
			rowsAffected, updateErr := gorm.G[parsejobmodel.ParseJob](w.db).
				Where("id = ? AND status = ? AND started_at < ?", job.ID, "processing", cutoff).
				Set(clause.Assignments(map[string]any{
					"status":        "failed",
					"error_message": "parse worker recovery limit exceeded",
					"finished_at":   now,
					"updated_at":    now,
				})).
				Update(ctx)
			if updateErr != nil {
				return fmt.Errorf("fail exhausted stale parse job %d: %w", job.ID, updateErr)
			}
			if rowsAffected == 1 {
				w.logger.WarnContext(ctx, "stale parse job recovery exhausted", "job_id", job.ID)
			}
			continue
		}

		rowsAffected, updateErr := gorm.G[parsejobmodel.ParseJob](w.db).
			Where("id = ? AND status = ? AND started_at < ?", job.ID, "processing", cutoff).
			Set(clause.Assignments(map[string]any{
				"status":        "pending",
				"retry_count":   job.RetryCount + 1,
				"error_message": nil,
				"started_at":    nil,
				"finished_at":   nil,
				"updated_at":    now,
			})).
			Update(ctx)
		if updateErr != nil {
			return fmt.Errorf("reset stale parse job %d: %w", job.ID, updateErr)
		}
		if rowsAffected == 1 {
			if _, publishErr := w.queue.Publish(ctx, job.ID); publishErr != nil {
				w.logger.ErrorContext(ctx, "republish recovered parse job failed", "job_id", job.ID, "error", publishErr)
			}
		}
	}
	return nil
}

func (w *Worker) reconcilePendingJobs(ctx context.Context) error {
	cutoff := time.Now().Add(-w.pendingGrace)
	jobs, err := gorm.G[parsejobmodel.ParseJob](w.db).
		Select("id").
		Where("status = ? AND updated_at < ?", "pending", cutoff).
		Order("updated_at ASC, id ASC").
		Limit(reconciliationBatchSize).
		Find(ctx)
	if err != nil {
		return fmt.Errorf("list pending parse jobs: %w", err)
	}
	for _, job := range jobs {
		if _, err := w.queue.Publish(ctx, job.ID); err != nil {
			return fmt.Errorf("republish pending parse job %d: %w", job.ID, err)
		}
		now := time.Now()
		if _, err := gorm.G[parsejobmodel.ParseJob](w.db).
			Where("id = ? AND status = ? AND updated_at < ?", job.ID, "pending", cutoff).
			Set(clause.Assignments(map[string]any{"updated_at": now})).
			Update(ctx); err != nil {
			return fmt.Errorf("mark pending parse job %d reconciled: %w", job.ID, err)
		}
	}
	return nil
}

func (w *Worker) ack(ctx context.Context, messageID string) {
	if err := w.queue.Ack(ctx, messageID); err != nil {
		w.logger.ErrorContext(ctx, "ack parse job message failed", "message_id", messageID, "error", err)
	}
}

func (w *Worker) acquireProcessSlot(ctx context.Context) bool {
	select {
	case w.processSlots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func newConsumerName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "worker"
	}
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("%s-%d-%d", hostname, os.Getpid(), time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%s", hostname, os.Getpid(), hex.EncodeToString(random))
}

func waitWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextBackoff(current time.Duration) time.Duration {
	current *= 2
	if current > 30*time.Second {
		return 30 * time.Second
	}
	return current
}

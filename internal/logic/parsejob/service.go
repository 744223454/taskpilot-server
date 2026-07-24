package parsejob

import (
	"context"
	"errors"
	"fmt"
	"time"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/model/documentmodel"
	"github.com/744223454/taskpilot-server/model/parsejobmodel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewService(ctx context.Context, svcCtx *svc.ServiceContext) *Service {
	return &Service{ctx: ctx, svcCtx: svcCtx}
}

func (s *Service) Create(userID int64, req *types.CreateParseJobRequest) (*types.ParseJobResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	var job parsejobmodel.ParseJob
	err := s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		document, err := gorm.G[documentmodel.Document](tx, clause.Locking{Strength: "UPDATE"}).
			Select("id", "status").
			Where("id = ? AND user_id = ?", req.DocumentID, userID).
			First(s.ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return logicerrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock document for parse job creation: %w", err)
		}
		if document.Status != "ready" {
			return logicerrors.ErrInvalidState
		}
		activeJobs, err := gorm.G[parsejobmodel.ParseJob](tx).
			Where("document_id = ? AND status IN ?", req.DocumentID, []string{"pending", "processing"}).
			Count(s.ctx, "id")
		if err != nil {
			return fmt.Errorf("count active parse jobs: %w", err)
		}
		if activeJobs > 0 {
			return logicerrors.ErrConflict
		}

		job = parsejobmodel.ParseJob{
			UserID:     userID,
			DocumentID: req.DocumentID,
			JobType:    "ai_parse",
			Status:     "pending",
			RetryCount: 0,
		}
		if err := gorm.G[parsejobmodel.ParseJob](tx).Create(s.ctx, &job); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return logicerrors.ErrConflict
			}
			return fmt.Errorf("create parse job: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	response := parseJobResponse(job)
	return &response, nil
}

func (s *Service) Get(userID, jobID int64) (*types.ParseJobResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	job, err := s.find(userID, jobID)
	if err != nil {
		return nil, err
	}
	response := parseJobResponse(job)
	return &response, nil
}

func (s *Service) Latest(userID, documentID int64) (*types.ParseJobResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	job, err := gorm.G[parsejobmodel.ParseJob](s.svcCtx.DB).
		Where("user_id = ? AND document_id = ?", userID, documentID).
		Order("created_at DESC, id DESC").
		First(s.ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, logicerrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest parse job: %w", err)
	}

	response := parseJobResponse(job)
	return &response, nil
}

func (s *Service) Retry(userID, jobID int64) (*types.ParseJobResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	initialJob, err := s.find(userID, jobID)
	if err != nil {
		return nil, err
	}

	var job parsejobmodel.ParseJob
	err = s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		_, err := gorm.G[documentmodel.Document](tx, clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("id = ? AND user_id = ?", initialJob.DocumentID, userID).
			First(s.ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return logicerrors.ErrInvalidState
		}
		if err != nil {
			return fmt.Errorf("lock document for parse job retry: %w", err)
		}

		lockedJob, err := gorm.G[parsejobmodel.ParseJob](tx, clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", jobID, userID).
			First(s.ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return logicerrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock parse job for retry: %w", err)
		}
		if lockedJob.Status != "failed" {
			return logicerrors.ErrInvalidState
		}

		rowsAffected, err := gorm.G[parsejobmodel.ParseJob](tx).
			Where("id = ? AND user_id = ? AND status = ?", lockedJob.ID, userID, "failed").
			Set(clause.Assignments(map[string]any{
				"status":        "pending",
				"retry_count":   lockedJob.RetryCount + 1,
				"error_message": nil,
				"started_at":    nil,
				"finished_at":   nil,
				"updated_at":    time.Now(),
			})).
			Update(s.ctx)
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return logicerrors.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("reset parse job for retry: %w", err)
		}
		if rowsAffected == 0 {
			return logicerrors.ErrInvalidState
		}

		job, err = gorm.G[parsejobmodel.ParseJob](tx).
			Where("id = ? AND user_id = ?", lockedJob.ID, userID).
			First(s.ctx)
		if err != nil {
			return fmt.Errorf("reload retried parse job: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	response := parseJobResponse(job)
	return &response, nil
}

func (s *Service) find(userID, jobID int64) (parsejobmodel.ParseJob, error) {
	job, err := gorm.G[parsejobmodel.ParseJob](s.svcCtx.DB).
		Where("id = ? AND user_id = ?", jobID, userID).
		First(s.ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return job, logicerrors.ErrNotFound
	}
	if err != nil {
		return job, fmt.Errorf("get parse job: %w", err)
	}
	return job, nil
}

func (s *Service) requireDB() error {
	if s.svcCtx.DB == nil {
		return logicerrors.ErrDatabaseUnavailable
	}
	return nil
}

func parseJobResponse(job parsejobmodel.ParseJob) types.ParseJobResponse {
	return types.ParseJobResponse{
		ID:           job.ID,
		DocumentID:   job.DocumentID,
		JobType:      job.JobType,
		Status:       job.Status,
		RetryCount:   job.RetryCount,
		ErrorMessage: job.ErrorMessage,
		StartedAt:    job.StartedAt,
		FinishedAt:   job.FinishedAt,
		CreatedAt:    job.CreatedAt,
		UpdatedAt:    job.UpdatedAt,
	}
}

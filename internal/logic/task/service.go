package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/model/projectmodel"
	"github.com/744223454/taskpilot-server/model/taskmodel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	projectStatusActive  = "active"
	projectStatusDeleted = "deleted"
)

type Service struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewService(ctx context.Context, svcCtx *svc.ServiceContext) *Service {
	return &Service{ctx: ctx, svcCtx: svcCtx}
}

func (s *Service) List(userID, projectID int64, req *types.TaskListRequest) (*types.TaskListResponse, error) {
	return s.list(userID, projectID, req, false)
}

func (s *Service) HistoryList(userID, projectID int64, req *types.TaskListRequest) (*types.TaskListResponse, error) {
	return s.list(userID, projectID, req, true)
}

func (s *Service) Create(userID, projectID int64, req *types.CreateTaskRequest) (*types.TaskResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	normalized, err := normalizeCreateRequest(userID, projectID, req)
	if err != nil {
		return nil, err
	}

	var created taskmodel.Task
	err = s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		project, lockErr := lockProject(s.ctx, tx, userID, projectID, false)
		if lockErr != nil {
			return lockErr
		}
		if project.Status != projectStatusActive {
			return logicerrors.ErrConflict
		}

		type maxSortOrder struct {
			Value int32
		}
		rows, queryErr := gorm.G[maxSortOrder](tx).
			Raw("SELECT COALESCE(MAX(sort_order), -1) AS value FROM tasks WHERE project_id = ? AND user_id = ?", projectID, userID).
			Find(s.ctx)
		if queryErr != nil {
			return fmt.Errorf("find maximum task sort order: %w", queryErr)
		}
		maxOrder := int32(-1)
		if len(rows) > 0 {
			maxOrder = rows[0].Value
		}

		created = taskmodel.Task{
			ProjectID:   projectID,
			UserID:      userID,
			Title:       normalized.Title,
			Description: normalized.Description,
			Status:      "todo",
			Priority:    normalized.Priority,
			Deadline:    normalized.Deadline,
			SortOrder:   maxOrder + 1,
			SourceType:  "manual",
			Version:     1,
		}
		if createErr := gorm.G[taskmodel.Task](tx).Create(s.ctx, &created); createErr != nil {
			return fmt.Errorf("create task: %w", createErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	response := taskResponse(created)
	return &response, nil
}

func (s *Service) Update(userID, taskID int64, req *types.UpdateTaskRequest) (*types.TaskResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	normalized, err := normalizeUpdateRequest(userID, taskID, req)
	if err != nil {
		return nil, err
	}

	var updated taskmodel.Task
	err = s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		current, lockErr := lockWritableTask(s.ctx, tx, userID, taskID)
		if lockErr != nil {
			return lockErr
		}
		if current.Version != normalized.Version {
			return logicerrors.ErrConflict
		}

		rowsAffected, updateErr := gorm.G[taskmodel.Task](tx).
			Where("id = ? AND user_id = ? AND version = ?", taskID, userID, current.Version).
			Set(clause.Assignments(map[string]any{
				"title":       normalized.Title,
				"description": normalized.Description,
				"priority":    normalized.Priority,
				"deadline":    normalized.Deadline,
				"version":     current.Version + 1,
				"updated_at":  time.Now(),
			})).
			Update(s.ctx)
		if updateErr != nil {
			return fmt.Errorf("update task: %w", updateErr)
		}
		if rowsAffected != 1 {
			return logicerrors.ErrConflict
		}
		updated, updateErr = gorm.G[taskmodel.Task](tx).
			Where("id = ? AND user_id = ?", taskID, userID).
			First(s.ctx)
		if updateErr != nil {
			return fmt.Errorf("reload updated task: %w", updateErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	response := taskResponse(updated)
	return &response, nil
}

func (s *Service) UpdateStatus(userID, taskID int64, req *types.UpdateTaskStatusRequest) (*types.TaskResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if userID <= 0 || taskID <= 0 || req == nil || !validTaskStatus(req.Status) {
		return nil, logicerrors.ErrInvalidInput
	}

	var updated taskmodel.Task
	err := s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		current, lockErr := lockWritableTask(s.ctx, tx, userID, taskID)
		if lockErr != nil {
			return lockErr
		}
		if current.Status == req.Status {
			updated = current
			return nil
		}
		rowsAffected, updateErr := gorm.G[taskmodel.Task](tx).
			Where("id = ? AND user_id = ? AND status = ?", taskID, userID, current.Status).
			Set(clause.Assignments(map[string]any{
				"status":     req.Status,
				"version":    current.Version + 1,
				"updated_at": time.Now(),
			})).
			Update(s.ctx)
		if updateErr != nil {
			return fmt.Errorf("update task status: %w", updateErr)
		}
		if rowsAffected != 1 {
			return logicerrors.ErrConflict
		}
		updated, updateErr = gorm.G[taskmodel.Task](tx).
			Where("id = ? AND user_id = ?", taskID, userID).
			First(s.ctx)
		if updateErr != nil {
			return fmt.Errorf("reload task after status update: %w", updateErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	response := taskResponse(updated)
	return &response, nil
}

func (s *Service) Delete(userID, taskID int64) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	if userID <= 0 || taskID <= 0 {
		return logicerrors.ErrInvalidInput
	}
	return s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := lockWritableTask(s.ctx, tx, userID, taskID); err != nil {
			return err
		}
		rowsAffected, err := gorm.G[taskmodel.Task](tx).
			Where("id = ? AND user_id = ?", taskID, userID).
			Delete(s.ctx)
		if err != nil {
			return fmt.Errorf("delete task: %w", err)
		}
		if rowsAffected != 1 {
			return logicerrors.ErrNotFound
		}
		return nil
	})
}

func (s *Service) Reorder(userID int64, req *types.ReorderTasksRequest) (*types.TaskListResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if err := validateReorderRequest(userID, req); err != nil {
		return nil, err
	}

	var reordered []taskmodel.Task
	err := s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		project, lockErr := lockProject(s.ctx, tx, userID, req.ProjectID, false)
		if lockErr != nil {
			return lockErr
		}
		if project.Status != projectStatusActive {
			return logicerrors.ErrConflict
		}

		current, findErr := gorm.G[taskmodel.Task](tx).
			Where("project_id = ? AND user_id = ?", req.ProjectID, userID).
			Order("sort_order ASC, id ASC").
			Find(s.ctx)
		if findErr != nil {
			return fmt.Errorf("list tasks before reorder: %w", findErr)
		}
		if len(current) != len(req.TaskIDs) {
			return logicerrors.ErrConflict
		}
		currentIDs := make(map[int64]struct{}, len(current))
		for _, task := range current {
			currentIDs[task.ID] = struct{}{}
		}
		for _, taskID := range req.TaskIDs {
			if _, ok := currentIDs[taskID]; !ok {
				return logicerrors.ErrConflict
			}
		}

		now := time.Now()
		for index, taskID := range req.TaskIDs {
			rowsAffected, updateErr := gorm.G[taskmodel.Task](tx).
				Where("id = ? AND project_id = ? AND user_id = ?", taskID, req.ProjectID, userID).
				Set(clause.Assignments(map[string]any{
					"sort_order": int32(index),
					"updated_at": now,
				})).
				Update(s.ctx)
			if updateErr != nil {
				return fmt.Errorf("update task sort order: %w", updateErr)
			}
			if rowsAffected != 1 {
				return logicerrors.ErrConflict
			}
		}
		reordered, findErr = gorm.G[taskmodel.Task](tx).
			Where("project_id = ? AND user_id = ?", req.ProjectID, userID).
			Order("sort_order ASC, id ASC").
			Find(s.ctx)
		if findErr != nil {
			return fmt.Errorf("reload reordered tasks: %w", findErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return taskListResponse(reordered), nil
}

func (s *Service) list(userID, projectID int64, req *types.TaskListRequest, includeDeleted bool) (*types.TaskListResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if userID <= 0 || projectID <= 0 || req == nil || (req.Status != "" && !validTaskStatus(req.Status)) {
		return nil, logicerrors.ErrInvalidInput
	}
	project, err := findProject(s.ctx, s.svcCtx.DB, userID, projectID, includeDeleted)
	if err != nil {
		return nil, err
	}
	if !includeDeleted && project.Status == projectStatusDeleted {
		return nil, logicerrors.ErrNotFound
	}
	query := gorm.G[taskmodel.Task](s.svcCtx.DB).
		Where("project_id = ? AND user_id = ?", projectID, userID)
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}
	tasks, err := query.Order("sort_order ASC, id ASC").Find(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("list project tasks: %w", err)
	}
	return taskListResponse(tasks), nil
}

func findProject(ctx context.Context, db *gorm.DB, userID, projectID int64, includeDeleted bool) (projectmodel.Project, error) {
	query := gorm.G[projectmodel.Project](db).Where("id = ? AND user_id = ?", projectID, userID)
	if !includeDeleted {
		query = query.Where("status <> ?", projectStatusDeleted)
	}
	project, err := query.First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return project, logicerrors.ErrNotFound
	}
	if err != nil {
		return project, fmt.Errorf("get task project: %w", err)
	}
	return project, nil
}

func lockProject(ctx context.Context, tx *gorm.DB, userID, projectID int64, includeDeleted bool) (projectmodel.Project, error) {
	query := gorm.G[projectmodel.Project](tx, clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", projectID, userID)
	if !includeDeleted {
		query = query.Where("status <> ?", projectStatusDeleted)
	}
	project, err := query.First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return project, logicerrors.ErrNotFound
	}
	if err != nil {
		return project, fmt.Errorf("lock task project: %w", err)
	}
	return project, nil
}

func lockTask(ctx context.Context, tx *gorm.DB, userID, taskID int64) (taskmodel.Task, error) {
	task, err := gorm.G[taskmodel.Task](tx, clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND user_id = ?", taskID, userID).
		First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return task, logicerrors.ErrNotFound
	}
	if err != nil {
		return task, fmt.Errorf("lock task: %w", err)
	}
	return task, nil
}

func findTask(ctx context.Context, db *gorm.DB, userID, taskID int64) (taskmodel.Task, error) {
	task, err := gorm.G[taskmodel.Task](db).
		Where("id = ? AND user_id = ?", taskID, userID).
		First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return task, logicerrors.ErrNotFound
	}
	if err != nil {
		return task, fmt.Errorf("get task: %w", err)
	}
	return task, nil
}

func lockWritableTask(ctx context.Context, tx *gorm.DB, userID, taskID int64) (taskmodel.Task, error) {
	initial, err := findTask(ctx, tx, userID, taskID)
	if err != nil {
		return initial, err
	}
	project, err := lockProject(ctx, tx, userID, initial.ProjectID, false)
	if err != nil {
		return initial, err
	}
	if project.Status != projectStatusActive {
		return initial, logicerrors.ErrConflict
	}
	locked, err := lockTask(ctx, tx, userID, taskID)
	if err != nil {
		return locked, err
	}
	if locked.ProjectID != project.ID {
		return locked, logicerrors.ErrConflict
	}
	return locked, nil
}

func normalizeCreateRequest(userID, projectID int64, req *types.CreateTaskRequest) (*types.CreateTaskRequest, error) {
	if userID <= 0 || projectID <= 0 || req == nil {
		return nil, logicerrors.ErrInvalidInput
	}
	normalized := *req
	if err := normalizeTaskFields(&normalized.Title, &normalized.Description, normalized.Priority); err != nil {
		return nil, err
	}
	return &normalized, nil
}

func normalizeUpdateRequest(userID, taskID int64, req *types.UpdateTaskRequest) (*types.UpdateTaskRequest, error) {
	if userID <= 0 || taskID <= 0 || req == nil || req.Version < 1 {
		return nil, logicerrors.ErrInvalidInput
	}
	normalized := *req
	if err := normalizeTaskFields(&normalized.Title, &normalized.Description, normalized.Priority); err != nil {
		return nil, err
	}
	return &normalized, nil
}

func normalizeTaskFields(title *string, description **string, priority string) error {
	*title = strings.TrimSpace(*title)
	if *title == "" || utf8.RuneCountInString(*title) > 255 || !validTaskPriority(priority) {
		return logicerrors.ErrInvalidInput
	}
	if *description != nil {
		normalized := strings.TrimSpace(**description)
		if normalized == "" || utf8.RuneCountInString(normalized) > 2000 {
			return logicerrors.ErrInvalidInput
		}
		*description = &normalized
	}
	return nil
}

func validateReorderRequest(userID int64, req *types.ReorderTasksRequest) error {
	if userID <= 0 || req == nil || req.ProjectID <= 0 || req.TaskIDs == nil {
		return logicerrors.ErrInvalidInput
	}
	seen := make(map[int64]struct{}, len(req.TaskIDs))
	for _, taskID := range req.TaskIDs {
		if taskID <= 0 {
			return logicerrors.ErrInvalidInput
		}
		if _, exists := seen[taskID]; exists {
			return logicerrors.ErrInvalidInput
		}
		seen[taskID] = struct{}{}
	}
	return nil
}

func validTaskStatus(status string) bool {
	return status == "todo" || status == "doing" || status == "done"
}

func validTaskPriority(priority string) bool {
	return priority == "low" || priority == "medium" || priority == "high"
}

func taskListResponse(tasks []taskmodel.Task) *types.TaskListResponse {
	items := make([]types.TaskResponse, len(tasks))
	for index, task := range tasks {
		items[index] = taskResponse(task)
	}
	return &types.TaskListResponse{Items: items}
}

func taskResponse(task taskmodel.Task) types.TaskResponse {
	return types.TaskResponse{
		ID:                  task.ID,
		ProjectID:           task.ProjectID,
		SourceParseResultID: task.SourceParseResultID,
		Title:               task.Title,
		Description:         task.Description,
		Status:              task.Status,
		Priority:            task.Priority,
		Deadline:            task.Deadline,
		SortOrder:           task.SortOrder,
		SourceType:          task.SourceType,
		Version:             task.Version,
		CreatedAt:           task.CreatedAt,
		UpdatedAt:           task.UpdatedAt,
	}
}

func (s *Service) requireDB() error {
	if s.svcCtx.DB == nil {
		return logicerrors.ErrDatabaseUnavailable
	}
	return nil
}

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/model/parseresultmodel"
	"github.com/744223454/taskpilot-server/model/projectmodel"
	"github.com/744223454/taskpilot-server/model/taskmodel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxGeneratedTasks = 100

const (
	projectStatusActive   = "active"
	projectStatusArchived = "archived"
	projectStatusDeleted  = "deleted"
)

type Service struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewService(ctx context.Context, svcCtx *svc.ServiceContext) *Service {
	return &Service{ctx: ctx, svcCtx: svcCtx}
}

func (s *Service) Create(userID int64, req *types.CreateProjectRequest) (*types.CreateProjectResponse, bool, error) {
	name, err := normalizeRequest(userID, req)
	if err != nil {
		return nil, false, err
	}
	if s.svcCtx.DB == nil {
		return nil, false, logicerrors.ErrDatabaseUnavailable
	}

	var project projectmodel.Project
	var tasks []taskmodel.Task
	created := false
	err = s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		result, lockErr := gorm.G[parseresultmodel.ParseResult](tx, clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", req.ParseResultID, userID).
			First(s.ctx)
		if errors.Is(lockErr, gorm.ErrRecordNotFound) {
			return logicerrors.ErrNotFound
		}
		if lockErr != nil {
			return fmt.Errorf("lock parse result for project creation: %w", lockErr)
		}

		existing, existingTasks, findErr := findProjectWithTasks(s.ctx, tx, userID, result.ID)
		if findErr == nil {
			if existing.Status == projectStatusDeleted {
				return logicerrors.ErrConflict
			}
			project = existing
			tasks = existingTasks
			return nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if !result.IsConfirmed {
			return fmt.Errorf("%w: parse result is not confirmed", logicerrors.ErrInvalidState)
		}

		generatedTasks, decodeErr := decodeGeneratedTasks(result.GeneratedTasks)
		if decodeErr != nil {
			return decodeErr
		}
		description := result.Summary
		project = projectmodel.Project{
			UserID:           userID,
			SourceDocumentID: result.DocumentID,
			ParseResultID:    result.ID,
			Name:             name,
			Description:      &description,
			Deadline:         result.Deadline,
			Status:           projectStatusActive,
			Version:          1,
		}
		if createErr := gorm.G[projectmodel.Project](tx).Create(s.ctx, &project); createErr != nil {
			return fmt.Errorf("create project: %w", createErr)
		}

		tasks = make([]taskmodel.Task, len(generatedTasks))
		for index, generatedTask := range generatedTasks {
			parseResultID := result.ID
			tasks[index] = taskmodel.Task{
				ProjectID:           project.ID,
				UserID:              userID,
				SourceParseResultID: &parseResultID,
				Title:               generatedTask.Title,
				Description:         generatedTask.Description,
				Status:              "todo",
				Priority:            generatedTask.Priority,
				Deadline:            generatedTask.Deadline,
				SortOrder:           int32(index),
				SourceType:          "ai",
				Version:             1,
			}
		}
		if len(tasks) > 0 {
			if createErr := gorm.G[taskmodel.Task](tx).CreateInBatches(s.ctx, &tasks, maxGeneratedTasks); createErr != nil {
				return fmt.Errorf("create project tasks: %w", createErr)
			}
		}
		created = true
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			existing, existingTasks, findErr := findProjectWithTasks(s.ctx, s.svcCtx.DB, userID, req.ParseResultID)
			if findErr == nil {
				if existing.Status == projectStatusDeleted {
					return nil, false, logicerrors.ErrConflict
				}
				response := createProjectResponse(existing, existingTasks)
				return &response, false, nil
			}
		}
		return nil, false, err
	}

	response := createProjectResponse(project, tasks)
	return &response, created, nil
}

func (s *Service) List(userID int64, req *types.ProjectListRequest) (*types.ProjectListResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if userID <= 0 || req == nil || !validPagination(req.Page, req.PageSize) {
		return nil, logicerrors.ErrInvalidInput
	}
	page, pageSize := pagination(req.Page, req.PageSize)
	status := req.Status
	if status == "" {
		status = projectStatusActive
	}
	if status != projectStatusActive && status != projectStatusArchived {
		return nil, logicerrors.ErrInvalidInput
	}
	return s.list(userID, page, pageSize, status, false)
}

func (s *Service) HistoryList(userID int64, req *types.HistoryProjectListRequest) (*types.ProjectListResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if userID <= 0 || req == nil || !validPagination(req.Page, req.PageSize) {
		return nil, logicerrors.ErrInvalidInput
	}
	page, pageSize := pagination(req.Page, req.PageSize)
	if req.Status != "" && req.Status != projectStatusActive && req.Status != projectStatusArchived && req.Status != projectStatusDeleted {
		return nil, logicerrors.ErrInvalidInput
	}
	return s.list(userID, page, pageSize, req.Status, true)
}

func (s *Service) Get(userID, projectID int64) (*types.ProjectResponse, error) {
	return s.get(userID, projectID, false)
}

func (s *Service) HistoryGet(userID, projectID int64) (*types.ProjectResponse, error) {
	return s.get(userID, projectID, true)
}

func (s *Service) Update(userID, projectID int64, req *types.UpdateProjectRequest) (*types.ProjectResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	normalized, err := normalizeUpdateRequest(userID, projectID, req)
	if err != nil {
		return nil, err
	}

	var updated projectmodel.Project
	err = s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		current, lockErr := lockProject(s.ctx, tx, userID, projectID, false)
		if lockErr != nil {
			return lockErr
		}
		if current.Status != projectStatusActive {
			return logicerrors.ErrConflict
		}
		if current.Version != normalized.Version {
			return logicerrors.ErrConflict
		}

		now := time.Now()
		rowsAffected, updateErr := gorm.G[projectmodel.Project](tx).
			Where("id = ? AND user_id = ? AND status = ? AND version = ?", projectID, userID, projectStatusActive, current.Version).
			Set(clause.Assignments(map[string]any{
				"name":        normalized.Name,
				"description": normalized.Description,
				"deadline":    normalized.Deadline,
				"version":     current.Version + 1,
				"updated_at":  now,
			})).
			Update(s.ctx)
		if updateErr != nil {
			return fmt.Errorf("update project: %w", updateErr)
		}
		if rowsAffected != 1 {
			return logicerrors.ErrConflict
		}
		updated, updateErr = gorm.G[projectmodel.Project](tx).
			Where("id = ? AND user_id = ?", projectID, userID).
			First(s.ctx)
		if updateErr != nil {
			return fmt.Errorf("reload updated project: %w", updateErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	response := projectResponse(updated)
	return &response, nil
}

func (s *Service) Archive(userID, projectID int64) (*types.ProjectResponse, error) {
	return s.transition(userID, projectID, projectStatusActive, projectStatusArchived, projectStatusArchived)
}

func (s *Service) Unarchive(userID, projectID int64) (*types.ProjectResponse, error) {
	return s.transition(userID, projectID, projectStatusArchived, projectStatusActive, projectStatusActive)
}

func (s *Service) Delete(userID, projectID int64) error {
	if err := s.requireDB(); err != nil {
		return err
	}
	if userID <= 0 || projectID <= 0 {
		return logicerrors.ErrInvalidInput
	}
	return s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockProject(s.ctx, tx, userID, projectID, false)
		if err != nil {
			return err
		}
		if current.Status == projectStatusActive {
			return logicerrors.ErrConflict
		}
		if current.Status != projectStatusArchived {
			return logicerrors.ErrNotFound
		}
		rowsAffected, err := gorm.G[projectmodel.Project](tx).
			Where("id = ? AND user_id = ? AND status = ?", projectID, userID, projectStatusArchived).
			Set(clause.Assignments(map[string]any{
				"status":     projectStatusDeleted,
				"version":    current.Version + 1,
				"updated_at": time.Now(),
			})).
			Update(s.ctx)
		if err != nil {
			return fmt.Errorf("delete project: %w", err)
		}
		if rowsAffected != 1 {
			return logicerrors.ErrConflict
		}
		return nil
	})
}

func (s *Service) transition(userID, projectID int64, sourceStatus, targetStatus, idempotentStatus string) (*types.ProjectResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if userID <= 0 || projectID <= 0 {
		return nil, logicerrors.ErrInvalidInput
	}
	var project projectmodel.Project
	err := s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockProject(s.ctx, tx, userID, projectID, false)
		if err != nil {
			return err
		}
		if current.Status == idempotentStatus {
			project = current
			return nil
		}
		if current.Status != sourceStatus {
			return logicerrors.ErrConflict
		}
		rowsAffected, err := gorm.G[projectmodel.Project](tx).
			Where("id = ? AND user_id = ? AND status = ?", projectID, userID, sourceStatus).
			Set(clause.Assignments(map[string]any{
				"status":     targetStatus,
				"version":    current.Version + 1,
				"updated_at": time.Now(),
			})).
			Update(s.ctx)
		if err != nil {
			return fmt.Errorf("transition project status: %w", err)
		}
		if rowsAffected != 1 {
			return logicerrors.ErrConflict
		}
		project, err = gorm.G[projectmodel.Project](tx).
			Where("id = ? AND user_id = ?", projectID, userID).
			First(s.ctx)
		if err != nil {
			return fmt.Errorf("reload transitioned project: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	response := projectResponse(project)
	return &response, nil
}

func (s *Service) get(userID, projectID int64, includeDeleted bool) (*types.ProjectResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if userID <= 0 || projectID <= 0 {
		return nil, logicerrors.ErrInvalidInput
	}
	query := gorm.G[projectmodel.Project](s.svcCtx.DB).Where("id = ? AND user_id = ?", projectID, userID)
	if !includeDeleted {
		query = query.Where("status <> ?", projectStatusDeleted)
	}
	project, err := query.First(s.ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, logicerrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	response := projectResponse(project)
	return &response, nil
}

func (s *Service) list(userID int64, page, pageSize int, status string, includeDeleted bool) (*types.ProjectListResponse, error) {
	query := gorm.G[projectmodel.Project](s.svcCtx.DB).Where("user_id = ?", userID)
	if status != "" {
		query = query.Where("status = ?", status)
	} else if !includeDeleted {
		query = query.Where("status <> ?", projectStatusDeleted)
	}
	total, err := query.Count(s.ctx, "id")
	if err != nil {
		return nil, fmt.Errorf("count projects: %w", err)
	}
	projects, err := query.
		Order("updated_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	items := make([]types.ProjectResponse, len(projects))
	for index, project := range projects {
		items[index] = projectResponse(project)
	}
	return &types.ProjectListResponse{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
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
		return project, fmt.Errorf("lock project: %w", err)
	}
	return project, nil
}

func normalizeUpdateRequest(userID, projectID int64, req *types.UpdateProjectRequest) (*types.UpdateProjectRequest, error) {
	if userID <= 0 || projectID <= 0 || req == nil || req.Version < 1 {
		return nil, logicerrors.ErrInvalidInput
	}
	normalized := *req
	normalized.Name = strings.TrimSpace(req.Name)
	if normalized.Name == "" || utf8.RuneCountInString(normalized.Name) > 255 {
		return nil, logicerrors.ErrInvalidInput
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if description == "" || utf8.RuneCountInString(description) > 5000 {
			return nil, logicerrors.ErrInvalidInput
		}
		normalized.Description = &description
	}
	return &normalized, nil
}

func pagination(page, pageSize int) (int, int) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 10
	}
	return page, pageSize
}

func validPagination(page, pageSize int) bool {
	return page >= 0 && page <= 1000000 && pageSize >= 0 && pageSize <= 100
}

func (s *Service) requireDB() error {
	if s.svcCtx.DB == nil {
		return logicerrors.ErrDatabaseUnavailable
	}
	return nil
}

func normalizeRequest(userID int64, req *types.CreateProjectRequest) (string, error) {
	if userID <= 0 || req == nil || req.ParseResultID <= 0 {
		return "", logicerrors.ErrInvalidInput
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > 255 {
		return "", logicerrors.ErrInvalidInput
	}
	return name, nil
}

func decodeGeneratedTasks(raw json.RawMessage) ([]types.GeneratedTask, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var tasks []types.GeneratedTask
	if err := decoder.Decode(&tasks); err != nil {
		return nil, fmt.Errorf("%w: decode generated tasks", logicerrors.ErrInvalidState)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: generated tasks contain trailing data", logicerrors.ErrInvalidState)
	}
	if tasks == nil || len(tasks) > maxGeneratedTasks {
		return nil, fmt.Errorf("%w: generated tasks must be an array with at most %d items", logicerrors.ErrInvalidState, maxGeneratedTasks)
	}

	normalized := make([]types.GeneratedTask, len(tasks))
	for index, original := range tasks {
		task := original
		task.Title = strings.TrimSpace(task.Title)
		if task.Title == "" || utf8.RuneCountInString(task.Title) > 255 {
			return nil, fmt.Errorf("%w: generated task title is invalid", logicerrors.ErrInvalidState)
		}
		if task.Description != nil {
			description := strings.TrimSpace(*task.Description)
			if description == "" || utf8.RuneCountInString(description) > 2000 {
				return nil, fmt.Errorf("%w: generated task description is invalid", logicerrors.ErrInvalidState)
			}
			task.Description = &description
		}
		if task.Priority == "" {
			task.Priority = "medium"
		}
		if task.Priority != "low" && task.Priority != "medium" && task.Priority != "high" {
			return nil, fmt.Errorf("%w: generated task priority is invalid", logicerrors.ErrInvalidState)
		}
		normalized[index] = task
	}
	return normalized, nil
}

func findProjectWithTasks(ctx context.Context, db *gorm.DB, userID, parseResultID int64) (projectmodel.Project, []taskmodel.Task, error) {
	project, err := gorm.G[projectmodel.Project](db).
		Where("parse_result_id = ? AND user_id = ?", parseResultID, userID).
		First(ctx)
	if err != nil {
		return project, nil, err
	}
	tasks, err := gorm.G[taskmodel.Task](db).
		Where("project_id = ? AND user_id = ?", project.ID, userID).
		Order("sort_order ASC, id ASC").
		Find(ctx)
	if err != nil {
		return project, nil, fmt.Errorf("list existing project tasks: %w", err)
	}
	return project, tasks, nil
}

func createProjectResponse(project projectmodel.Project, tasks []taskmodel.Task) types.CreateProjectResponse {
	response := types.CreateProjectResponse{
		Project: projectResponse(project),
		Tasks:   make([]types.TaskResponse, len(tasks)),
	}
	for index, task := range tasks {
		response.Tasks[index] = types.TaskResponse{
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
	return response
}

func projectResponse(project projectmodel.Project) types.ProjectResponse {
	return types.ProjectResponse{
		ID:               project.ID,
		SourceDocumentID: project.SourceDocumentID,
		ParseResultID:    project.ParseResultID,
		Name:             project.Name,
		Description:      project.Description,
		Deadline:         project.Deadline,
		Status:           project.Status,
		Version:          project.Version,
		CreatedAt:        project.CreatedAt,
		UpdatedAt:        project.UpdatedAt,
	}
}

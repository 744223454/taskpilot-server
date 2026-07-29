package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
			Status:           "active",
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
				response := createProjectResponse(existing, existingTasks)
				return &response, false, nil
			}
		}
		return nil, false, err
	}

	response := createProjectResponse(project, tasks)
	return &response, created, nil
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
		Project: types.ProjectResponse{
			ID:               project.ID,
			SourceDocumentID: project.SourceDocumentID,
			ParseResultID:    project.ParseResultID,
			Name:             project.Name,
			Description:      project.Description,
			Deadline:         project.Deadline,
			Status:           project.Status,
			CreatedAt:        project.CreatedAt,
			UpdatedAt:        project.UpdatedAt,
		},
		Tasks: make([]types.TaskResponse, len(tasks)),
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
			CreatedAt:           task.CreatedAt,
			UpdatedAt:           task.UpdatedAt,
		}
	}
	return response
}

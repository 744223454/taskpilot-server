package parseresult

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/model/parseresultmodel"
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

func (s *Service) GetByJob(userID, jobID int64) (*types.ParseResultResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	result, err := gorm.G[parseresultmodel.ParseResult](s.svcCtx.DB).
		Where("parse_job_id = ? AND user_id = ?", jobID, userID).
		First(s.ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, logicerrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get parse result by job: %w", err)
	}
	return parseResultResponse(result)
}

func (s *Service) Get(userID, resultID int64) (*types.ParseResultResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	result, err := s.find(userID, resultID)
	if err != nil {
		return nil, err
	}
	return parseResultResponse(result)
}

func (s *Service) HistoryList(userID int64, req *types.ParseResultHistoryListRequest) (*types.ParseResultHistoryListResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if userID <= 0 || req == nil || req.Page < 0 || req.Page > 1000000 || req.PageSize < 0 || req.PageSize > 100 {
		return nil, logicerrors.ErrInvalidInput
	}
	page := req.Page
	if page == 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = 10
	}
	total, err := gorm.G[parseresultmodel.ParseResult](s.svcCtx.DB).
		Where("user_id = ?", userID).
		Count(s.ctx, "id")
	if err != nil {
		return nil, fmt.Errorf("count parse result history: %w", err)
	}
	results, err := gorm.G[parseresultmodel.ParseResult](s.svcCtx.DB).
		Where("user_id = ?", userID).
		Order("updated_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("list parse result history: %w", err)
	}
	items := make([]types.ParseResultResponse, len(results))
	for index, result := range results {
		response, responseErr := parseResultResponse(result)
		if responseErr != nil {
			return nil, fmt.Errorf("build parse result history response: %w", responseErr)
		}
		items[index] = *response
	}
	return &types.ParseResultHistoryListResponse{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) Update(userID, resultID int64, req *types.UpdateParseResultRequest) (*types.ParseResultResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	normalized, err := normalizeUpdate(req)
	if err != nil {
		return nil, err
	}

	var updated parseresultmodel.ParseResult
	err = s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		current, err := gorm.G[parseresultmodel.ParseResult](tx, clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", resultID, userID).
			First(s.ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return logicerrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock parse result for update: %w", err)
		}
		if current.IsConfirmed {
			return logicerrors.ErrInvalidState
		}
		if current.Version != normalized.Version {
			return logicerrors.ErrConflict
		}

		deliverables, _ := json.Marshal(normalized.Deliverables)
		keyRequirements, _ := json.Marshal(normalized.KeyRequirements)
		riskWarnings, _ := json.Marshal(normalized.RiskWarnings)
		generatedTasks, _ := json.Marshal(normalized.GeneratedTasks)
		now := time.Now()
		rowsAffected, err := gorm.G[parseresultmodel.ParseResult](tx).
			Where("id = ? AND user_id = ? AND version = ? AND is_confirmed = ?", resultID, userID, current.Version, false).
			Set(clause.Assignments(map[string]any{
				"title":            normalized.Title,
				"summary":          normalized.Summary,
				"deadline":         normalized.Deadline,
				"deliverables":     deliverables,
				"key_requirements": keyRequirements,
				"risk_warnings":    riskWarnings,
				"generated_tasks":  generatedTasks,
				"version":          current.Version + 1,
				"updated_at":       now,
			})).
			Update(s.ctx)
		if err != nil {
			return fmt.Errorf("update parse result: %w", err)
		}
		if rowsAffected != 1 {
			return logicerrors.ErrConflict
		}
		updated, err = gorm.G[parseresultmodel.ParseResult](tx).
			Where("id = ? AND user_id = ?", resultID, userID).
			First(s.ctx)
		if err != nil {
			return fmt.Errorf("reload updated parse result: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return parseResultResponse(updated)
}

func (s *Service) Confirm(userID, resultID int64) (*types.ParseResultResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	var confirmed parseresultmodel.ParseResult
	err := s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		current, err := gorm.G[parseresultmodel.ParseResult](tx, clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", resultID, userID).
			First(s.ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return logicerrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock parse result for confirmation: %w", err)
		}
		if current.IsConfirmed {
			confirmed = current
			return nil
		}

		now := time.Now()
		rowsAffected, err := gorm.G[parseresultmodel.ParseResult](tx).
			Where("id = ? AND user_id = ? AND is_confirmed = ?", resultID, userID, false).
			Set(clause.Assignments(map[string]any{
				"is_confirmed": true,
				"updated_at":   now,
			})).
			Update(s.ctx)
		if err != nil {
			return fmt.Errorf("confirm parse result: %w", err)
		}
		if rowsAffected != 1 {
			return logicerrors.ErrConflict
		}
		confirmed, err = gorm.G[parseresultmodel.ParseResult](tx).
			Where("id = ? AND user_id = ?", resultID, userID).
			First(s.ctx)
		if err != nil {
			return fmt.Errorf("reload confirmed parse result: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return parseResultResponse(confirmed)
}

func (s *Service) find(userID, resultID int64) (parseresultmodel.ParseResult, error) {
	result, err := gorm.G[parseresultmodel.ParseResult](s.svcCtx.DB).
		Where("id = ? AND user_id = ?", resultID, userID).
		First(s.ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, logicerrors.ErrNotFound
	}
	if err != nil {
		return result, fmt.Errorf("get parse result: %w", err)
	}
	return result, nil
}

func (s *Service) requireDB() error {
	if s.svcCtx.DB == nil {
		return logicerrors.ErrDatabaseUnavailable
	}
	return nil
}

func normalizeUpdate(req *types.UpdateParseResultRequest) (*types.UpdateParseResultRequest, error) {
	if req == nil || req.Version < 1 {
		return nil, logicerrors.ErrInvalidInput
	}
	normalized := *req
	normalized.Title = strings.TrimSpace(req.Title)
	normalized.Summary = strings.TrimSpace(req.Summary)
	if normalized.Title == "" || utf8.RuneCountInString(normalized.Title) > 255 || normalized.Summary == "" || utf8.RuneCountInString(normalized.Summary) > 5000 {
		return nil, logicerrors.ErrInvalidInput
	}
	var err error
	normalized.Deliverables, err = normalizeStrings(req.Deliverables, 50, 1000)
	if err != nil {
		return nil, logicerrors.ErrInvalidInput
	}
	normalized.KeyRequirements, err = normalizeStrings(req.KeyRequirements, 100, 1000)
	if err != nil {
		return nil, logicerrors.ErrInvalidInput
	}
	normalized.RiskWarnings, err = normalizeStrings(req.RiskWarnings, 50, 1000)
	if err != nil {
		return nil, logicerrors.ErrInvalidInput
	}
	if req.GeneratedTasks == nil || len(req.GeneratedTasks) > 100 {
		return nil, logicerrors.ErrInvalidInput
	}
	normalized.GeneratedTasks = make([]types.GeneratedTask, len(req.GeneratedTasks))
	for index, original := range req.GeneratedTasks {
		task := original
		task.Title = strings.TrimSpace(task.Title)
		if task.Title == "" || utf8.RuneCountInString(task.Title) > 255 {
			return nil, logicerrors.ErrInvalidInput
		}
		if task.Description != nil {
			description := strings.TrimSpace(*task.Description)
			if description == "" || utf8.RuneCountInString(description) > 2000 {
				return nil, logicerrors.ErrInvalidInput
			}
			task.Description = &description
		}
		if task.Priority == "" {
			task.Priority = "medium"
		}
		if task.Priority != "low" && task.Priority != "medium" && task.Priority != "high" {
			return nil, logicerrors.ErrInvalidInput
		}
		normalized.GeneratedTasks[index] = task
	}
	return &normalized, nil
}

func normalizeStrings(values []string, maxItems, maxRunes int) ([]string, error) {
	if values == nil || len(values) > maxItems {
		return nil, logicerrors.ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || utf8.RuneCountInString(value) > maxRunes {
			return nil, logicerrors.ErrInvalidInput
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func parseResultResponse(result parseresultmodel.ParseResult) (*types.ParseResultResponse, error) {
	response := &types.ParseResultResponse{
		ID:          result.ID,
		DocumentID:  result.DocumentID,
		ParseJobID:  result.ParseJobID,
		Title:       result.Title,
		Summary:     result.Summary,
		Deadline:    result.Deadline,
		AIModel:     result.AIModel,
		Version:     result.Version,
		IsConfirmed: result.IsConfirmed,
		CreatedAt:   result.CreatedAt,
		UpdatedAt:   result.UpdatedAt,
	}
	if err := json.Unmarshal(result.Deliverables, &response.Deliverables); err != nil {
		return nil, fmt.Errorf("decode parse result deliverables: %w", err)
	}
	if err := json.Unmarshal(result.KeyRequirements, &response.KeyRequirements); err != nil {
		return nil, fmt.Errorf("decode parse result requirements: %w", err)
	}
	if err := json.Unmarshal(result.RiskWarnings, &response.RiskWarnings); err != nil {
		return nil, fmt.Errorf("decode parse result risks: %w", err)
	}
	if err := json.Unmarshal(result.GeneratedTasks, &response.GeneratedTasks); err != nil {
		return nil, fmt.Errorf("decode parse result tasks: %w", err)
	}
	if response.Deliverables == nil {
		response.Deliverables = []string{}
	}
	if response.KeyRequirements == nil {
		response.KeyRequirements = []string{}
	}
	if response.RiskWarnings == nil {
		response.RiskWarnings = []string{}
	}
	if response.GeneratedTasks == nil {
		response.GeneratedTasks = []types.GeneratedTask{}
	}
	return response, nil
}

package document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

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

func (s *Service) CreateText(userID int64, req *types.CreateTextDocumentRequest) (*types.DocumentResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	title := strings.TrimSpace(req.Title)
	text := strings.TrimSpace(req.Text)
	if title == "" || text == "" || utf8.RuneCountInString(text) > types.MaxTextDocumentChars {
		return nil, logicerrors.ErrInvalidInput
	}

	document := documentmodel.Document{
		UserID:     userID,
		SourceType: "text",
		Title:      &title,
		RawText:    &text,
		TextInput:  &text,
		Status:     "ready",
	}
	if err := gorm.G[documentmodel.Document](s.svcCtx.DB).Create(s.ctx, &document); err != nil {
		return nil, fmt.Errorf("create text document: %w", err)
	}

	response := documentResponse(document)
	return &response, nil
}

func (s *Service) Get(userID, documentID int64) (*types.DocumentResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	document, err := gorm.G[documentmodel.Document](s.svcCtx.DB).
		Where("id = ? AND user_id = ?", documentID, userID).
		First(s.ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, logicerrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}

	response := documentResponse(document)
	return &response, nil
}

func (s *Service) List(userID int64, req *types.DocumentListRequest) (*types.DocumentListResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}

	page := req.Page
	if page == 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = 10
	}

	total, err := gorm.G[documentmodel.Document](s.svcCtx.DB).
		Where("user_id = ?", userID).
		Count(s.ctx, "id")
	if err != nil {
		return nil, fmt.Errorf("count documents: %w", err)
	}

	documents, err := gorm.G[documentmodel.Document](s.svcCtx.DB).
		Where("user_id = ?", userID).
		Omit("raw_text", "text_input").
		Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}

	items := make([]types.DocumentSummaryResponse, 0, len(documents))
	for _, document := range documents {
		items = append(items, documentSummaryResponse(document))
	}

	return &types.DocumentListResponse{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func (s *Service) Delete(userID, documentID int64) error {
	if err := s.requireDB(); err != nil {
		return err
	}

	return s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		_, err := gorm.G[documentmodel.Document](tx, clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", documentID, userID).
			First(s.ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return logicerrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock document for deletion: %w", err)
		}

		activeJobs, err := gorm.G[parsejobmodel.ParseJob](tx).
			Where("document_id = ? AND user_id = ? AND status IN ?", documentID, userID, []string{"pending", "processing"}).
			Count(s.ctx, "id")
		if err != nil {
			return fmt.Errorf("count active parse jobs before document deletion: %w", err)
		}
		if activeJobs > 0 {
			return logicerrors.ErrConflict
		}

		rowsAffected, err := gorm.G[documentmodel.Document](tx).
			Where("id = ? AND user_id = ?", documentID, userID).
			Delete(s.ctx)
		if err != nil {
			return fmt.Errorf("soft delete document: %w", err)
		}
		if rowsAffected == 0 {
			return logicerrors.ErrNotFound
		}
		return nil
	})
}

func (s *Service) requireDB() error {
	if s.svcCtx.DB == nil {
		return logicerrors.ErrDatabaseUnavailable
	}
	return nil
}

func documentResponse(document documentmodel.Document) types.DocumentResponse {
	content := document.TextInput
	if content == nil {
		content = document.RawText
	}
	return types.DocumentResponse{
		DocumentSummaryResponse: documentSummaryResponse(document),
		Content:                 content,
	}
}

func documentSummaryResponse(document documentmodel.Document) types.DocumentSummaryResponse {
	return types.DocumentSummaryResponse{
		ID:         document.ID,
		SourceType: document.SourceType,
		Title:      document.Title,
		FileName:   document.FileName,
		FileURL:    document.FileURL,
		PageCount:  document.PageCount,
		FileSize:   document.FileSize,
		Status:     document.Status,
		CreatedAt:  document.CreatedAt,
		UpdatedAt:  document.UpdatedAt,
	}
}

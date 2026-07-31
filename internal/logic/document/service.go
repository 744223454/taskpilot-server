package document

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/model/documentmodel"
	"github.com/744223454/taskpilot-server/model/parsejobmodel"
	"github.com/744223454/taskpilot-server/pkg/upload"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func (s *Service) CreatePDF(userID int64, req *types.CreatePDFDocumentRequest, reader io.Reader) (*types.DocumentResponse, error) {
	if err := s.requireDB(); err != nil {
		return nil, err
	}
	if s.svcCtx.Files == nil || s.svcCtx.PDFExtractor == nil {
		return nil, errors.New("PDF upload service is not configured")
	}

	title := strings.TrimSpace(req.Title)
	fileName := sanitizedFileName(req.FileName)
	if title == "" {
		title = strings.TrimSpace(fileName)
		if strings.EqualFold(filepath.Ext(title), ".pdf") {
			title = strings.TrimSpace(title[:len(title)-len(filepath.Ext(title))])
		}
	}
	if title == "" || utf8.RuneCountInString(title) > 255 || fileName == "" {
		return nil, logicerrors.ErrInvalidInput
	}

	maxFileBytes := s.svcCtx.Config.Upload.MaxFileBytes
	if maxFileBytes == 0 {
		maxFileBytes = 10 << 20
	}
	temporary, err := s.svcCtx.Files.SaveTemp(s.ctx, reader, maxFileBytes)
	if errors.Is(err, upload.ErrFileTooLarge) {
		return nil, logicerrors.ErrPayloadTooLarge
	}
	if err != nil {
		return nil, fmt.Errorf("save temporary PDF: %w", err)
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			if deleteErr := s.svcCtx.Files.Delete(context.Background(), temporary.Key); deleteErr != nil {
				s.logger().Error("temporary PDF cleanup failed", "storage_key", temporary.Key, "error", deleteErr)
			}
		}
	}()

	if err := validatePDFHeader(temporary.LocalPath); err != nil {
		return nil, err
	}
	extracted, err := s.svcCtx.PDFExtractor.Extract(s.ctx, temporary.LocalPath)
	if err != nil {
		return nil, mapExtractionError(err)
	}

	finalKey, err := upload.DocumentKey(userID, time.Now())
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Files.Promote(s.ctx, temporary.Key, finalKey); err != nil {
		return nil, fmt.Errorf("save final PDF: %w", err)
	}
	removeTemporary = false
	removeFinal := true
	defer func() {
		if removeFinal {
			if deleteErr := s.svcCtx.Files.Delete(context.Background(), finalKey); deleteErr != nil {
				s.logger().Error("final PDF rollback cleanup failed", "storage_key", finalKey, "error", deleteErr)
			}
		}
	}()

	pageCount := extracted.PageCount
	fileSize := temporary.Size
	document := documentmodel.Document{
		UserID: userID, SourceType: "pdf", Title: &title, FileName: &fileName, FileURL: &finalKey,
		RawText: &extracted.Text, PageCount: &pageCount, FileSize: &fileSize, Status: "ready",
	}
	if err := gorm.G[documentmodel.Document](s.svcCtx.DB).Create(s.ctx, &document); err != nil {
		return nil, fmt.Errorf("create PDF document: %w", err)
	}
	removeFinal = false
	response := documentResponse(document)
	return &response, nil
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

	var fileKey string
	err := s.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		document, err := gorm.G[documentmodel.Document](tx, clause.Locking{Strength: "UPDATE"}).
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
		if document.SourceType == "pdf" && document.FileURL != nil {
			fileKey = *document.FileURL
		}
		return nil
	})
	if err != nil {
		return err
	}
	if fileKey != "" && s.svcCtx.Files != nil {
		if err := s.svcCtx.Files.Delete(context.Background(), fileKey); err != nil {
			s.logger().Error("deleted document PDF cleanup failed", "document_id", documentID, "user_id", userID, "storage_key", fileKey, "error", err)
		}
	}
	return nil
}

func (s *Service) requireDB() error {
	if s.svcCtx.DB == nil {
		return logicerrors.ErrDatabaseUnavailable
	}
	return nil
}

func (s *Service) logger() *slog.Logger {
	if s.svcCtx.Logger != nil {
		return s.svcCtx.Logger
	}
	return slog.Default()
}

func sanitizedFileName(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(value)
	value = strings.Map(func(character rune) rune {
		if character < 32 || character == 127 {
			return -1
		}
		return character
	}, value)
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > 255 {
		runes := []rune(value)
		value = string(runes[:255])
	}
	return value
}

func validatePDFHeader(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open temporary PDF: %w", err)
	}
	defer file.Close()
	header := make([]byte, 5)
	if _, err := io.ReadFull(file, header); err != nil || string(header) != "%PDF-" {
		return logicerrors.ErrUnsupportedFileType
	}
	return nil
}

func mapExtractionError(err error) error {
	switch {
	case errors.Is(err, upload.ErrPDFExtractorBusy):
		return logicerrors.ErrExtractionBusy
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, upload.ErrPDFInvalid), errors.Is(err, upload.ErrPDFEncrypted),
		errors.Is(err, upload.ErrPDFTooManyPages), errors.Is(err, upload.ErrPDFTextTooLarge),
		errors.Is(err, upload.ErrPDFNoUsableText), errors.Is(err, upload.ErrPDFExtractTimeout):
		return logicerrors.ErrPDFUnprocessable
	default:
		return fmt.Errorf("extract PDF text: %w", err)
	}
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

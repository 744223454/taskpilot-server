package types

import "time"

const MaxTextDocumentChars = 50000

type CreateTextDocumentRequest struct {
	Title string `json:"title" binding:"required,max=255"`
	Text  string `json:"text" binding:"required,max=50000"`
}

type DocumentListRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1,max=1000000"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

type DocumentSummaryResponse struct {
	ID         int64     `json:"id"`
	SourceType string    `json:"source_type"`
	Title      *string   `json:"title,omitempty"`
	FileName   *string   `json:"file_name,omitempty"`
	FileURL    *string   `json:"file_url,omitempty"`
	PageCount  *int32    `json:"page_count,omitempty"`
	FileSize   *int64    `json:"file_size,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type DocumentResponse struct {
	DocumentSummaryResponse
	Content *string `json:"content,omitempty"`
}

type DocumentListResponse struct {
	Items    []DocumentSummaryResponse `json:"items"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
	Total    int64                     `json:"total"`
}

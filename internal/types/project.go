package types

import "time"

type CreateProjectRequest struct {
	ParseResultID int64  `json:"parse_result_id" binding:"required,min=1"`
	Name          string `json:"name" binding:"required,max=255"`
}

type ProjectListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1,max=1000000"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Status   string `form:"status" binding:"omitempty,oneof=active archived"`
}

type HistoryProjectListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1,max=1000000"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
	Status   string `form:"status" binding:"omitempty,oneof=active archived deleted"`
}

type UpdateProjectRequest struct {
	Version     int32      `json:"version" binding:"required,min=1"`
	Name        string     `json:"name" binding:"required,max=255"`
	Description *string    `json:"description" binding:"omitempty,max=5000"`
	Deadline    *time.Time `json:"deadline"`
}

type ProjectResponse struct {
	ID               int64      `json:"id"`
	SourceDocumentID int64      `json:"source_document_id"`
	ParseResultID    int64      `json:"parse_result_id"`
	Name             string     `json:"name"`
	Description      *string    `json:"description"`
	Deadline         *time.Time `json:"deadline"`
	Status           string     `json:"status"`
	Version          int32      `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type ProjectListResponse struct {
	Items    []ProjectResponse `json:"items"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int64             `json:"total"`
}

type TaskResponse struct {
	ID                  int64      `json:"id"`
	ProjectID           int64      `json:"project_id"`
	SourceParseResultID *int64     `json:"source_parse_result_id"`
	Title               string     `json:"title"`
	Description         *string    `json:"description"`
	Status              string     `json:"status"`
	Priority            string     `json:"priority"`
	Deadline            *time.Time `json:"deadline"`
	SortOrder           int32      `json:"sort_order"`
	SourceType          string     `json:"source_type"`
	Version             int32      `json:"version"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type CreateProjectResponse struct {
	Project ProjectResponse `json:"project"`
	Tasks   []TaskResponse  `json:"tasks"`
}

package types

import "time"

type CreateProjectRequest struct {
	ParseResultID int64  `json:"parse_result_id" binding:"required,min=1"`
	Name          string `json:"name" binding:"required,max=255"`
}

type ProjectResponse struct {
	ID               int64      `json:"id"`
	SourceDocumentID int64      `json:"source_document_id"`
	ParseResultID    int64      `json:"parse_result_id"`
	Name             string     `json:"name"`
	Description      *string    `json:"description"`
	Deadline         *time.Time `json:"deadline"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type CreateProjectResponse struct {
	Project ProjectResponse `json:"project"`
	Tasks   []TaskResponse  `json:"tasks"`
}

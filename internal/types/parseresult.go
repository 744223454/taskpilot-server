package types

import "time"

type GeneratedTask struct {
	Title       string     `json:"title" binding:"required,max=255"`
	Description *string    `json:"description"`
	Priority    string     `json:"priority" binding:"omitempty,oneof=low medium high"`
	Deadline    *time.Time `json:"deadline"`
}

type ParseResultHistoryListRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1,max=1000000"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

type UpdateParseResultRequest struct {
	Version         int32           `json:"version" binding:"required,min=1"`
	Title           string          `json:"title" binding:"required,max=255"`
	Summary         string          `json:"summary" binding:"required,max=5000"`
	Deadline        *time.Time      `json:"deadline"`
	Deliverables    []string        `json:"deliverables" binding:"required,max=50,dive,required,max=1000"`
	KeyRequirements []string        `json:"key_requirements" binding:"required,max=100,dive,required,max=1000"`
	RiskWarnings    []string        `json:"risk_warnings" binding:"required,max=50,dive,required,max=1000"`
	GeneratedTasks  []GeneratedTask `json:"generated_tasks" binding:"required,max=100,dive"`
}

type ParseResultResponse struct {
	ID              int64           `json:"id"`
	DocumentID      int64           `json:"document_id"`
	ParseJobID      int64           `json:"parse_job_id"`
	Title           string          `json:"title"`
	Summary         string          `json:"summary"`
	Deadline        *time.Time      `json:"deadline"`
	Deliverables    []string        `json:"deliverables"`
	KeyRequirements []string        `json:"key_requirements"`
	RiskWarnings    []string        `json:"risk_warnings"`
	GeneratedTasks  []GeneratedTask `json:"generated_tasks"`
	AIModel         *string         `json:"ai_model,omitempty"`
	Version         int32           `json:"version"`
	IsConfirmed     bool            `json:"is_confirmed"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type ParseResultHistoryListResponse struct {
	Items    []ParseResultResponse `json:"items"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Total    int64                 `json:"total"`
}

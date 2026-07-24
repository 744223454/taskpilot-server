package types

import "time"

type CreateParseJobRequest struct {
	DocumentID int64 `json:"document_id" binding:"required,gt=0"`
}

type ParseJobResponse struct {
	ID           int64      `json:"id"`
	DocumentID   int64      `json:"document_id"`
	JobType      string     `json:"job_type"`
	Status       string     `json:"status"`
	RetryCount   int32      `json:"retry_count"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

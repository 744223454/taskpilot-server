package parsejobmodel

import "time"

type ParseJob struct {
	ID           int64      `gorm:"primaryKey;autoIncrement"`
	UserID       int64      `gorm:"column:user_id;not null;index"`
	DocumentID   int64      `gorm:"column:document_id;not null;index"`
	JobType      string     `gorm:"column:job_type;type:varchar(30);not null"`
	Status       string     `gorm:"type:varchar(20);not null"`
	RetryCount   int32      `gorm:"column:retry_count;not null;default:0"`
	ErrorMessage *string    `gorm:"column:error_message;type:text"`
	StartedAt    *time.Time `gorm:"column:started_at"`
	FinishedAt   *time.Time `gorm:"column:finished_at"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

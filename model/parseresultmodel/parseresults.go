package parseresultmodel

import (
	"encoding/json"
	"time"
)

type ParseResult struct {
	ID              int64           `gorm:"primaryKey;autoIncrement"`
	UserID          int64           `gorm:"column:user_id;not null;index"`
	DocumentID      int64           `gorm:"column:document_id;not null;index"`
	ParseJobID      int64           `gorm:"column:parse_job_id;not null;uniqueIndex"`
	Title           string          `gorm:"type:varchar(255);not null"`
	Summary         string          `gorm:"type:text;not null"`
	Deadline        *time.Time      `gorm:"column:deadline"`
	Deliverables    json.RawMessage `gorm:"column:deliverables;type:jsonb;not null"`
	KeyRequirements json.RawMessage `gorm:"column:key_requirements;type:jsonb;not null"`
	RiskWarnings    json.RawMessage `gorm:"column:risk_warnings;type:jsonb;not null"`
	GeneratedTasks  json.RawMessage `gorm:"column:generated_tasks;type:jsonb;not null"`
	AIModel         *string         `gorm:"column:ai_model;type:varchar(64)"`
	Version         int32           `gorm:"column:version;not null;default:1"`
	IsConfirmed     bool            `gorm:"column:is_confirmed;not null;default:false"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

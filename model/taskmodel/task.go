package taskmodel

import "time"

type Task struct {
	ID                  int64      `gorm:"primaryKey;autoIncrement"`
	ProjectID           int64      `gorm:"column:project_id;not null;index"`
	UserID              int64      `gorm:"column:user_id;not null;index"`
	SourceParseResultID *int64     `gorm:"column:source_parse_result_id;index"`
	Title               string     `gorm:"type:varchar(255);not null"`
	Description         *string    `gorm:"type:text"`
	Status              string     `gorm:"type:varchar(20);not null"`
	Priority            string     `gorm:"type:varchar(20);not null"`
	Deadline            *time.Time `gorm:"column:deadline"`
	SortOrder           int32      `gorm:"column:sort_order;not null;default:0"`
	SourceType          string     `gorm:"column:source_type;type:varchar(20);not null"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

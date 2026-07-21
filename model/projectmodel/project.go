package projectmodel

import "time"

type Project struct {
	ID               int64      `gorm:"primaryKey;autoIncrement"`
	UserID           int64      `gorm:"column:user_id;not null;index"`
	SourceDocumentID int64      `gorm:"column:source_document_id;not null;index"`
	ParseResultID    int64      `gorm:"column:parse_result_id;not null;index"`
	Name             string     `gorm:"type:varchar(255);not null"`
	Description      *string    `gorm:"type:text"`
	Deadline         *time.Time `gorm:"column:deadline"`
	Status           string     `gorm:"type:varchar(20);not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

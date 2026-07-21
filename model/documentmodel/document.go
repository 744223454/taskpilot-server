package documentmodel

import "time"

type Document struct {
	ID         int64   `gorm:"primaryKey;autoIncrement"`
	UserID     int64   `gorm:"column:user_id;not null;index"`
	SourceType string  `gorm:"column:source_type;type:varchar(20);not null"`
	Title      *string `gorm:"type:varchar(255)"`
	FileName   *string `gorm:"column:file_name;type:varchar(255)"`
	FileURL    *string `gorm:"column:file_url;type:varchar(500)"`
	RawText    *string `gorm:"column:raw_text;type:text"`
	TextInput  *string `gorm:"column:text_input;type:text"`
	PageCount  *int32  `gorm:"column:page_count"`
	FileSize   *int64  `gorm:"column:file_size"`
	Status     string  `gorm:"type:varchar(20);not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

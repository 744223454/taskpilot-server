package usermodel

import "time"

type User struct {
	ID           int64   `gorm:"primaryKey;autoIncrement"`
	Email        string  `gorm:"type:varchar(128);not null;index:uq_users_email_lower,unique,expression:LOWER(email)"`
	PasswordHash string  `gorm:"column:password_hash;type:varchar(255);not null"`
	Nickname     string  `gorm:"column:nickname;type:varchar(64);not null"`
	AvatarURL    *string `gorm:"column:avatar_url;type:varchar(255)"`
	Status       int16   `gorm:"type:smallint;not null;default:1"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

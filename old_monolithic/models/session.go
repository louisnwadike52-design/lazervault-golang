package models

import (
	"time"
)

type Session struct {
	ID           string    `gorm:"primaryKey;type:uuid"`
	UserID       uint      `gorm:"not null"`
	User         User      `gorm:"foreignKey:UserID"`
	AccessToken  string    `gorm:"not null"`
	RefreshToken string    `gorm:"not null;unique"`
	UserAgent    string    `gorm:"not null"`
	ClientIP     string    `gorm:"not null"`
	IsBlocked    bool      `gorm:"not null;default:false"`
	ExpiresAt    time.Time `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

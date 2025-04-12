package models

import (
	"time"
)

type Session struct {
	ID           string    `gorm:"primaryKey"`
	UserID       uint      `gorm:"not null"`
	AccessToken  string    `gorm:"not null"`
	RefreshToken string    `gorm:"not null"`
	UserAgent    string    `gorm:"not null"`
	ClientIP     string    `gorm:"not null"`
	IsBlocked    bool      `gorm:"not null;default:false"`
	ExpiresAt    time.Time `gorm:"not null"`
	CreatedAt    time.Time
}

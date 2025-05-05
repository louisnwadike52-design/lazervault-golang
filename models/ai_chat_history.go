package models

import (
	"time"
)

// AIChatHistory stores individual interactions with the AI chat.
type AIChatHistory struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"not null;index"` // Foreign key to User model
	Query     string    `gorm:"type:text;not null"`
	Response  string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"index"`

	User User `gorm:"foreignKey:UserID"` // Optional relationship
}

// TableName specifies the custom table name
func (AIChatHistory) TableName() string {
	return "ai_chat_histories"
}

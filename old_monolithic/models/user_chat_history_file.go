package models

import (
	"time"
)

// UserChatHistoryFile stores the path to the generated chat history file in GCS for a user.
type UserChatHistoryFile struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null;uniqueIndex"` // Foreign key to User model, unique constraint
	FilePath  string `gorm:"not null"`             // The GCS path (e.g., gs://bucket/user-chat-history/123/history.log)
	UpdatedAt time.Time

	User User `gorm:"foreignKey:UserID"` // Optional relationship
}

// TableName specifies the custom table name
func (UserChatHistoryFile) TableName() string {
	return "user_chat_history_files"
}

package models

import (
	"time"

	"gorm.io/gorm"
)

// ChatMessage represents a message in the database
type ChatMessage struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	SenderUserID     string    `gorm:"type:uuid;not null;index:idx_chat_participants"` // Index for querying user chats
	ReceiverUserID   string    `gorm:"type:uuid;not null;index:idx_chat_participants"` // Index for querying user chats
	Content          string    `gorm:"type:text;not null"`                             // Text content or file description
	MessageType      string    `gorm:"type:varchar(20);not null"`                      // e.g., "TEXT", "IMAGE", "VIDEO", "FILE"
	AttachmentURL    *string   `gorm:"type:text"`                                      // Pointer to allow NULL, stores URL or path
	ReplyToMessageID *string   `gorm:"type:uuid"`                                      // Pointer to allow NULL
	Timestamp        time.Time `gorm:"not null;index"`                                 // Index for sorting
	CreatedAt        time.Time `gorm:"default:CURRENT_TIMESTAMP"`
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`

	// Optional: Define relationships if needed, e.g., to fetch the replied-to message easily
	// ReplyToMessage   *ChatMessage `gorm:"foreignKey:ReplyToMessageID"`
}

// TableName specifies the table name for the ChatMessage model
func (ChatMessage) TableName() string {
	return "chat_messages"
}

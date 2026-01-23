package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScanSession represents an AI scan-to-pay session
type ScanSession struct {
	ID            string    `gorm:"type:varchar(255);primaryKey" json:"id"`
	UserID        string    `gorm:"type:varchar(255);not null;index" json:"user_id"`
	ScanType      string    `gorm:"type:varchar(50);not null" json:"scan_type"`
	Status        string    `gorm:"type:varchar(50);not null;default:'PENDING'" json:"status"`
	ExtractedData string    `gorm:"type:text" json:"extracted_data"`
	TransactionID string    `gorm:"type:varchar(255)" json:"transaction_id"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName specifies the table name for ScanSession
func (ScanSession) TableName() string {
	return "scan_sessions"
}

// BeforeCreate hook to generate ID
func (s *ScanSession) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}

// ScanChatMessage represents a chat message in an AI scan session
type ScanChatMessage struct {
	ID        string    `gorm:"type:varchar(255);primaryKey" json:"id"`
	SessionID string    `gorm:"type:varchar(255);not null;index" json:"session_id"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	IsUser    bool      `gorm:"not null;default:false" json:"is_user"`
	Timestamp time.Time `gorm:"autoCreateTime" json:"timestamp"`
}

// TableName specifies the table name for ScanChatMessage
func (ScanChatMessage) TableName() string {
	return "scan_chat_messages"
}

// BeforeCreate hook to generate ID
func (s *ScanChatMessage) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}

package models

import (
	"time"
)

// UserVoiceFile stores the path to uploaded voice files in GCS for a user.
type UserVoiceFile struct {
	ID            uint       `gorm:"primaryKey"`
	UserID        uint       `gorm:"not null;index"` // Foreign key to User model, indexed for quick lookups
	FileURL       string     `gorm:"not null"`       // The GCS public URL
	Filename      string     `gorm:"not null"`       // Original filename
	ContentType   string     `gorm:"not null"`       // MIME type
	FileSize      int64      `gorm:"not null"`       // File size in bytes
	ProcessedAt   *time.Time `gorm:"default:null"`   // When the file was processed by AI (nullable)
	Response      string     `gorm:"type:text"`      // AI response text
	Transcription string     `gorm:"type:text"`      // Transcribed audio text
	CreatedAt     time.Time  `gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime"`

	User User `gorm:"foreignKey:UserID"` // Optional relationship
}

// TableName specifies the custom table name
func (UserVoiceFile) TableName() string {
	return "user_voice_files"
}

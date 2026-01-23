package models

import (
	"time"
)

// EmailVerification stores data related to email verification codes.
type EmailVerification struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	UserID    uint       `gorm:"not null;index" json:"user_id"` // Foreign key to users table
	Email     string     `gorm:"not null;index" json:"email"`   // Email being verified
	Code      string     `gorm:"not null;uniqueIndex" json:"-"` // The secret verification code (don't expose in JSON)
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UsedAt    *time.Time `gorm:"index" json:"used_at,omitempty"` // Pointer allows null, timestamp when the code was used

	// Optional: Define foreign key relationship if using GORM migrations
	// User User `gorm:"foreignKey:UserID"`
}

// TableName specifies the database table name for GORM.
func (EmailVerification) TableName() string {
	return "email_verifications"
}

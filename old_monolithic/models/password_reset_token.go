package models

import (
	"time"

	"gorm.io/gorm"
)

// PasswordResetToken stores information for resetting passwords.
type PasswordResetToken struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	UserID    uint           `gorm:"not null;index" json:"user_id"`                 // Foreign key to the User model
	User      User           `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"` // Belongs To relationship
	Token     string         `gorm:"not null;uniqueIndex" json:"token"`             // The unique reset token
	ExpiresAt time.Time      `gorm:"not null" json:"expires_at"`                    // When the token expires
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

package models

import (
	"time"
)

// PasswordResetOTP stores data for password reset OTPs.
type PasswordResetOTP struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"not null;index" json:"user_id"`    // Foreign key to users table
	Identifier string     `gorm:"not null;index" json:"identifier"` // Email or phone used to initiate reset
	OTPCode    string     `gorm:"not null;index" json:"-"`          // The secret OTP code (hashed maybe? For now, plaintext)
	ExpiresAt  time.Time  `gorm:"not null" json:"expires_at"`
	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UsedAt     *time.Time `gorm:"index" json:"used_at,omitempty"` // Pointer allows null, timestamp when the code was used

	// Optional: Define foreign key relationship
	// User User `gorm:"foreignKey:UserID"`
}

// TableName specifies the database table name for GORM.
func (PasswordResetOTP) TableName() string {
	return "password_reset_otps"
}

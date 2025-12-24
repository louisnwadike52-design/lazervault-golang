package models

import (
	"time"

	"gorm.io/gorm"
)

// SyncedContact represents a device contact that has been synced to the backend
type SyncedContact struct {
	gorm.Model
	UserID           uint      `gorm:"not null;index:idx_user_contacts"`                    // Owner user ID
	Name             string    `gorm:"type:varchar(255);not null;index:idx_contact_search"` // Contact display name
	PhoneNumbers     string    `gorm:"type:text"`                                           // JSON array of phone numbers
	Emails           string    `gorm:"type:text"`                                           // JSON array of emails
	PhotoURL         string    `gorm:"type:varchar(500)"`                                   // Profile photo URL or base64
	DeviceContactID  string    `gorm:"type:varchar(255);index"`                             // Original device contact ID
	IsLazerVaultUser bool      `gorm:"not null;default:false;index"`                        // Whether this contact is a registered user
	LazerVaultUserID *uint     `gorm:"index"`                                               // LazerVault user ID if registered
	LastSyncedAt     time.Time `gorm:"not null;default:current_timestamp"`                  // Last sync timestamp

	// Relationship
	User           User  `gorm:"foreignKey:UserID"`
	LazerVaultUser *User `gorm:"foreignKey:LazerVaultUserID"` // Nullable if not a LazerVault user
}

// TableName specifies the database table name for GORM
func (SyncedContact) TableName() string {
	return "synced_contacts"
}

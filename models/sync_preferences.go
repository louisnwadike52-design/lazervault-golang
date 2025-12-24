package models

import (
	"time"

	"gorm.io/gorm"
)

// SyncFrequency represents how often contacts should be synced
type SyncFrequency string

const (
	SyncFrequencyUnspecified SyncFrequency = "unspecified"
	SyncFrequencyManual      SyncFrequency = "manual"
	SyncFrequencyDaily       SyncFrequency = "daily"
	SyncFrequencyWeekly      SyncFrequency = "weekly"
	SyncFrequencyRealTime    SyncFrequency = "real_time"
)

// SyncPreferences stores user preferences for contact syncing
type SyncPreferences struct {
	gorm.Model
	UserID              uint          `gorm:"not null;uniqueIndex"` // One preference record per user
	AutoSyncEnabled     bool          `gorm:"not null;default:false"`
	SyncFrequency       SyncFrequency `gorm:"type:varchar(50);not null;default:'manual'"`
	MatchWithUsers      bool          `gorm:"not null;default:true"`  // Auto-match contacts with LazerVault users
	SyncPhotos          bool          `gorm:"not null;default:false"` // Include contact photos in sync
	LastSyncAt          *time.Time    `gorm:"index"`                  // Last successful sync timestamp
	TotalSyncedContacts int           `gorm:"not null;default:0"`     // Total number of synced contacts
	TotalMatchedUsers   int           `gorm:"not null;default:0"`     // Total matched LazerVault users

	// Relationship
	User User `gorm:"foreignKey:UserID"`
}

// TableName specifies the database table name for GORM
func (SyncPreferences) TableName() string {
	return "sync_preferences"
}

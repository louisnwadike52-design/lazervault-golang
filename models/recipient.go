package models

import (
	"gorm.io/gorm"
)

// Recipient represents a saved destination for transfers (internal or external)
type Recipient struct {
	gorm.Model
	OwnerUserID uint   `gorm:"not null;index"`             // User who owns/saved this recipient
	Name        string `gorm:"type:varchar(255);not null"` // Recipient's name
	IsFavorite  bool   `gorm:"not null;default:false"`
	Type        string `gorm:"type:varchar(50);not null;index"` // "internal" or "external"

	// Fields for INTERNAL recipients
	InternalAccountID *uint `gorm:"index"` // Pointer to link to an internal Account ID
	InternalUserID    *uint `gorm:"index"` // Pointer to link to an internal User ID (denormalized for easier lookup)

	// Fields for EXTERNAL recipients
	AccountNumber string `gorm:"type:varchar(100)"` // Encrypt sensitive fields in production!
	SortCode      string `gorm:"type:varchar(50)"`  // Or Routing Number etc.
	BankName      string `gorm:"type:varchar(255)"`
	CountryCode   string `gorm:"type:varchar(10)"` // e.g., "GB", "US"
	Email         string `gorm:"type:varchar(255)"`
	PhoneNumber   string `gorm:"type:varchar(50)"`
	Currency      string `gorm:"type:varchar(10)"`
	SwiftCode     string `gorm:"type:varchar(20)"`
	IBAN          string `gorm:"type:varchar(50)"`

	// Relationships
	Owner           User     `gorm:"foreignKey:OwnerUserID"`
	InternalAccount *Account `gorm:"foreignKey:InternalAccountID"` // Nullable if external
}

// TableName specifies the database table name for GORM.
func (Recipient) TableName() string {
	return "recipients"
}

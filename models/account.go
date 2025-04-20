package models

import (
	"gorm.io/gorm"
)

// Constants for Account Types
const (
	AccountTypePersonal   = "personal"
	AccountTypeSavings    = "savings"
	AccountTypeInvestment = "investment"
)

// Account represents a user's financial account within LazerVault.
type Account struct {
	gorm.Model            // Includes ID, CreatedAt, UpdatedAt, DeletedAt
	OwnerUserID   uint    `gorm:"not null;index"` // Foreign key to the User model
	AccountNumber string  `gorm:"type:varchar(100);uniqueIndex;not null"`
	AccountType   string  `gorm:"type:varchar(50);not null"` // e.g., personal, savings, investment
	Currency      string  `gorm:"type:varchar(10);not null"` // e.g., USD, GBP
	Balance       float64 `gorm:"type:decimal(18,2);not null;default:0.0"`
	IsActive      bool    `gorm:"not null;default:true"`

	// Relationships
	Owner User          `gorm:"foreignKey:OwnerUserID"` // Belongs To relationship
	Cards []AccountCard `gorm:"foreignKey:AccountID"`   // Has Many relationship
}

// TableName specifies the table name for the Account model.
func (Account) TableName() string {
	return "accounts"
}

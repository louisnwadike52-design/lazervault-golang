package models

import (
	"gorm.io/gorm"
)

// Recipient represents a saved external bank account detail for sending funds to.
type Recipient struct {
	gorm.Model           // Includes ID, CreatedAt, UpdatedAt, DeletedAt
	OwnerUserID   uint   `gorm:"not null;index" json:"owner_user_id"`
	Name          string `gorm:"type:varchar(255);not null" json:"name"`           // Nickname for the recipient (e.g., "Landlord", "Alice Savings")
	AccountNumber string `gorm:"type:varchar(100);not null" json:"account_number"` // External bank account number
	SortCode      string `gorm:"type:varchar(50)" json:"sort_code,omitempty"`      // Optional: Sort code (UK specific, example)
	BankName      string `gorm:"type:varchar(255);not null" json:"bank_name"`      // Name of the recipient's bank
	IsFavorite    bool   `gorm:"default:false" json:"is_favorite"`                 // Flag for easy access

	// Define relationship
	Owner User `gorm:"foreignKey:OwnerUserID"`
}

// TableName specifies the database table name for GORM.
func (Recipient) TableName() string {
	return "recipients"
}

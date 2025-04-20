package models

import (
	"gorm.io/gorm"
)

type Recipient struct {
	gorm.Model           // Includes ID (uint), CreatedAt, UpdatedAt, DeletedAt
	OwnerUserID   uint   `gorm:"not null;index"` // Matches User ID type
	Name          string `gorm:"type:varchar(255);not null"`
	AccountNumber string `gorm:"type:varchar(100);not null"` // Bank account number
	SortCode      string `gorm:"type:varchar(20)"`           // Optional sort code
	BankName      string `gorm:"type:varchar(255);not null"`
	IsFavorite    bool   `gorm:"not null;default:false"`

	// Relationship (optional, depends if you need easy access to User from Recipient)
	// Owner         User   `gorm:"foreignKey:OwnerUserID"`
}

func (Recipient) TableName() string {
	return "recipients"
}

// Removed BeforeCreate hook as ID is now uint and handled by gorm.Model

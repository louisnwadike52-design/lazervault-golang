package models

import (
	"gorm.io/gorm"
)

type Balance struct {
	gorm.Model
	UserID   uint   `gorm:"uniqueIndex;not null"` // Foreign key to users table. uniqueIndex ensures one-to-one.
	Currency string `gorm:"size:3;not null;default:'USD'"`
	Amount   int64  `gorm:"not null;default:0"`
}

func (Balance) TableName() string {
	return "balances"
}

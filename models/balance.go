package models

import (
	"gorm.io/gorm"
)

type Balance struct {
	gorm.Model
	UserID uint    `gorm:"uniqueIndex;not null"` // Foreign key to User
	Amount float64 `gorm:"not null;default:0"`
}

func (Balance) TableName() string {
	return "balances"
}

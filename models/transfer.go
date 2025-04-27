package models

import (
	"time"

	"gorm.io/gorm"
)

type TransferStatus string

const (
	TransferStatusPending   TransferStatus = "pending"
	TransferStatusCompleted TransferStatus = "completed"
	TransferStatusFailed    TransferStatus = "failed"
	TransferStatusReverted  TransferStatus = "reverted"
)

type Transfer struct {
	gorm.Model
	FromUserID    uint           `json:"from_user_id"`
	ToUserID      uint           `json:"to_user_id"`
	FromAccountID uint           `json:"from_account_id" gorm:"index"`
	ToAccountID   uint           `json:"to_account_id" gorm:"index"`
	Amount        int64          `json:"amount" gorm:"not null"`
	Fee           int64          `json:"fee" gorm:"not null;default:0"`
	TotalAmount   int64          `json:"total_amount" gorm:"not null"`
	Status        TransferStatus `json:"status" gorm:"not null;default:'pending'"`
	Reference     string         `json:"reference" gorm:"type:varchar(255)"`
	Category      string         `json:"category" gorm:"type:varchar(100)"`
	ScheduledAt   *time.Time     `json:"scheduled_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
	FailedAt      *time.Time     `json:"failed_at"`
	FailureReason string         `json:"failure_reason" gorm:"type:text"`
	FromUser      User           `json:"from_user" gorm:"foreignKey:FromUserID"`
	ToUser        User           `json:"to_user" gorm:"foreignKey:ToUserID"`
	FromAccount   Account        `json:"from_account" gorm:"foreignKey:FromAccountID"`
	ToAccount     Account        `json:"to_account" gorm:"foreignKey:ToAccountID"`
}

type FailedTransfer struct {
	gorm.Model
	TransferID    uint       `json:"transfer_id" gorm:"not null;index"`
	FromUserID    uint       `json:"from_user_id"`
	ToUserID      uint       `json:"to_user_id"`
	FromAccountID uint       `json:"from_account_id"`
	ToAccountID   uint       `json:"to_account_id"`
	Amount        int64      `json:"amount" gorm:"not null"`
	Fee           int64      `json:"fee" gorm:"not null"`
	TotalAmount   int64      `json:"total_amount" gorm:"not null"`
	FailureReason string     `json:"failure_reason" gorm:"type:text;not null"`
	Reverted      bool       `json:"reverted" gorm:"default:false"`
	RevertedAt    *time.Time `json:"reverted_at"`
}

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
	Amount        float64        `json:"amount"`
	Fee           float64        `json:"fee"`
	TotalAmount   float64        `json:"total_amount"`
	Status        TransferStatus `json:"status"`
	Reference     string         `json:"reference"`
	Category      string         `json:"category"`
	ScheduledAt   *time.Time     `json:"scheduled_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
	FailedAt      *time.Time     `json:"failed_at"`
	FailureReason string         `json:"failure_reason"`
	FromUser      User           `json:"from_user" gorm:"foreignKey:FromUserID"`
	ToUser        User           `json:"to_user" gorm:"foreignKey:ToUserID"`
	FromAccount   Account        `json:"from_account" gorm:"foreignKey:FromAccountID"`
	ToAccount     Account        `json:"to_account" gorm:"foreignKey:ToAccountID"`
}

type FailedTransfer struct {
	gorm.Model
	TransferID    uint       `json:"transfer_id"`
	FromUserID    uint       `json:"from_user_id"`
	ToUserID      uint       `json:"to_user_id"`
	FromAccountID uint       `json:"from_account_id"`
	ToAccountID   uint       `json:"to_account_id"`
	Amount        float64    `json:"amount"`
	Fee           float64    `json:"fee"`
	TotalAmount   float64    `json:"total_amount"`
	FailureReason string     `json:"failure_reason"`
	Reverted      bool       `json:"reverted"`
	RevertedAt    *time.Time `json:"reverted_at"`
	FromUser      User       `json:"from_user" gorm:"foreignKey:FromUserID"`
	ToUser        User       `json:"to_user" gorm:"foreignKey:ToUserID"`
}

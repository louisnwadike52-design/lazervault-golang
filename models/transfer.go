package models

import (
	"time"

	"gorm.io/gorm"
)

type TransferStatus string

const (
	TransferStatusPending    TransferStatus = "pending"
	TransferStatusProcessing TransferStatus = "processing"
	TransferStatusScheduled  TransferStatus = "scheduled"
	TransferStatusCompleted  TransferStatus = "completed"
	TransferStatusFailed     TransferStatus = "failed"
	TransferStatusReverted   TransferStatus = "reverted"
)

type Transfer struct {
	gorm.Model
	FromUserID    uint           `json:"from_user_id"`
	FromAccountID uint           `json:"from_account_id" gorm:"index"`
	ToUserID      *uint          `json:"to_user_id"`
	ToAccountID   *uint          `json:"to_account_id" gorm:"index"`
	RecipientID   *uint          `json:"recipient_id" gorm:"index"`
	Amount        int64          `json:"amount" gorm:"not null"`
	Fee           int64          `json:"fee" gorm:"not null;default:0"`
	TotalAmount   int64          `json:"total_amount" gorm:"not null"`
	Status        TransferStatus `json:"status" gorm:"not null;default:'pending'"`
	Reference      string         `json:"reference" gorm:"type:varchar(255)"`
	Category       string         `json:"category" gorm:"type:varchar(100)"`
	ScheduledAt    *string        `json:"scheduled_at" gorm:"default:null"`
	IdempotencyKey *string        `json:"idempotency_key" gorm:"type:varchar(255);uniqueIndex"`
	BatchID        *string        `json:"batch_id" gorm:"type:varchar(255);index"`
	CompletedAt    *time.Time     `json:"completed_at"`
	FailedAt       *time.Time     `json:"failed_at"`
	FailureReason  string         `json:"failure_reason" gorm:"type:text"`
	FromUser      User           `json:"from_user" gorm:"foreignKey:FromUserID"`
	ToUser        *User          `json:"to_user" gorm:"foreignKey:ToUserID"`
	FromAccount   Account        `json:"from_account" gorm:"foreignKey:FromAccountID"`
	ToAccount     *Account       `json:"to_account" gorm:"foreignKey:ToAccountID"`
	Recipient     *Recipient     `json:"recipient" gorm:"foreignKey:RecipientID"`
}

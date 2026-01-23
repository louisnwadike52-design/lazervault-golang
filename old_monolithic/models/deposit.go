package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DepositStatus defines the possible states of a deposit.
type DepositStatus string

const (
	DepositStatusPending    DepositStatus = "PENDING"
	DepositStatusProcessing DepositStatus = "PROCESSING" // Added status for when worker picks it up
	DepositStatusCompleted  DepositStatus = "COMPLETED"
	DepositStatusFailed     DepositStatus = "FAILED" // Transient status before moving to FailedDeposit
)

// Deposit represents a deposit transaction attempt.
// Its status is updated asynchronously by a worker.
type Deposit struct {
	ID string `gorm:"primaryKey;type:varchar(36)" json:"id"` // UUID

	UserID          uint    `gorm:"not null;index" json:"user_id"`
	TargetAccountID uint    `gorm:"not null;index" json:"target_account_id"`
	Account         Account `gorm:"foreignKey:TargetAccountID"` // Foreign key relationship

	Amount         int64  `gorm:"not null" json:"amount"`
	Currency       string `gorm:"not null;size:3" json:"currency"`
	SourceBankName string `gorm:"not null;size:255" json:"source_bank_name"`

	Status DepositStatus `gorm:"not null;type:varchar(20);default:'PENDING';index" json:"status"`

	ExternalTransactionID *string `gorm:"size:255;index" json:"external_transaction_id,omitempty"` // Optional ID from payment provider
	FailureReason         *string `gorm:"size:500" json:"failure_reason,omitempty"`                // Set only if Status becomes FAILED

	CreatedAt    time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
	ProcessingAt *time.Time `json:"processing_at,omitempty"` // When worker started processing
	CompletedAt  *time.Time `gorm:"index" json:"completed_at,omitempty"`
	FailedAt     *time.Time `json:"failed_at,omitempty"` // When processing determined failure
	UpdatedAt    time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// BeforeCreate Hook to generate UUID for Deposit
func (d *Deposit) BeforeCreate(tx *gorm.DB) (err error) {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	return
}

// TableName specifies the database table name for GORM.
func (Deposit) TableName() string {
	return "deposits"
}

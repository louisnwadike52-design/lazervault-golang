package models

import (
	"time"
)

// WithdrawalStatus defines the possible states of a withdrawal.
type WithdrawalStatus string

const (
	WithdrawalStatusPending    WithdrawalStatus = "PENDING"
	WithdrawalStatusProcessing WithdrawalStatus = "PROCESSING" // If async payout
	WithdrawalStatusCompleted  WithdrawalStatus = "COMPLETED"
	WithdrawalStatusFailed     WithdrawalStatus = "FAILED"
)

// Withdrawal represents a withdrawal transaction from a LazerVault account.
type Withdrawal struct {
	ID              string `gorm:"primaryKey;type:varchar(36)" json:"id"` // Using UUID as string
	UserID          uint   `gorm:"not null;index" json:"user_id"`
	SourceAccountID uint   `gorm:"not null;index" json:"source_account_id"`
	Amount          int64  `gorm:"not null" json:"amount"` // Changed to int64 (minor units)
	// Fee                 int64            `gorm:"default:0" json:"fee"` // Changed to int64 (minor units)
	// TotalAmount         int64            `gorm:"not null" json:"total_amount"` // Changed to int64 (minor units)
	Currency              string           `gorm:"not null;size:3" json:"currency"`
	TargetBankName        string           `gorm:"not null;size:255" json:"target_bank_name"`
	TargetAccountNumber   string           `gorm:"not null;size:100" json:"target_account_number"` // Consider encrypting this?
	TargetSortCode        string           `gorm:"size:50" json:"target_sort_code,omitempty"`
	Status                WithdrawalStatus `gorm:"not null;type:varchar(20);default:'PENDING';index" json:"status"`
	ExternalTransactionID *string          `gorm:"size:255;index" json:"external_transaction_id,omitempty"` // Added
	TransactionReference  string           `gorm:"size:255;index" json:"transaction_reference,omitempty"`   // Optional internal ref
	FailureReason         string           `gorm:"size:500" json:"failure_reason,omitempty"`                // Changed to string
	CreatedAt             time.Time        `gorm:"autoCreateTime" json:"created_at"`
	ProcessingAt          *time.Time       `json:"processing_at,omitempty"` // Added
	CompletedAt           *time.Time       `gorm:"index" json:"completed_at,omitempty"`
	FailedAt              *time.Time       `json:"failed_at,omitempty"`
	UpdatedAt             time.Time        `gorm:"autoUpdateTime" json:"updated_at"`

	// Define foreign key relationships
	User    User    `gorm:"foreignKey:UserID"`
	Account Account `gorm:"foreignKey:SourceAccountID"`
}

// TableName specifies the database table name for GORM.
func (Withdrawal) TableName() string {
	return "withdrawals"
}

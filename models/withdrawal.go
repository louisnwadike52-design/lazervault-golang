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
	ID              string  `gorm:"primaryKey;type:varchar(36)" json:"id"` // Using UUID as string
	UserID          uint    `gorm:"not null;index" json:"user_id"`
	SourceAccountID uint    `gorm:"not null;index" json:"source_account_id"`
	Amount          float64 `gorm:"not null" json:"amount"` // Amount requested to withdraw
	// Fee                 float64          `gorm:"default:0.0" json:"fee"`             // Optional withdrawal fee
	// TotalAmount         float64          `gorm:"not null" json:"total_amount"`         // Amount + Fee deducted
	Currency             string           `gorm:"not null;size:3" json:"currency"`
	TargetBankName       string           `gorm:"not null;size:255" json:"target_bank_name"`
	TargetAccountNumber  string           `gorm:"not null;size:100" json:"target_account_number"` // Consider encrypting this?
	TargetSortCode       string           `gorm:"size:50" json:"target_sort_code,omitempty"`
	Status               WithdrawalStatus `gorm:"not null;type:varchar(20);default:'PENDING';index" json:"status"`
	TransactionReference string           `gorm:"size:255;index" json:"transaction_reference,omitempty"` // Optional external ref
	FailureReason        string           `gorm:"size:255" json:"failure_reason,omitempty"`
	CreatedAt            time.Time        `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time        `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt          *time.Time       `gorm:"index" json:"completed_at,omitempty"`
	FailedAt             *time.Time       `json:"failed_at,omitempty"`

	// Define foreign key relationships
	User    User    `gorm:"foreignKey:UserID"`
	Account Account `gorm:"foreignKey:SourceAccountID"`
}

// TableName specifies the database table name for GORM.
func (Withdrawal) TableName() string {
	return "withdrawals"
}

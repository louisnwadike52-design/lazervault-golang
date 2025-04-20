package models

import (
	"time"
)

// DepositStatus defines the possible states of a deposit.
type DepositStatus string

const (
	DepositStatusPending   DepositStatus = "PENDING"
	DepositStatusCompleted DepositStatus = "COMPLETED"
	DepositStatusFailed    DepositStatus = "FAILED"
)

// Deposit represents a deposit transaction into a LazerVault account.
type Deposit struct {
	ID                   string        `gorm:"primaryKey;type:varchar(36)" json:"id"` // Using UUID as string
	UserID               uint          `gorm:"not null;index" json:"user_id"`
	TargetAccountID      uint          `gorm:"not null;index" json:"target_account_id"`
	Amount               float64       `gorm:"not null" json:"amount"`
	Currency             string        `gorm:"not null;size:3" json:"currency"`
	SourceBankName       string        `gorm:"not null;size:255" json:"source_bank_name"`
	Status               DepositStatus `gorm:"not null;type:varchar(20);default:'PENDING';index" json:"status"`
	TransactionReference string        `gorm:"size:255;index" json:"transaction_reference,omitempty"` // Optional external ref
	FailureReason        string        `gorm:"size:255" json:"failure_reason,omitempty"`
	CreatedAt            time.Time     `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time     `gorm:"autoUpdateTime" json:"updated_at"`
	CompletedAt          *time.Time    `gorm:"index" json:"completed_at,omitempty"`
	FailedAt             *time.Time    `json:"failed_at,omitempty"`

	// Define foreign key relationships
	User    User    `gorm:"foreignKey:UserID"`
	Account Account `gorm:"foreignKey:TargetAccountID"`
}

// TableName specifies the database table name for GORM.
func (Deposit) TableName() string {
	return "deposits"
}

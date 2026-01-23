package models

import (
	"time"

	"gorm.io/gorm"
)

// FailedDeposit represents a deposit transaction that failed during processing.
// It stores a snapshot of the deposit attempt for auditing/review.
type FailedDeposit struct {
	gorm.Model               // Includes ID, CreatedAt, UpdatedAt, DeletedAt (useful for tracking when failure was recorded)
	OriginalDepositID string `gorm:"type:varchar(36);not null;index"` // Link back to the original (now likely deleted) deposit ID

	UserID                uint    `gorm:"not null;index"`
	TargetAccountID       uint    `gorm:"not null;index"`
	Amount                float64 `gorm:"not null"`
	Currency              string  `gorm:"not null;size:3"`
	SourceBankName        string  `gorm:"not null;size:255"`
	ExternalTransactionID *string `gorm:"size:255;index"`
	FailureReason         string  `gorm:"not null;size:500"` // Reason is mandatory for failed deposits

	AttemptedAt time.Time `gorm:"not null;index"` // When the original deposit was created
	FailedAt    time.Time `gorm:"not null;index"` // When the failure was processed
}

// TableName specifies the database table name for GORM.
func (FailedDeposit) TableName() string {
	return "failed_deposits"
}

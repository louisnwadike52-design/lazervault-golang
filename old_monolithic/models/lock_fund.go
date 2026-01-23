package models

import (
	"time"

	"gorm.io/gorm"
)

// LockType represents the type of lock
type LockType string

const (
	LockTypeSavings       LockType = "SAVINGS"
	LockTypeInvestment    LockType = "INVESTMENT"
	LockTypeEmergencyFund LockType = "EMERGENCY_FUND"
	LockTypeGoalBased     LockType = "GOAL_BASED"
)

// LockStatus represents the status of a lock
type LockStatus string

const (
	LockStatusActive    LockStatus = "ACTIVE"
	LockStatusMatured   LockStatus = "MATURED"
	LockStatusUnlocked  LockStatus = "UNLOCKED"
	LockStatusCancelled LockStatus = "CANCELLED"
)

// LockFund represents a locked fund in the database
type LockFund struct {
	ID                        string         `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	UserID                    uint           `gorm:"not null;index" json:"user_id"`
	LockType                  LockType       `gorm:"type:varchar(50);not null" json:"lock_type"`
	Amount                    float64        `gorm:"not null" json:"amount"`
	Currency                  string         `gorm:"type:varchar(3);not null;default:'USD'" json:"currency"`
	LockDurationDays          int32          `gorm:"not null" json:"lock_duration_days"`
	InterestRate              float64        `gorm:"not null" json:"interest_rate"`
	LockedAt                  time.Time      `gorm:"not null" json:"locked_at"`
	UnlockAt                  time.Time      `gorm:"not null" json:"unlock_at"`
	Status                    LockStatus     `gorm:"type:varchar(20);not null;default:'ACTIVE'" json:"status"`
	AutoRenew                 bool           `gorm:"default:false" json:"auto_renew"`
	GoalName                  string         `gorm:"type:varchar(200)" json:"goal_name"`
	GoalDescription           string         `gorm:"type:text" json:"goal_description"`
	EarlyUnlockPenaltyPercent float64        `gorm:"default:0" json:"early_unlock_penalty_percent"`
	AccruedInterest           float64        `gorm:"default:0" json:"accrued_interest"`
	PaymentMethod             string         `gorm:"type:varchar(50)" json:"payment_method"`
	TransactionID             string         `gorm:"type:varchar(100)" json:"transaction_id"`
	CreatedAt                 time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt                 time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt                 gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	User         User                  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Transactions []LockFundTransaction `gorm:"foreignKey:LockFundID" json:"transactions,omitempty"`
}

// TableName specifies the table name for LockFund
func (LockFund) TableName() string {
	return "lock_funds"
}

// LockFundTransaction represents a transaction related to a lock fund
type LockFundTransaction struct {
	ID              string    `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()" json:"id"`
	LockFundID      string    `gorm:"type:uuid;not null;index" json:"lock_fund_id"`
	UserID          uint      `gorm:"not null;index" json:"user_id"`
	TransactionType string    `gorm:"type:varchar(50);not null" json:"transaction_type"` // LOCK, UNLOCK, INTEREST_ACCRUAL, PENALTY
	Amount          float64   `gorm:"not null" json:"amount"`
	Currency        string    `gorm:"type:varchar(3);not null;default:'USD'" json:"currency"`
	PaymentMethod   string    `gorm:"type:varchar(50)" json:"payment_method"`
	Status          string    `gorm:"type:varchar(20);not null;default:'COMPLETED'" json:"status"`
	TransactionDate time.Time `gorm:"not null" json:"transaction_date"`
	Description     string    `gorm:"type:text" json:"description"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	LockFund LockFund `gorm:"foreignKey:LockFundID" json:"lock_fund,omitempty"`
	User     User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName specifies the table name for LockFundTransaction
func (LockFundTransaction) TableName() string {
	return "lock_fund_transactions"
}

// DaysRemaining calculates days remaining until unlock
func (l *LockFund) DaysRemaining() int32 {
	if l.Status != LockStatusActive {
		return 0
	}

	now := time.Now()
	if now.After(l.UnlockAt) {
		return 0
	}

	duration := l.UnlockAt.Sub(now)
	return int32(duration.Hours() / 24)
}

// ProgressPercent calculates progress percentage
func (l *LockFund) ProgressPercent() float64 {
	if l.Status != LockStatusActive {
		return 100.0
	}

	totalDuration := l.UnlockAt.Sub(l.LockedAt)
	elapsed := time.Since(l.LockedAt)

	if elapsed >= totalDuration {
		return 100.0
	}

	return (elapsed.Seconds() / totalDuration.Seconds()) * 100.0
}

// TotalValue calculates total value (amount + accrued interest)
func (l *LockFund) TotalValue() float64 {
	return l.Amount + l.AccruedInterest
}

// CanUnlockEarly determines if early unlock is allowed
func (l *LockFund) CanUnlockEarly() bool {
	return l.Status == LockStatusActive && l.LockType != LockTypeInvestment
}

// CalculateInterest calculates accrued interest based on time elapsed
func (l *LockFund) CalculateInterest() float64 {
	if l.Status != LockStatusActive {
		return l.AccruedInterest
	}

	now := time.Now()
	daysElapsed := now.Sub(l.LockedAt).Hours() / 24.0

	// Simple interest calculation: Principal * Rate * Time
	// Interest rate is annual, so we divide by 365
	dailyRate := l.InterestRate / 365.0 / 100.0
	interest := l.Amount * dailyRate * daysElapsed

	return interest
}

// UpdateAccruedInterest updates the accrued interest field
func (l *LockFund) UpdateAccruedInterest() {
	l.AccruedInterest = l.CalculateInterest()
}

// IsMatured checks if the lock has matured
func (l *LockFund) IsMatured() bool {
	return time.Now().After(l.UnlockAt) && l.Status == LockStatusActive
}

package models

import (
	"time"

	"gorm.io/gorm"
)

type TriggerType string
type AmountType string
type AutoSaveStatus string
type ScheduleFrequency string

const (
	TriggerTypeOnDeposit TriggerType = "on_deposit"
	TriggerTypeScheduled TriggerType = "scheduled"
	TriggerTypeRoundUp   TriggerType = "round_up"
)

const (
	AmountTypeFixed      AmountType = "fixed"
	AmountTypePercentage AmountType = "percentage"
)

const (
	AutoSaveStatusActive    AutoSaveStatus = "active"
	AutoSaveStatusPaused    AutoSaveStatus = "paused"
	AutoSaveStatusCompleted AutoSaveStatus = "completed"
	AutoSaveStatusCancelled AutoSaveStatus = "cancelled"
)

const (
	ScheduleFrequencyDaily    ScheduleFrequency = "daily"
	ScheduleFrequencyWeekly   ScheduleFrequency = "weekly"
	ScheduleFrequencyBiweekly ScheduleFrequency = "biweekly"
	ScheduleFrequencyMonthly  ScheduleFrequency = "monthly"
)

type AutoSaveRule struct {
	gorm.Model
	UserID               uint               `json:"user_id" gorm:"not null;index"`
	Name                 string             `json:"name" gorm:"type:varchar(255);not null"`
	Description          string             `json:"description" gorm:"type:text"`
	TriggerType          TriggerType        `json:"trigger_type" gorm:"type:varchar(50);not null"`
	AmountType           AmountType         `json:"amount_type" gorm:"type:varchar(50);not null"`
	AmountValue          float64            `json:"amount_value" gorm:"not null"`
	SourceAccountID      uint               `json:"source_account_id" gorm:"not null;index"`
	DestinationAccountID uint               `json:"destination_account_id" gorm:"not null;index"`
	Status               AutoSaveStatus     `json:"status" gorm:"type:varchar(50);not null;default:'active'"`
	Frequency            *ScheduleFrequency `json:"frequency" gorm:"type:varchar(50)"`
	ScheduleTime         *string            `json:"schedule_time" gorm:"type:varchar(10)"` // Format: HH:MM
	ScheduleDay          *int               `json:"schedule_day"`                           // Day of week (1-7) or day of month (1-31)
	RoundUpTo            *int               `json:"round_up_to"`                            // Round up to nearest value (e.g., 10, 100)
	TargetAmount         *float64           `json:"target_amount"`                          // Optional savings goal
	MinimumBalance       *float64           `json:"minimum_balance"`                        // Minimum balance to maintain in source account
	MaximumPerSave       *float64           `json:"maximum_per_save"`                       // Maximum amount per save operation
	TriggerCount         int                `json:"trigger_count" gorm:"default:0"`         // Number of times triggered
	TotalSaved           float64            `json:"total_saved" gorm:"default:0"`           // Total amount saved
	LastTriggeredAt      *time.Time         `json:"last_triggered_at"`
	User                 User               `json:"user" gorm:"foreignKey:UserID"`
	SourceAccount        Account            `json:"source_account" gorm:"foreignKey:SourceAccountID"`
	DestinationAccount   Account            `json:"destination_account" gorm:"foreignKey:DestinationAccountID"`
}

type AutoSaveTransaction struct {
	gorm.Model
	RuleID               uint        `json:"rule_id" gorm:"not null;index"`
	UserID               uint        `json:"user_id" gorm:"not null;index"`
	SourceAccountID      uint        `json:"source_account_id" gorm:"not null"`
	DestinationAccountID uint        `json:"destination_account_id" gorm:"not null"`
	Amount               float64     `json:"amount" gorm:"not null"`
	TriggerType          TriggerType `json:"trigger_type" gorm:"type:varchar(50);not null"`
	TriggerReason        string      `json:"trigger_reason" gorm:"type:text"`
	Success              bool        `json:"success" gorm:"not null;default:false"`
	ErrorMessage         *string     `json:"error_message" gorm:"type:text"`
	Rule                 AutoSaveRule `json:"rule" gorm:"foreignKey:RuleID"`
	User                 User        `json:"user" gorm:"foreignKey:UserID"`
	SourceAccount        Account     `json:"source_account" gorm:"foreignKey:SourceAccountID"`
	DestinationAccount   Account     `json:"destination_account" gorm:"foreignKey:DestinationAccountID"`
}

// TableName overrides the table name
func (AutoSaveRule) TableName() string {
	return "autosave_rules"
}

// TableName overrides the table name
func (AutoSaveTransaction) TableName() string {
	return "autosave_transactions"
}

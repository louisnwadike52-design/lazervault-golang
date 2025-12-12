package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// ============================================================================
// ENUMS
// ============================================================================

type GroupAccountStatus string

const (
	GroupAccountStatusActive    GroupAccountStatus = "active"
	GroupAccountStatusSuspended GroupAccountStatus = "suspended"
	GroupAccountStatusClosed    GroupAccountStatus = "closed"
)

type GroupMemberRole string

const (
	GroupMemberRoleAdmin  GroupMemberRole = "admin"
	GroupMemberRoleMember GroupMemberRole = "member"
	GroupMemberRoleViewer GroupMemberRole = "viewer"
)

type GroupMemberStatus string

const (
	GroupMemberStatusActive    GroupMemberStatus = "active"
	GroupMemberStatusInactive  GroupMemberStatus = "inactive"
	GroupMemberStatusSuspended GroupMemberStatus = "suspended"
	GroupMemberStatusRemoved   GroupMemberStatus = "removed"
)

type ContributionType string

const (
	ContributionTypeOneTime         ContributionType = "one_time"
	ContributionTypeRecurring       ContributionType = "recurring"
	ContributionTypeRotatingSavings ContributionType = "rotating_savings"
)

type ContributionFrequency string

const (
	ContributionFrequencyDaily     ContributionFrequency = "daily"
	ContributionFrequencyWeekly    ContributionFrequency = "weekly"
	ContributionFrequencyBiweekly  ContributionFrequency = "biweekly"
	ContributionFrequencyMonthly   ContributionFrequency = "monthly"
	ContributionFrequencyQuarterly ContributionFrequency = "quarterly"
	ContributionFrequencyYearly    ContributionFrequency = "yearly"
)

type ContributionStatus string

const (
	ContributionStatusActive    ContributionStatus = "active"
	ContributionStatusPaused    ContributionStatus = "paused"
	ContributionStatusCompleted ContributionStatus = "completed"
	ContributionStatusCancelled ContributionStatus = "cancelled"
)

type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "pending"
	PaymentStatusProcessing PaymentStatus = "processing"
	PaymentStatusCompleted  PaymentStatus = "completed"
	PaymentStatusFailed     PaymentStatus = "failed"
	PaymentStatusRefunded   PaymentStatus = "refunded"
)

type PayoutStatus string

const (
	PayoutStatusPending   PayoutStatus = "pending"
	PayoutStatusCompleted PayoutStatus = "completed"
	PayoutStatusCancelled PayoutStatus = "cancelled"
)

type PayoutTransactionStatus string

const (
	PayoutTransactionStatusPending    PayoutTransactionStatus = "pending"
	PayoutTransactionStatusProcessing PayoutTransactionStatus = "processing"
	PayoutTransactionStatusCompleted  PayoutTransactionStatus = "completed"
	PayoutTransactionStatusFailed     PayoutTransactionStatus = "failed"
	PayoutTransactionStatusRefunded   PayoutTransactionStatus = "refunded"
)

// ============================================================================
// JSONB TYPES
// ============================================================================

// JSONB is a custom type for storing JSON data in PostgreSQL
type JSONB map[string]interface{}

// Value implements the driver.Valuer interface
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// ============================================================================
// MODELS
// ============================================================================

// GroupAccount represents a group savings account
type GroupAccount struct {
	gorm.Model
	Name        string             `json:"name" gorm:"type:varchar(255);not null"`
	Description string             `json:"description" gorm:"type:text"`
	AdminID     uint               `json:"admin_id" gorm:"not null;index"`
	Status      GroupAccountStatus `json:"status" gorm:"type:varchar(50);not null;default:'active';index"`
	Metadata    JSONB              `json:"metadata" gorm:"type:jsonb"`

	// Relationships
	Admin         User                   `json:"admin" gorm:"foreignKey:AdminID"`
	Members       []GroupMember          `json:"members" gorm:"foreignKey:GroupID"`
	Contributions []Contribution         `json:"contributions" gorm:"foreignKey:GroupID"`
}

// GroupMember represents a member of a group account
type GroupMember struct {
	gorm.Model
	GroupID      uint              `json:"group_id" gorm:"not null;index"`
	UserID       uint              `json:"user_id" gorm:"not null;index"`
	Role         GroupMemberRole   `json:"role" gorm:"type:varchar(50);not null;default:'member'"`
	Status       GroupMemberStatus `json:"status" gorm:"type:varchar(50);not null;default:'active';index"`
	Permissions  JSONB             `json:"permissions" gorm:"type:jsonb"`

	// Relationships
	User  User         `json:"user" gorm:"foreignKey:UserID"`
	Group GroupAccount `json:"group" gorm:"foreignKey:GroupID"`
}

// Contribution represents a group contribution/savings goal
type Contribution struct {
	gorm.Model
	GroupID         uint               `json:"group_id" gorm:"not null;index"`
	Title           string             `json:"title" gorm:"type:varchar(255);not null"`
	Description     string             `json:"description" gorm:"type:text"`
	TargetAmount    int64              `json:"target_amount" gorm:"not null"`
	CurrentAmount   int64              `json:"current_amount" gorm:"not null;default:0"`
	Currency        string             `json:"currency" gorm:"type:varchar(10);not null;default:'USD'"`
	Deadline        time.Time          `json:"deadline" gorm:"not null"`
	Status          ContributionStatus `json:"status" gorm:"type:varchar(50);not null;default:'active';index"`
	CreatedBy       uint               `json:"created_by" gorm:"not null;index"`
	Metadata        JSONB              `json:"metadata" gorm:"type:jsonb"`

	// Type-specific fields
	Type          ContributionType       `json:"type" gorm:"type:varchar(50);not null;default:'one_time'"`
	Frequency     *ContributionFrequency `json:"frequency" gorm:"type:varchar(50)"`
	RegularAmount *int64                 `json:"regular_amount"`
	NextPaymentDate *time.Time           `json:"next_payment_date" gorm:"index"`
	StartDate       *time.Time           `json:"start_date"`
	TotalCycles     *int                 `json:"total_cycles"`
	CurrentCycle    *int                 `json:"current_cycle" gorm:"default:1"`

	// Payout/Rotation fields (for rotating savings)
	CurrentPayoutRecipient *uint      `json:"current_payout_recipient" gorm:"index"`
	NextPayoutDate         *time.Time `json:"next_payout_date" gorm:"index"`

	// Payment settings
	AutoPayEnabled       bool   `json:"auto_pay_enabled" gorm:"default:false"`
	PenaltyAmount        *int64 `json:"penalty_amount"`
	GracePeriodDays      *int   `json:"grace_period_days"`
	AllowPartialPayments bool   `json:"allow_partial_payments" gorm:"default:true"`
	MinimumBalance       *int64 `json:"minimum_balance"`

	// Relationships
	Group            GroupAccount          `json:"group" gorm:"foreignKey:GroupID"`
	Creator          User                  `json:"creator" gorm:"foreignKey:CreatedBy"`
	Payments         []ContributionPayment `json:"payments" gorm:"foreignKey:ContributionID"`
	PayoutSchedules  []PayoutSchedule      `json:"payout_schedules" gorm:"foreignKey:ContributionID"`
	PayoutHistory    []PayoutTransaction   `json:"payout_history" gorm:"foreignKey:ContributionID"`
}

// ContributionPayment represents a payment made to a contribution
type ContributionPayment struct {
	gorm.Model
	ContributionID uint          `json:"contribution_id" gorm:"not null;index"`
	GroupID        uint          `json:"group_id" gorm:"not null;index"`
	UserID         uint          `json:"user_id" gorm:"not null;index"`
	Amount         int64         `json:"amount" gorm:"not null"`
	Currency       string        `json:"currency" gorm:"type:varchar(10);not null"`
	PaymentDate    time.Time     `json:"payment_date" gorm:"not null;index"`
	Status         PaymentStatus `json:"status" gorm:"type:varchar(50);not null;default:'pending';index"`
	TransactionID  *string       `json:"transaction_id" gorm:"type:varchar(255);uniqueIndex"`
	ReceiptID      *string       `json:"receipt_id" gorm:"type:varchar(255)"`
	Notes          *string       `json:"notes" gorm:"type:text"`
	Metadata       JSONB         `json:"metadata" gorm:"type:jsonb"`

	// Relationships
	Contribution Contribution `json:"contribution" gorm:"foreignKey:ContributionID"`
	Group        GroupAccount `json:"group" gorm:"foreignKey:GroupID"`
	User         User         `json:"user" gorm:"foreignKey:UserID"`
}

// PayoutSchedule represents the payout rotation schedule for rotating savings
type PayoutSchedule struct {
	gorm.Model
	ContributionID uint         `json:"contribution_id" gorm:"not null;index"`
	UserID         uint         `json:"user_id" gorm:"not null;index"`
	Position       int          `json:"position" gorm:"not null"` // Position in rotation
	ScheduledDate  time.Time    `json:"scheduled_date" gorm:"not null;index"`
	ExpectedAmount int64        `json:"expected_amount" gorm:"not null"`
	Status         PayoutStatus `json:"status" gorm:"type:varchar(50);not null;default:'pending';index"`
	ReceivedDate   *time.Time   `json:"received_date"`
	ActualAmount   *int64       `json:"actual_amount"`
	Notes          *string      `json:"notes" gorm:"type:text"`

	// Relationships
	Contribution Contribution `json:"contribution" gorm:"foreignKey:ContributionID"`
	User         User         `json:"user" gorm:"foreignKey:UserID"`
}

// PayoutTransaction represents a completed payout transaction
type PayoutTransaction struct {
	gorm.Model
	ContributionID   uint                    `json:"contribution_id" gorm:"not null;index"`
	GroupID          uint                    `json:"group_id" gorm:"not null;index"`
	RecipientUserID  uint                    `json:"recipient_user_id" gorm:"not null;index"`
	Amount           int64                   `json:"amount" gorm:"not null"`
	Currency         string                  `json:"currency" gorm:"type:varchar(10);not null"`
	PayoutDate       time.Time               `json:"payout_date" gorm:"not null;index"`
	Status           PayoutTransactionStatus `json:"status" gorm:"type:varchar(50);not null;default:'pending';index"`
	TransactionID    *string                 `json:"transaction_id" gorm:"type:varchar(255);uniqueIndex"`
	PaymentMethod    *string                 `json:"payment_method" gorm:"type:varchar(100)"`
	FailureReason    *string                 `json:"failure_reason" gorm:"type:text"`
	Metadata         JSONB                   `json:"metadata" gorm:"type:jsonb"`

	// Relationships
	Contribution  Contribution `json:"contribution" gorm:"foreignKey:ContributionID"`
	Group         GroupAccount `json:"group" gorm:"foreignKey:GroupID"`
	RecipientUser User         `json:"recipient_user" gorm:"foreignKey:RecipientUserID"`
}

// ContributionReceipt represents a receipt for a contribution payment
type ContributionReceipt struct {
	gorm.Model
	PaymentID      uint      `json:"payment_id" gorm:"not null;uniqueIndex"`
	ContributionID uint      `json:"contribution_id" gorm:"not null;index"`
	GroupID        uint      `json:"group_id" gorm:"not null;index"`
	UserID         uint      `json:"user_id" gorm:"not null;index"`
	Amount        int64     `json:"amount" gorm:"not null"`
	Currency      string    `json:"currency" gorm:"type:varchar(10);not null"`
	PaymentDate   time.Time `json:"payment_date" gorm:"not null"`
	GeneratedAt   time.Time `json:"generated_at" gorm:"not null;index"`
	ReceiptNumber string    `json:"receipt_number" gorm:"type:varchar(100);uniqueIndex"`
	ReceiptData   JSONB     `json:"receipt_data" gorm:"type:jsonb"`

	// Relationships
	Payment      ContributionPayment `json:"payment" gorm:"foreignKey:PaymentID"`
	Contribution Contribution        `json:"contribution" gorm:"foreignKey:ContributionID"`
	Group        GroupAccount        `json:"group" gorm:"foreignKey:GroupID"`
	User         User                `json:"user" gorm:"foreignKey:UserID"`
}

// ============================================================================
// TABLE NAMES (Optional - for custom table names)
// ============================================================================

func (GroupAccount) TableName() string {
	return "group_accounts"
}

func (GroupMember) TableName() string {
	return "group_members"
}

func (Contribution) TableName() string {
	return "contributions"
}

func (ContributionPayment) TableName() string {
	return "contribution_payments"
}

func (PayoutSchedule) TableName() string {
	return "payout_schedules"
}

func (PayoutTransaction) TableName() string {
	return "payout_transactions"
}

func (ContributionReceipt) TableName() string {
	return "contribution_receipts"
}

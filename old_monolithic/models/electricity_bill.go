package models

import (
	"time"
)

// ElectricityProvider represents a cached provider from payment gateways
type ElectricityProvider struct {
	ID             string    `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	ProviderCode   string    `json:"provider_code" gorm:"size:50;uniqueIndex;not null"`
	ProviderName   string    `json:"provider_name" gorm:"size:200;not null"`
	Country        string    `json:"country" gorm:"size:10;not null;index"`
	LogoURL        string    `json:"logo_url" gorm:"type:text"`
	IsActive       bool      `json:"is_active" gorm:"default:true;index"`
	PaymentGateway string    `json:"payment_gateway" gorm:"size:50;not null"` // flutterwave, paystack
	MinAmount      float64   `json:"min_amount" gorm:"type:decimal(20,2)"`
	MaxAmount      float64   `json:"max_amount" gorm:"type:decimal(20,2)"`
	ServiceFee     float64   `json:"service_fee" gorm:"type:decimal(20,2)"` // Fixed or percentage
	FeeType        string    `json:"fee_type" gorm:"size:20"`               // fixed, percentage
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name
func (ElectricityProvider) TableName() string {
	return "electricity_providers"
}

// BillPaymentStatus represents the status of a bill payment
type BillPaymentStatus string

const (
	BillPaymentStatusPending    BillPaymentStatus = "pending"
	BillPaymentStatusProcessing BillPaymentStatus = "processing"
	BillPaymentStatusCompleted  BillPaymentStatus = "completed"
	BillPaymentStatusFailed     BillPaymentStatus = "failed"
)

// BillPayment represents a payment transaction record
type BillPayment struct {
	ID               string            `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID           uint              `json:"user_id" gorm:"not null;index"`
	ProviderID       string            `json:"provider_id" gorm:"type:uuid;not null;index"`
	ProviderCode     string            `json:"provider_code" gorm:"size:50;not null;index"`
	ProviderName     string            `json:"provider_name" gorm:"size:200;not null"`
	MeterNumber      string            `json:"meter_number" gorm:"size:100;not null;index"`
	CustomerName     string            `json:"customer_name" gorm:"size:200"`
	CustomerAddress  string            `json:"customer_address" gorm:"type:text"`
	Amount           float64           `json:"amount" gorm:"type:decimal(20,2);not null"`
	ServiceFee       float64           `json:"service_fee" gorm:"type:decimal(20,2)"`
	TotalAmount      float64           `json:"total_amount" gorm:"type:decimal(20,2);not null"`
	Currency         string            `json:"currency" gorm:"size:10;not null;default:'NGN'"`
	Status           BillPaymentStatus `json:"status" gorm:"size:20;not null;default:'pending';index"`
	PaymentGateway   string            `json:"payment_gateway" gorm:"size:50;not null"`
	GatewayReference string            `json:"gateway_reference" gorm:"size:200;index"`
	ReferenceNumber  string            `json:"reference_number" gorm:"size:100;uniqueIndex;not null"`
	Token            string            `json:"token" gorm:"type:text"` // Electricity token for prepaid
	Units            float64           `json:"units" gorm:"type:decimal(20,2)"`
	MeterType        string            `json:"meter_type" gorm:"size:20"` // prepaid, postpaid
	FailureReason    string            `json:"failure_reason" gorm:"type:text"`
	CreatedAt        time.Time         `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt        time.Time         `json:"updated_at" gorm:"autoUpdateTime"`
	CompletedAt      *time.Time        `json:"completed_at"`
	FailedAt         *time.Time        `json:"failed_at"`

	// Relations
	Provider *ElectricityProvider `json:"provider,omitempty" gorm:"foreignKey:ProviderID"`
	User     *User                `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name
func (BillPayment) TableName() string {
	return "bill_payments"
}

// BillBeneficiary represents saved meter numbers for quick payments
type BillBeneficiary struct {
	ID              string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID          uint       `json:"user_id" gorm:"not null;index"`
	ProviderID      string     `json:"provider_id" gorm:"type:uuid;not null"`
	ProviderCode    string     `json:"provider_code" gorm:"size:50;not null"`
	ProviderName    string     `json:"provider_name" gorm:"size:200;not null"`
	MeterNumber     string     `json:"meter_number" gorm:"size:100;not null"`
	CustomerName    string     `json:"customer_name" gorm:"size:200"`
	CustomerAddress string     `json:"customer_address" gorm:"type:text"`
	Nickname        string     `json:"nickname" gorm:"size:100"` // User's custom label
	MeterType       string     `json:"meter_type" gorm:"size:20"`
	IsDefault       bool       `json:"is_default" gorm:"default:false"`
	CreatedAt       time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
	LastUsedAt      *time.Time `json:"last_used_at"`

	// Relations
	Provider *ElectricityProvider `json:"provider,omitempty" gorm:"foreignKey:ProviderID"`
}

// TableName specifies the table name
func (BillBeneficiary) TableName() string {
	return "bill_beneficiaries"
}

// RechargeFrequency represents how often auto-recharge should run
type RechargeFrequency string

const (
	FrequencyDaily   RechargeFrequency = "daily"
	FrequencyWeekly  RechargeFrequency = "weekly"
	FrequencyMonthly RechargeFrequency = "monthly"
)

// AutoRechargeStatus represents the status of an auto-recharge configuration
type AutoRechargeStatus string

const (
	AutoRechargeStatusActive  AutoRechargeStatus = "active"
	AutoRechargeStatusPaused  AutoRechargeStatus = "paused"
	AutoRechargeStatusExpired AutoRechargeStatus = "expired"
)

// AutoRecharge represents scheduled recurring payment configurations
type AutoRecharge struct {
	ID            string             `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID        uint               `json:"user_id" gorm:"not null;index"`
	BeneficiaryID string             `json:"beneficiary_id" gorm:"type:uuid;not null"`
	ProviderID    string             `json:"provider_id" gorm:"type:uuid;not null"`
	ProviderCode  string             `json:"provider_code" gorm:"size:50;not null"`
	ProviderName  string             `json:"provider_name" gorm:"size:200;not null"`
	MeterNumber   string             `json:"meter_number" gorm:"size:100;not null"`
	CustomerName  string             `json:"customer_name" gorm:"size:200"`
	MeterType     string             `json:"meter_type" gorm:"size:20"`
	Amount        float64            `json:"amount" gorm:"type:decimal(20,2);not null"`
	Currency      string             `json:"currency" gorm:"size:10;not null;default:'NGN'"`
	Frequency     RechargeFrequency  `json:"frequency" gorm:"size:20;not null"`
	DayOfWeek     *int               `json:"day_of_week"`  // 0-6 for weekly
	DayOfMonth    *int               `json:"day_of_month"` // 1-31 for monthly
	NextRunDate   time.Time          `json:"next_run_date" gorm:"not null;index"`
	LastRunDate   *time.Time         `json:"last_run_date"`
	Status        AutoRechargeStatus `json:"status" gorm:"size:20;not null;default:'active';index"`
	FailureCount  int                `json:"failure_count" gorm:"default:0"`
	MaxRetries    int                `json:"max_retries" gorm:"default:3"`
	CreatedAt     time.Time          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time          `json:"updated_at" gorm:"autoUpdateTime"`

	// Relations
	Beneficiary *BillBeneficiary `json:"beneficiary,omitempty" gorm:"foreignKey:BeneficiaryID"`
}

// TableName specifies the table name
func (AutoRecharge) TableName() string {
	return "auto_recharges"
}

// ReminderStatus represents the status of a payment reminder
type ReminderStatus string

const (
	ReminderStatusActive    ReminderStatus = "active"
	ReminderStatusCompleted ReminderStatus = "completed"
	ReminderStatusCancelled ReminderStatus = "cancelled"
)

// PaymentReminder represents user-configured payment reminders
type PaymentReminder struct {
	ID             string         `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID         uint           `json:"user_id" gorm:"not null;index"`
	BeneficiaryID  *string        `json:"beneficiary_id" gorm:"type:uuid"` // Optional link
	Title          string         `json:"title" gorm:"size:200;not null"`
	Description    string         `json:"description" gorm:"type:text"`
	ReminderDate   time.Time      `json:"reminder_date" gorm:"not null;index"`
	Amount         *float64       `json:"amount" gorm:"type:decimal(20,2)"`
	Currency       string         `json:"currency" gorm:"size:10;default:'NGN'"`
	IsRecurring    bool           `json:"is_recurring" gorm:"default:false"`
	RecurrenceType *string        `json:"recurrence_type" gorm:"size:20"` // monthly, weekly
	Status         ReminderStatus `json:"status" gorm:"size:20;not null;default:'active';index"`
	NotifiedAt     *time.Time     `json:"notified_at"`
	CreatedAt      time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name
func (PaymentReminder) TableName() string {
	return "payment_reminders"
}

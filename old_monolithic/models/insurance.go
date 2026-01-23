package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Insurance represents an insurance policy
type Insurance struct {
	gorm.Model
	ID                string    `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	PolicyNumber      string    `json:"policy_number" gorm:"uniqueIndex;size:50;not null"`
	PolicyHolderName  string    `json:"policy_holder_name" gorm:"size:255;not null"`
	PolicyHolderEmail string    `json:"policy_holder_email" gorm:"size:255;not null"`
	PolicyHolderPhone string    `json:"policy_holder_phone" gorm:"size:20;not null"`
	Type              string    `json:"type" gorm:"size:20;not null;check:type IN ('health', 'auto', 'home', 'life', 'travel', 'business')"`
	Provider          string    `json:"provider" gorm:"size:255;not null"`
	ProviderLogo      string    `json:"provider_logo" gorm:"type:text"`
	PremiumAmount     float64   `json:"premium_amount" gorm:"type:decimal(15,2);not null"`
	CoverageAmount    float64   `json:"coverage_amount" gorm:"type:decimal(15,2);not null"`
	Currency          string    `json:"currency" gorm:"size:3;not null;default:'USD'"`
	StartDate         time.Time `json:"start_date" gorm:"not null"`
	EndDate           time.Time `json:"end_date" gorm:"not null"`
	NextPaymentDate   time.Time `json:"next_payment_date" gorm:"not null"`
	Status            string    `json:"status" gorm:"size:20;not null;default:'pending';check:status IN ('active', 'pending', 'expired', 'cancelled', 'suspended')"`
	Beneficiaries     JSON      `json:"beneficiaries" gorm:"type:jsonb;default:'[]'"`
	CoverageDetails   JSON      `json:"coverage_details" gorm:"type:jsonb;default:'{}'"`
	Description       *string   `json:"description" gorm:"type:text"`
	UserID            uint      `json:"user_id" gorm:"not null"`
	User              User      `json:"user" gorm:"foreignKey:UserID"`

	// Relationships
	Payments []InsurancePayment `json:"payments,omitempty" gorm:"foreignKey:InsuranceID"`
	Claims   []InsuranceClaim   `json:"claims,omitempty" gorm:"foreignKey:InsuranceID"`
}

// InsurancePayment represents a payment for an insurance policy
type InsurancePayment struct {
	gorm.Model
	ID              string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	InsuranceID     string     `json:"insurance_id" gorm:"type:uuid;not null"`
	PolicyNumber    string     `json:"policy_number" gorm:"size:50;not null"`
	Amount          float64    `json:"amount" gorm:"type:decimal(15,2);not null"`
	Currency        string     `json:"currency" gorm:"size:3;not null;default:'USD'"`
	PaymentMethod   string     `json:"payment_method" gorm:"size:20;not null;check:payment_method IN ('bank_transfer', 'card', 'mobile_money', 'crypto', 'wallet')"`
	Status          string     `json:"status" gorm:"size:20;not null;default:'pending';check:status IN ('pending', 'processing', 'completed', 'failed', 'cancelled', 'refunded')"`
	TransactionID   *string    `json:"transaction_id" gorm:"size:100"`
	ReferenceNumber *string    `json:"reference_number" gorm:"size:100"`
	PaymentDate     time.Time  `json:"payment_date" gorm:"not null;default:now()"`
	DueDate         time.Time  `json:"due_date" gorm:"not null"`
	ProcessedAt     *time.Time `json:"processed_at"`
	PaymentDetails  JSON       `json:"payment_details" gorm:"type:jsonb"`
	FailureReason   *string    `json:"failure_reason" gorm:"type:text"`
	ReceiptURL      *string    `json:"receipt_url" gorm:"type:text"`
	UserID          uint       `json:"user_id" gorm:"not null"`
	User            User       `json:"user" gorm:"foreignKey:UserID"`

	// Relationships
	Insurance Insurance `json:"insurance,omitempty" gorm:"foreignKey:InsuranceID"`
}

// InsuranceClaim represents a claim for an insurance policy
type InsuranceClaim struct {
	gorm.Model
	ID                string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	ClaimNumber       string     `json:"claim_number" gorm:"uniqueIndex;size:50;not null"`
	InsuranceID       string     `json:"insurance_id" gorm:"type:uuid;not null"`
	PolicyNumber      string     `json:"policy_number" gorm:"size:50;not null"`
	Type              string     `json:"type" gorm:"size:50;not null"`
	Status            string     `json:"status" gorm:"size:20;not null;default:'submitted';check:status IN ('submitted', 'under_review', 'approved', 'rejected', 'settled')"`
	Title             string     `json:"title" gorm:"size:255;not null"`
	Description       string     `json:"description" gorm:"type:text;not null"`
	ClaimAmount       float64    `json:"claim_amount" gorm:"type:decimal(15,2);not null"`
	ApprovedAmount    *float64   `json:"approved_amount" gorm:"type:decimal(15,2)"`
	Currency          string     `json:"currency" gorm:"size:3;not null;default:'USD'"`
	IncidentDate      time.Time  `json:"incident_date" gorm:"not null"`
	IncidentLocation  string     `json:"incident_location" gorm:"type:text;not null"`
	Attachments       JSON       `json:"attachments" gorm:"type:jsonb;default:'[]'"`
	Documents         JSON       `json:"documents" gorm:"type:jsonb;default:'[]'"`
	AdditionalInfo    JSON       `json:"additional_info" gorm:"type:jsonb;default:'{}'"`
	RejectionReason   *string    `json:"rejection_reason" gorm:"type:text"`
	SettlementDate    *time.Time `json:"settlement_date"`
	SettlementDetails *string    `json:"settlement_details" gorm:"type:text"`
	UserID            uint       `json:"user_id" gorm:"not null"`
	User              User       `json:"user" gorm:"foreignKey:UserID"`

	// Relationships
	Insurance Insurance `json:"insurance,omitempty" gorm:"foreignKey:InsuranceID"`
}

// InsuranceStatistics represents statistics for insurance policies
type InsuranceStatistics struct {
	TotalPolicies       int            `json:"total_policies"`
	ActivePolicies      int            `json:"active_policies"`
	ExpiredPolicies     int            `json:"expired_policies"`
	TotalCoverageAmount float64        `json:"total_coverage_amount"`
	TotalPremiumAmount  float64        `json:"total_premium_amount"`
	PoliciesByType      map[string]int `json:"policies_by_type"`
}

// PaymentStatistics represents statistics for insurance payments
type PaymentStatistics struct {
	TotalPayments     int            `json:"total_payments"`
	CompletedPayments int            `json:"completed_payments"`
	PendingPayments   int            `json:"pending_payments"`
	FailedPayments    int            `json:"failed_payments"`
	TotalAmount       float64        `json:"total_amount"`
	CompletedAmount   float64        `json:"completed_amount"`
	PaymentsByMethod  map[string]int `json:"payments_by_method"`
}

// PaginationInfo represents pagination information
type PaginationInfo struct {
	CurrentPage  int32 `json:"current_page"`
	TotalPages   int32 `json:"total_pages"`
	TotalItems   int32 `json:"total_items"`
	ItemsPerPage int32 `json:"items_per_page"`
	HasNext      bool  `json:"has_next"`
	HasPrev      bool  `json:"has_prev"`
}

// JSON type for handling JSONB fields
type JSON json.RawMessage

// Value implements the driver.Valuer interface
func (j JSON) Value() (driver.Value, error) {
	if j.IsNull() {
		return nil, nil
	}
	return string(j), nil
}

// Scan implements the sql.Scanner interface
func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	s, ok := value.([]byte)
	if !ok {
		return errors.New("invalid scan source")
	}
	*j = append((*j)[0:0], s...)
	return nil
}

// MarshalJSON returns j as the JSON encoding of j.
func (j JSON) MarshalJSON() ([]byte, error) {
	if j.IsNull() {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON sets *j to a copy of data.
func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return errors.New("null point exception")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

// IsNull returns true if j is null
func (j JSON) IsNull() bool {
	return len(j) == 0 || string(j) == "null"
}

// String returns the string representation of j
func (j JSON) String() string {
	return string(j)
}

// TableName returns the table name for Insurance
func (Insurance) TableName() string {
	return "insurances"
}

// TableName returns the table name for InsurancePayment
func (InsurancePayment) TableName() string {
	return "insurance_payments"
}

// TableName returns the table name for InsuranceClaim
func (InsuranceClaim) TableName() string {
	return "insurance_claims"
}

// BeforeCreate hook for Insurance
func (i *Insurance) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	if i.PolicyNumber == "" {
		i.PolicyNumber = generatePolicyNumber(i.Type)
	}
	return nil
}

// BeforeCreate hook for InsurancePayment
func (ip *InsurancePayment) BeforeCreate(tx *gorm.DB) error {
	if ip.ID == "" {
		ip.ID = uuid.New().String()
	}
	return nil
}

// BeforeCreate hook for InsuranceClaim
func (ic *InsuranceClaim) BeforeCreate(tx *gorm.DB) error {
	if ic.ID == "" {
		ic.ID = uuid.New().String()
	}
	if ic.ClaimNumber == "" {
		ic.ClaimNumber = generateClaimNumber()
	}
	return nil
}

// Helper function to generate policy number
func generatePolicyNumber(insuranceType string) string {
	return "POL-" + insuranceType + "-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
}

// Helper function to generate claim number
func generateClaimNumber() string {
	return "CLM-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
}

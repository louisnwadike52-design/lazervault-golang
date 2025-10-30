package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Account Type (using constants from account.go)
type AccountType string

// JSON Map for storing metadata
type PaymentMetadataMap map[string]interface{}

func (j PaymentMetadataMap) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *PaymentMetadataMap) Scan(value interface{}) error {
	if value == nil {
		*j = make(PaymentMetadataMap)
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, j)
	case string:
		return json.Unmarshal([]byte(v), j)
	}
	return nil
}

// Invoice Payment Transaction Model
type InvoicePaymentTransaction struct {
	ID                 uuid.UUID            `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	TransactionID      string               `json:"transaction_id" gorm:"size:255;uniqueIndex;not null"`
	InvoiceID          string               `json:"invoice_id" gorm:"type:uuid;not null;index"`
	UserID             string               `json:"user_id" gorm:"type:uuid;not null;index"`
	Amount             float64              `json:"amount" gorm:"type:decimal(15,2);not null"`
	Currency           string               `json:"currency" gorm:"size:10;not null"`
	PaymentMethod      PaymentMethodType    `json:"payment_method" gorm:"size:50;not null"`
	Status             InvoicePaymentStatus `json:"status" gorm:"size:50;not null"`
	Reference          string               `json:"reference" gorm:"size:255"`
	Description        string               `json:"description" gorm:"type:text"`
	FeeAmount          float64              `json:"fee_amount" gorm:"type:decimal(15,2);default:0"`
	PaymentProcessorID string               `json:"payment_processor_id" gorm:"size:255"`
	ReceiptURL         string               `json:"receipt_url" gorm:"size:500"`
	ConfirmationCode   string               `json:"confirmation_code" gorm:"size:100"`
	Metadata           PaymentMetadataMap   `json:"metadata" gorm:"type:jsonb"`
	ProcessedAt        *time.Time           `json:"processed_at"`
	CreatedAt          time.Time            `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          time.Time            `json:"updated_at" gorm:"autoUpdateTime"`
}

// User Account Balance Model
type UserAccountBalance struct {
	ID               uuid.UUID   `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	UserID           string      `json:"user_id" gorm:"type:uuid;not null;index"`
	Currency         string      `json:"currency" gorm:"size:10;not null"`
	AvailableBalance float64     `json:"available_balance" gorm:"type:decimal(15,2);not null;default:0"`
	PendingBalance   float64     `json:"pending_balance" gorm:"type:decimal(15,2);not null;default:0"`
	TotalBalance     float64     `json:"total_balance" gorm:"type:decimal(15,2);not null;default:0"`
	AccountType      AccountType `json:"account_type" gorm:"size:50;not null"`
	AccountNumber    string      `json:"account_number" gorm:"size:100;uniqueIndex"`
	AccountName      string      `json:"account_name" gorm:"size:255"`
	IsPrimary        bool        `json:"is_primary" gorm:"default:false"`
	IsActive         bool        `json:"is_active" gorm:"default:true"`
	CreatedAt        time.Time   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        time.Time   `json:"updated_at" gorm:"autoUpdateTime"`
	LastUpdated      time.Time   `json:"last_updated" gorm:"autoUpdateTime"`
}

// Payment Method Model
type UserPaymentMethod struct {
	ID             uuid.UUID          `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	UserID         string             `json:"user_id" gorm:"type:uuid;not null;index"`
	MethodID       string             `json:"method_id" gorm:"size:255;uniqueIndex;not null"`
	Type           PaymentMethodType  `json:"type" gorm:"size:50;not null"`
	DisplayName    string             `json:"display_name" gorm:"size:255;not null"`
	Last4          string             `json:"last4" gorm:"size:4"`
	Brand          string             `json:"brand" gorm:"size:50"`
	IsDefault      bool               `json:"is_default" gorm:"default:false"`
	IsVerified     bool               `json:"is_verified" gorm:"default:false"`
	BillingAddress string             `json:"billing_address" gorm:"type:text"`
	Metadata       PaymentMetadataMap `json:"metadata" gorm:"type:jsonb"`
	ExpiresAt      *time.Time         `json:"expires_at"`
	CreatedAt      time.Time          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time          `json:"updated_at" gorm:"autoUpdateTime"`
}

// Tagged Invoice Model (for recipients)
type TaggedInvoice struct {
	ID            uuid.UUID            `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	InvoiceID     string               `json:"invoice_id" gorm:"type:uuid;not null;index"`
	UserID        string               `json:"user_id" gorm:"type:uuid;not null;index"`
	Priority      string               `json:"priority" gorm:"size:50;default:'medium'"`
	PaymentStatus InvoicePaymentStatus `json:"payment_status" gorm:"size:50;not null;default:'pending'"`

	// Tracking Information
	IsViewed     bool       `json:"is_viewed" gorm:"default:false"`
	ViewedAt     *time.Time `json:"viewed_at"`
	TaggedAt     time.Time  `json:"tagged_at" gorm:"autoCreateTime"`
	ReminderDate *time.Time `json:"reminder_date"`
	ReminderSent bool       `json:"reminder_sent" gorm:"default:false"`

	// Additional Information
	Notes    string             `json:"notes" gorm:"type:text"`
	Metadata PaymentMetadataMap `json:"metadata" gorm:"type:jsonb"`

	// Audit Information
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// Payment Dispute Model
type PaymentDispute struct {
	ID            uuid.UUID     `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	DisputeID     string        `json:"dispute_id" gorm:"size:255;uniqueIndex;not null"`
	TransactionID string        `json:"transaction_id" gorm:"size:255;not null;index"`
	InvoiceID     string        `json:"invoice_id" gorm:"type:uuid;not null;index"`
	UserID        string        `json:"user_id" gorm:"type:uuid;not null;index"`
	Reason        string        `json:"reason" gorm:"size:255;not null"`
	Description   string        `json:"description" gorm:"type:text;not null"`
	Status        DisputeStatus `json:"status" gorm:"size:50;not null;default:'pending'"`
	Amount        float64       `json:"amount" gorm:"type:decimal(15,2);not null"`
	Currency      string        `json:"currency" gorm:"size:10;not null"`
	EvidenceFiles string        `json:"evidence_files" gorm:"type:text"`
	Resolution    string        `json:"resolution" gorm:"type:text"`
	ResolvedAt    *time.Time    `json:"resolved_at"`
	CreatedAt     time.Time     `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time     `json:"updated_at" gorm:"autoUpdateTime"`
}

// Table names
func (InvoicePaymentTransaction) TableName() string { return "invoice_payment_transactions" }
func (UserAccountBalance) TableName() string        { return "user_account_balances" }
func (UserPaymentMethod) TableName() string         { return "user_payment_methods" }
func (TaggedInvoice) TableName() string             { return "tagged_invoices" }
func (PaymentDispute) TableName() string            { return "payment_disputes" }

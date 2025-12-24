package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// CustomerDetails holds customer info for embedding in Invoice
type CustomerDetails struct {
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Address string `json:"address,omitempty"`
}

// AddressDetails holds address information for invoice participants
type AddressDetails struct {
	CompanyName  string `json:"company_name,omitempty"`
	ContactName  string `json:"contact_name,omitempty"`
	Email        string `json:"email,omitempty"`
	Phone        string `json:"phone,omitempty"`
	AddressLine1 string `json:"address_line1,omitempty"`
	AddressLine2 string `json:"address_line2,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	Postcode     string `json:"postcode,omitempty"`
	Country      string `json:"country,omitempty"`
}

// InvoiceItem holds line item info for embedding in Invoice
type InvoiceItem struct {
	ItemID      string  `json:"id,omitempty"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	TotalPrice  float64 `json:"total_price"`
	Category    string  `json:"category,omitempty"`
}

// Invoice represents an invoice in the system
type Invoice struct {
	ID               string               `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID           string               `json:"user_id" gorm:"type:uuid;not null"`
	RecipientID      string               `json:"recipient_id" gorm:"type:uuid;not null"`
	Title            string               `json:"title" gorm:"size:255;not null"`
	Description      string               `json:"description" gorm:"type:text"`
	Amount           float64              `json:"amount" gorm:"type:decimal(15,2);not null"`
	Currency         string               `json:"currency" gorm:"size:10;not null"`
	DueDate          time.Time            `json:"due_date"`
	IsPaid           bool                 `json:"is_paid" gorm:"default:false"`
	PaymentMethodID  *string              `json:"payment_method_id" gorm:"type:uuid"`
	PaymentReference string               `json:"payment_reference" gorm:"size:255"`
	Status           InvoicePaymentStatus `json:"status" gorm:"size:50;not null;default:'pending'"`
	Metadata         datatypes.JSON       `json:"metadata" gorm:"type:jsonb"`
	Items            datatypes.JSON       `json:"items" gorm:"type:jsonb"`
	Notes            string               `json:"notes" gorm:"type:text"`
	TaxAmount        float64              `json:"tax_amount" gorm:"type:decimal(15,2);default:0"`
	DiscountAmount   float64              `json:"discount_amount" gorm:"type:decimal(15,2);default:0"`
	TotalAmount      float64              `json:"total_amount" gorm:"type:decimal(15,2);not null;default:0"`
	RecipientDetails datatypes.JSON       `json:"recipient_details" gorm:"type:jsonb"`
	PayerDetails     datatypes.JSON       `json:"payer_details" gorm:"type:jsonb"`
	ToEmail          string               `json:"to_email" gorm:"size:255"`
	ToName           string               `json:"to_name" gorm:"size:255"`
	CreatedAt        time.Time            `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        time.Time            `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate hook to ensure ID is set
func (i *Invoice) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return nil
}

// TableName specifies the table name for Invoice
func (i *Invoice) TableName() string {
	return "invoices"
}

// --- JSON Scan/Value Helpers ---

// Scan implements the sql.Scanner interface for CustomerDetails
func (cd *CustomerDetails) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed for CustomerDetails")
	}
	return json.Unmarshal(bytes, &cd)
}

// Value implements the driver.Valuer interface for CustomerDetails
func (cd CustomerDetails) Value() (driver.Value, error) {
	return json.Marshal(cd)
}

// Scan implements the sql.Scanner interface for AddressDetails
func (ad *AddressDetails) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed for AddressDetails")
	}
	return json.Unmarshal(bytes, &ad)
}

// Value implements the driver.Valuer interface for AddressDetails
func (ad AddressDetails) Value() (driver.Value, error) {
	return json.Marshal(ad)
}

// Scan implements the sql.Scanner interface for []InvoiceItem
// Note: GORM's datatypes.JSON handles slice/map scanning/valuing automatically,
// so these custom Scan/Value methods for the *slice* might not be strictly necessary
// if using datatypes.JSON directly on the field. Keeping them for potential future use
// or if not using datatypes.JSON.
// func (ii *[]InvoiceItem) Scan(value interface{}) error {
// 	bytes, ok := value.([]byte)
// 	if !ok {
// 		return errors.New("type assertion to []byte failed for InvoiceItems")
// 	}
// 	return json.Unmarshal(bytes, &ii)
// }

// Value implements the driver.Valuer interface for []InvoiceItem
// func (ii []InvoiceItem) Value() (driver.Value, error) {
// 	return json.Marshal(ii)
// }

// --- Helper Functions (Example) ---

// generateInvoiceNumber generates a simple sequential invoice number (replace with robust logic)
// Warning: This is NOT concurrency-safe. Needs proper sequence generation.
func generateInvoiceNumber(db *gorm.DB, userID string) (string, error) {
	var count int64
	if err := db.Model(&Invoice{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return "", fmt.Errorf("failed to count existing invoices: %w", err)
	}
	return fmt.Sprintf("INV-%04d", count+1), nil
}

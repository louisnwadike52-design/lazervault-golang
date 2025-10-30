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

// InvoiceItem holds line item info for embedding in Invoice
type InvoiceItem struct {
	ItemID      string  `json:"item_id,omitempty"`
	Description string  `json:"description"`
	Quantity    int32   `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	TotalPrice  float64 `json:"total_price"`
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

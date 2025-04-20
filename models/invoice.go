package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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

// Invoice represents an invoice document in the database
type Invoice struct {
	ID              string         `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID          string         `gorm:"type:uuid;not null;index"`                                   // User who issued the invoice
	InvoiceNumber   string         `gorm:"type:varchar(100);not null;uniqueIndex:idx_user_invoice_no"` // Must be unique per user
	CustomerDetails datatypes.JSON `gorm:"type:jsonb;not null"`
	Items           datatypes.JSON `gorm:"type:jsonb;not null"` // Store array of InvoiceItem as JSON
	Subtotal        float64        `gorm:"not null"`
	Tax             float64        `gorm:"default:0.0"`
	TotalAmount     float64        `gorm:"not null"`
	CurrencyCode    string         `gorm:"type:varchar(10);not null"`
	IssueDate       time.Time      `gorm:"not null"`
	DueDate         time.Time      `gorm:"not null;index"`
	Status          string         `gorm:"type:varchar(20);not null;index"` // e.g., DRAFT, SENT, PAID
	Notes           string         `gorm:"type:text"`
	CreatedAt       time.Time      `gorm:"default:CURRENT_TIMESTAMP;index"`
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`

	// User User `gorm:"foreignKey:UserID"` // Optional relation
}

// TableName specifies the table name
func (Invoice) TableName() string {
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

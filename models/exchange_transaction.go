package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ReceiverDetails holds receiver info for embedding
// Note: Using JSONB is simple but less queryable than a separate table.
type ReceiverDetails struct {
	FullName      string `json:"full_name"`
	AccountNumber string `json:"account_number"`
	BankName      string `json:"bank_name"`
	SwiftBicCode  string `json:"swift_bic_code"`
	// Address string `json:"address,omitempty"`
	// Country string `json:"country,omitempty"`
}

// ExchangeTransaction represents an international transfer in the database
type ExchangeTransaction struct {
	ID              string         `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID          string         `gorm:"type:uuid;not null;index"` // The user who initiated the transfer
	FromCurrency    string         `gorm:"type:varchar(10);not null"`
	ToCurrency      string         `gorm:"type:varchar(10);not null"`
	AmountFrom      float64        `gorm:"not null"`
	AmountTo        float64        `gorm:"not null"` // Calculated amount receiver gets
	ExchangeRate    float64        `gorm:"not null"`
	Fees            float64        `gorm:"default:0.0"`
	ReceiverDetails datatypes.JSON `gorm:"type:jsonb"`                      // Store ReceiverDetails as JSON
	Status          string         `gorm:"type:varchar(20);not null;index"` // e.g., PENDING, COMPLETED, FAILED
	CreatedAt       time.Time      `gorm:"default:CURRENT_TIMESTAMP;index"`
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`

	// User User `gorm:"foreignKey:UserID"` // Optional relation to User model
}

// TableName specifies the table name
func (ExchangeTransaction) TableName() string {
	return "exchange_transactions"
}

// Helper methods for ReceiverDetails JSON handling (optional but helpful)

// Scan implements the sql.Scanner interface for ReceiverDetails
func (rd *ReceiverDetails) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed for ReceiverDetails")
	}
	return json.Unmarshal(bytes, &rd)
}

// Value implements the driver.Valuer interface for ReceiverDetails
func (rd ReceiverDetails) Value() (driver.Value, error) {
	return json.Marshal(rd)
}

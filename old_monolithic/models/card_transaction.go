package models

import (
	"time"

	"gorm.io/gorm"
)

// CardTransaction represents a transaction made using a card
type CardTransaction struct {
	gorm.Model
	UUID              string     `gorm:"type:uuid;default:gen_random_uuid();uniqueIndex" json:"uuid"`
	CardID            uint       `gorm:"not null;index" json:"card_id"`            // Foreign key to AccountCard
	UserID            uint       `gorm:"not null;index" json:"user_id"`            // Foreign key to User
	AccountID         uint       `gorm:"not null;index" json:"account_id"`         // Foreign key to Account
	Amount            float64    `gorm:"not null" json:"amount"`                   // Transaction amount
	Currency          string     `gorm:"size:3;not null" json:"currency"`          // Currency code
	MerchantName      string     `gorm:"size:255" json:"merchant_name"`            // Merchant name
	MerchantCategory  string     `gorm:"size:100" json:"merchant_category"`        // Merchant category
	TransactionType   string     `gorm:"size:50;not null" json:"transaction_type"` // purchase, refund, reversal
	Status            string     `gorm:"size:20;not null;index" json:"status"`     // pending, completed, failed, declined
	DeclineReason     string     `gorm:"type:text" json:"decline_reason"`          // Reason for decline
	AuthorizationCode string     `gorm:"size:50" json:"authorization_code"`        // Authorization code
	Description       string     `gorm:"type:text" json:"description"`             // Transaction description
	TransactionDate   time.Time  `gorm:"not null;index" json:"transaction_date"`   // Date of transaction
	SettledAt         *time.Time `json:"settled_at"`                               // Settlement date
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// TableName specifies the database table name for the CardTransaction model
func (CardTransaction) TableName() string {
	return "card_transactions"
}

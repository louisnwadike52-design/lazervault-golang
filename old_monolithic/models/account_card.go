package models

import (
	"time"

	"gorm.io/gorm"
)

// CardType defines the nature of the card.
const (
	CardTypeVirtual    = "Virtual"
	CardTypeDisposable = "Disposable"
	CardTypePermanent  = "Permanent" // Or Physical
)

// AccountCard represents a specific debit/credit card linked to a user's Account.
type AccountCard struct {
	gorm.Model
	UUID            string         `gorm:"type:uuid;default:gen_random_uuid();uniqueIndex" json:"uuid"`
	AccountID       uint           `gorm:"not null;index" json:"account_id"`             // Foreign key to the Account
	UserID          uint           `gorm:"not null;index" json:"user_id"`                // Foreign key to the User
	CardType        string         `gorm:"not null;index" json:"card_type"`              // Virtual, Disposable, Permanent
	CardHolderName  string         `gorm:"not null" json:"card_holder_name"`             // Name as it appears on the card
	CardNumber      string         `gorm:"not null" json:"card_number"`                  // Securely stored card number (e.g., encrypted)
	CardExpiry      string         `gorm:"not null;size:5" json:"card_expiry"`           // Expiry date (e.g., "MM/YY" like "12/25")
	CVV             string         `gorm:"size:3" json:"cvv"`                            // CVV (encrypted, for virtual/disposable cards)
	Brand           string         `gorm:"" json:"brand"`                                // Card brand (e.g., Visa, Mastercard)
	Last4           string         `gorm:"size:4" json:"last4"`                          // Last 4 digits of the card number
	IsActive        bool           `gorm:"not null;default:true;index" json:"is_active"` // Whether the card is active
	IsDefault       bool           `gorm:"not null;default:false" json:"is_default"`     // Whether this is the default card for the account
	CardNickname    string         `gorm:"size:100" json:"card_nickname"`                // Optional nickname for the card
	SpendingLimit   float64        `gorm:"default:0" json:"spending_limit"`              // Spending limit for disposable cards
	RemainingLimit  float64        `gorm:"default:0" json:"remaining_limit"`             // Remaining spending limit
	ExpiresAt       *time.Time     `gorm:"index" json:"expires_at"`                      // Expiration for disposable cards
	UsageCount      int            `gorm:"default:0" json:"usage_count"`                 // Number of times card has been used
	MaxUsageCount   int            `gorm:"default:0" json:"max_usage_count"`             // Max usage count for disposable cards (0 = unlimited)
	Currency        string         `gorm:"size:3;default:'USD'" json:"currency"`         // Currency code (USD, GBP, etc.)
	BillingAddress  string         `gorm:"type:text" json:"billing_address"`             // Billing address for the card
	Status          string         `gorm:"size:20;default:'active';index" json:"status"` // active, frozen, cancelled, expired
	FrozenReason    string         `gorm:"type:text" json:"frozen_reason"`               // Reason for freezing the card
	CancelledReason string         `gorm:"type:text" json:"cancelled_reason"`            // Reason for cancelling the card
	LastUsedAt      *time.Time     `json:"last_used_at"`                                 // Last time card was used
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	// IMPORTANT SECURITY NOTE:
	// Storing CardNumber and CVV requires strong encryption and security practices.
	// Consider tokenization or only storing Last4 if full PAN is not needed.
	// CVV should be encrypted and only decrypted when absolutely necessary.
}

// TableName specifies the database table name for the AccountCard model.
func (AccountCard) TableName() string {
	return "account_cards"
}

// BeforeSave Hook (Example): Automatically populate Last4 before saving
func (card *AccountCard) BeforeSave(tx *gorm.DB) (err error) {
	if len(card.CardNumber) >= 4 {
		// Ensure we only use the raw card number briefly before it's encrypted/discarded by the service
		card.Last4 = card.CardNumber[len(card.CardNumber)-4:]
	} else {
		card.Last4 = ""
	}
	// Encryption logic is handled in the service layer
	return
}

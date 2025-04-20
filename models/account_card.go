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
	AccountID      uint      `gorm:"not null;index" json:"account_id"`         // Foreign key to the Account
	CardType       string    `gorm:"not null" json:"card_type"`                // Virtual, Disposable, Permanent
	CardHolderName string    `gorm:"not null" json:"card_holder_name"`         // Name as it appears on the card
	CardNumber     string    `gorm:"not null" json:"card_number"`              // Securely stored card number (e.g., encrypted)
	CardExpiry     string    `gorm:"not null;size:5" json:"card_expiry"`       // Expiry date (e.g., "MM/YY" like "12/25")
	Brand          string    `gorm:"" json:"brand"`                            // Card brand (e.g., Visa, Mastercard) - Optional
	Last4          string    `gorm:"size:4" json:"last4"`                      // Last 4 digits of the card number - Useful for display
	IsActive       bool      `gorm:"not null;default:true" json:"is_active"`   // Whether the card is active
	IsDefault      bool      `gorm:"not null;default:false" json:"is_default"` // Whether this is the default card for the account
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// IMPORTANT SECURITY NOTE:
	// Storing CardNumber requires strong encryption and security practices.
	// Consider tokenization or only storing Last4 if full PAN is not needed.
	// Storing CVV/CVC is generally NOT recommended due to security risks and PCI DSS rules.
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

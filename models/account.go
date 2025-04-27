package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Constants for Account Types
const (
	AccountTypePersonal   = "personal"
	AccountTypeSavings    = "savings"
	AccountTypeInvestment = "investment"
)

// Constants for Account Status
const (
	AccountStatusActive           = "active"
	AccountStatusBlockedTemporary = "blocked_temporary"
	AccountStatusBlockedPermanent = "blocked_permanent"
	AccountStatusBlockedStolen    = "blocked_stolen" // Added specific status for stolen
)

// Account represents a user's financial account within LazerVault.
type Account struct {
	gorm.Model           // Includes ID, CreatedAt, UpdatedAt, DeletedAt
	OwnerUserID   uint   `gorm:"not null;index"`                         // Foreign key to the User model
	AccountNumber string `gorm:"type:varchar(100);uniqueIndex;not null"` // Full account number (IBAN/etc.)
	AccountType   string `gorm:"type:varchar(50);not null"`              // e.g., personal, savings, investment
	Currency      string `gorm:"type:varchar(10);not null"`              // e.g., USD, GBP
	Balance       int64  `gorm:"not null;default:0"`                     // GORM typically maps int64 to bigint
	IsActive      bool   `gorm:"not null;default:true"`                  // Consider replacing with Status field

	// New fields for UI parity
	Status         string  `gorm:"type:varchar(50);not null;default:'active'"` // 'active', 'blocked_temporary', 'blocked_permanent', 'blocked_stolen'
	CardHolderName string  `gorm:"type:varchar(255);not null"`                 // Should match User's name
	CardType       string  `gorm:"type:varchar(50);default:'debit'"`           // e.g., 'debit', 'credit'
	ExpiryDate     string  `gorm:"type:varchar(5)"`                            // Store as "MM/YY" string
	IBAN           *string `gorm:"type:varchar(34);uniqueIndex"`               // Added: International Bank Account Number (Nullable, Unique)
	BICSwift       *string `gorm:"type:varchar(11)"`                           // Added: Bank Identifier Code (Nullable)
	// Spending Limits (Consider if these should be configurable or derived)
	DailyLimit   int64 `gorm:"default:500000"`  // e.g., 5000.00 * 100
	MonthlyLimit int64 `gorm:"default:5000000"` // e.g., 50000.00 * 100
	// Security Flags
	Enable3DSecure       bool   `gorm:"default:true"`
	EnableContactless    bool   `gorm:"default:true"`
	EnableOnlinePayments bool   `gorm:"default:true"`
	PINHash              string `gorm:"type:varchar(255)"` // Added field for storing hashed PIN

	// Relationships
	Owner User          `gorm:"foreignKey:OwnerUserID"` // Belongs To relationship
	Cards []AccountCard `gorm:"foreignKey:AccountID"`   // Has Many relationship (If using separate Card entities)
}

// BeforeCreate Hook to set default CardHolderName
func (a *Account) BeforeCreate(tx *gorm.DB) (err error) {
	if a.OwnerUserID != 0 && a.CardHolderName == "" {
		var user User
		if err = tx.Select("first_name", "last_name").First(&user, a.OwnerUserID).Error; err == nil {
			a.CardHolderName = user.FirstName + " " + user.LastName
		} else {
			// Handle error: User not found? Log or return specific error
			return fmt.Errorf("failed to find user %d for account creation: %w", a.OwnerUserID, err)
		}
	}
	// Set a default expiry date if not provided (e.g., 3 years from now)
	if a.ExpiryDate == "" {
		expiry := time.Now().AddDate(3, 0, 0) // 3 years from now
		a.ExpiryDate = expiry.Format("01/06") // Format as MM/YY
	}

	// Generate account number if empty (using a simple placeholder for now)
	if a.AccountNumber == "" {
		a.AccountNumber = fmt.Sprintf("LV%d%d", time.Now().UnixNano(), a.OwnerUserID) // Example generation
	}

	return nil
}

// TableName specifies the table name for the Account model.
func (Account) TableName() string {
	return "accounts"
}

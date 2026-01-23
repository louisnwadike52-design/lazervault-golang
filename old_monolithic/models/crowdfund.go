package models

import (
	"fmt"
	"math/rand"
	"time"

	"gorm.io/gorm"
)

// CrowdfundStatus represents the status of a crowdfund campaign
type CrowdfundStatus string

const (
	CrowdfundStatusActive    CrowdfundStatus = "active"
	CrowdfundStatusPaused    CrowdfundStatus = "paused"
	CrowdfundStatusCompleted CrowdfundStatus = "completed"
	CrowdfundStatusCancelled CrowdfundStatus = "cancelled"
)

// CrowdfundVisibility represents the visibility level of a crowdfund
type CrowdfundVisibility string

const (
	CrowdfundVisibilityPublic   CrowdfundVisibility = "public"
	CrowdfundVisibilityPrivate  CrowdfundVisibility = "private"
	CrowdfundVisibilityUnlisted CrowdfundVisibility = "unlisted"
)

// DonationStatus represents the status of a donation
type DonationStatus string

const (
	DonationStatusPending    DonationStatus = "pending"
	DonationStatusProcessing DonationStatus = "processing"
	DonationStatusCompleted  DonationStatus = "completed"
	DonationStatusFailed     DonationStatus = "failed"
	DonationStatusRefunded   DonationStatus = "refunded"
)

// Crowdfund represents a crowdfunding campaign
type Crowdfund struct {
	ID            string              `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CreatorUserID uint                `gorm:"not null;index:idx_crowdfunds_creator" json:"creator_user_id"`
	Title         string              `gorm:"type:varchar(255);not null" json:"title"`
	Description   string              `gorm:"type:text" json:"description"`
	Story         string              `gorm:"type:text" json:"story"`
	CrowdfundCode string              `gorm:"type:varchar(50);uniqueIndex:idx_crowdfunds_code" json:"crowdfund_code"`
	TargetAmount  int64               `gorm:"not null" json:"target_amount"`
	CurrentAmount int64               `gorm:"not null;default:0" json:"current_amount"`
	Currency      string              `gorm:"type:varchar(10);not null;default:'GBP'" json:"currency"`
	Deadline      *time.Time          `gorm:"index" json:"deadline,omitempty"`
	Category      string              `gorm:"type:varchar(50)" json:"category"`
	Status        CrowdfundStatus     `gorm:"type:varchar(50);not null;default:'active';index:idx_crowdfunds_status" json:"status"`
	ImageURL      *string             `gorm:"type:text" json:"image_url,omitempty"`
	Visibility    CrowdfundVisibility `gorm:"type:varchar(20);not null;default:'public'" json:"visibility"`
	Metadata      JSONB               `gorm:"type:jsonb" json:"metadata,omitempty"`
	DonorCount    int                 `gorm:"not null;default:0" json:"donor_count"`
	CreatedAt     time.Time           `gorm:"index:idx_crowdfunds_created" json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	DeletedAt     gorm.DeletedAt      `gorm:"index" json:"-"`

	// Relationships
	Creator   User                `gorm:"foreignKey:CreatorUserID" json:"creator,omitempty"`
	Donations []CrowdfundDonation `gorm:"foreignKey:CrowdfundID" json:"donations,omitempty"`
}

// TableName specifies the table name for Crowdfund
func (Crowdfund) TableName() string {
	return "crowdfunds"
}

// CrowdfundDonation represents a donation to a crowdfund campaign
type CrowdfundDonation struct {
	ID              string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	CrowdfundID     string         `gorm:"type:uuid;not null;index:idx_donations_crowdfund" json:"crowdfund_id"`
	DonorUserID     uint           `gorm:"not null;index:idx_donations_donor" json:"donor_user_id"`
	Amount          int64          `gorm:"not null" json:"amount"`
	Currency        string         `gorm:"type:varchar(10);not null" json:"currency"`
	DonationDate    time.Time      `gorm:"not null;index:idx_donations_date" json:"donation_date"`
	Status          DonationStatus `gorm:"type:varchar(50);not null;default:'pending';index" json:"status"`
	TransactionID   *string        `gorm:"type:varchar(255);uniqueIndex" json:"transaction_id,omitempty"`
	ReceiptID       *string        `gorm:"type:varchar(255)" json:"receipt_id,omitempty"`
	Message         *string        `gorm:"type:text" json:"message,omitempty"`
	IsAnonymous     bool           `gorm:"default:false" json:"is_anonymous"`
	PaymentMethod   string         `gorm:"type:varchar(50)" json:"payment_method"`
	SourceAccountID *uint          `gorm:"index" json:"source_account_id,omitempty"`
	Metadata        JSONB          `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Crowdfund Crowdfund `gorm:"foreignKey:CrowdfundID" json:"crowdfund,omitempty"`
	Donor     User      `gorm:"foreignKey:DonorUserID" json:"donor,omitempty"`
}

// TableName specifies the table name for CrowdfundDonation
func (CrowdfundDonation) TableName() string {
	return "crowdfund_donations"
}

// CrowdfundReceipt represents a receipt for a crowdfund donation
type CrowdfundReceipt struct {
	ID            string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DonationID    string    `gorm:"type:uuid;not null;uniqueIndex" json:"donation_id"`
	CrowdfundID   string    `gorm:"type:uuid;not null;index" json:"crowdfund_id"`
	DonorUserID   uint      `gorm:"not null;index" json:"donor_user_id"`
	Amount        int64     `gorm:"not null" json:"amount"`
	Currency      string    `gorm:"type:varchar(10);not null" json:"currency"`
	DonationDate  time.Time `gorm:"not null" json:"donation_date"`
	GeneratedAt   time.Time `gorm:"not null;index" json:"generated_at"`
	ReceiptNumber string    `gorm:"type:varchar(100);uniqueIndex" json:"receipt_number"`
	ReceiptData   JSONB     `gorm:"type:jsonb" json:"receipt_data,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Relationships
	Donation  CrowdfundDonation `gorm:"foreignKey:DonationID" json:"donation,omitempty"`
	Crowdfund Crowdfund         `gorm:"foreignKey:CrowdfundID" json:"crowdfund,omitempty"`
	Donor     User              `gorm:"foreignKey:DonorUserID" json:"donor,omitempty"`
}

// TableName specifies the table name for CrowdfundReceipt
func (CrowdfundReceipt) TableName() string {
	return "crowdfund_receipts"
}

// GenerateCrowdfundCode generates a unique crowdfund code in the format CF-YYYYMMDD-XXXXX
func GenerateCrowdfundCode() string {
	timestamp := time.Now().Format("20060102")
	randomPart := generateRandomString(5)
	return fmt.Sprintf("CF-%s-%s", timestamp, randomPart)
}

// generateRandomString generates a random alphanumeric string of specified length
func generateRandomString(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// GenerateReceiptNumber generates a unique receipt number in the format RCP-CF-YYYYMMDDHHMMSS-XXXX
func GenerateReceiptNumber() string {
	timestamp := time.Now().Format("20060102150405")
	randomPart := generateRandomString(4)
	return fmt.Sprintf("RCP-CF-%s-%s", timestamp, randomPart)
}

// GenerateTransactionID generates a unique transaction ID for donations
func GenerateTransactionID(prefix string) string {
	timestamp := time.Now().UnixNano()
	randomPart := generateRandomString(6)
	return fmt.Sprintf("%s-%d-%s", prefix, timestamp, randomPart)
}

package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TagPay represents a user's tag pay (username-based money transfer)
type TagPay struct {
	ID          string    `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID      string    `json:"user_id" gorm:"type:uuid;not null;uniqueIndex"`
	TagPay      string    `json:"tag_pay" gorm:"size:50;not null;uniqueIndex"` // e.g., "johndoe"
	DisplayName string    `json:"display_name" gorm:"size:100;not null"`
	AvatarURL   string    `json:"avatar_url" gorm:"size:500"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// BeforeCreate hook to ensure ID is set
func (t *TagPay) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}

// TableName specifies the table name for TagPay
func (t *TagPay) TableName() string {
	return "tag_pays"
}

// TagPayTransactionStatus represents the status of a tag pay transaction
type TagPayTransactionStatus string

const (
	TagPayTransactionStatusPending    TagPayTransactionStatus = "pending"
	TagPayTransactionStatusProcessing TagPayTransactionStatus = "processing"
	TagPayTransactionStatusCompleted  TagPayTransactionStatus = "completed"
	TagPayTransactionStatusFailed     TagPayTransactionStatus = "failed"
	TagPayTransactionStatusCancelled  TagPayTransactionStatus = "cancelled"
	TagPayTransactionStatusRefunded   TagPayTransactionStatus = "refunded"
)

// TagPayTransactionType represents the type of tag pay transaction
type TagPayTransactionType string

const (
	TagPayTransactionTypeSend             TagPayTransactionType = "send"
	TagPayTransactionTypeReceive          TagPayTransactionType = "receive"
	TagPayTransactionTypeRequest          TagPayTransactionType = "request"
	TagPayTransactionTypeRequestFulfilled TagPayTransactionType = "request_fulfilled"
)

// TagPayTransaction represents a transaction using tag pay
type TagPayTransaction struct {
	ID              string                  `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	SenderID        string                  `json:"sender_id" gorm:"type:uuid;not null"`
	SenderTagPay    string                  `json:"sender_tag_pay" gorm:"size:50;not null"`
	SenderName      string                  `json:"sender_name" gorm:"size:200;not null"`
	ReceiverID      string                  `json:"receiver_id" gorm:"type:uuid;not null"`
	ReceiverTagPay  string                  `json:"receiver_tag_pay" gorm:"size:50;not null"`
	ReceiverName    string                  `json:"receiver_name" gorm:"size:200;not null"`
	Amount          float64                 `json:"amount" gorm:"type:decimal(15,2);not null"`
	Currency        string                  `json:"currency" gorm:"size:10;not null;default:'GBP'"`
	Description     string                  `json:"description" gorm:"size:500"`
	Status          TagPayTransactionStatus `json:"status" gorm:"size:50;not null;default:'pending'"`
	Type            TagPayTransactionType   `json:"type" gorm:"size:50;not null"`
	ReferenceNumber string                  `json:"reference_number" gorm:"size:100;uniqueIndex"`
	AccountID       *string                 `json:"account_id" gorm:"type:uuid"` // Source/destination account (nullable)
	CreatedAt       time.Time               `json:"created_at" gorm:"autoCreateTime"`
	CompletedAt     *time.Time              `json:"completed_at"`
}

// BeforeCreate hook
func (t *TagPayTransaction) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	if t.ReferenceNumber == "" {
		t.ReferenceNumber = "TP-" + uuid.New().String()[:8]
	}
	return nil
}

// TableName specifies the table name
func (t *TagPayTransaction) TableName() string {
	return "tag_pay_transactions"
}

// MoneyRequestStatus represents the status of a money request
type MoneyRequestStatus string

const (
	MoneyRequestStatusPending   MoneyRequestStatus = "pending"
	MoneyRequestStatusAccepted  MoneyRequestStatus = "accepted"
	MoneyRequestStatusDeclined  MoneyRequestStatus = "declined"
	MoneyRequestStatusExpired   MoneyRequestStatus = "expired"
	MoneyRequestStatusCancelled MoneyRequestStatus = "cancelled"
)

// MoneyRequest represents a request for money using tag pay
type MoneyRequest struct {
	ID               string             `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	RequesterID      string             `json:"requester_id" gorm:"type:uuid;not null"`
	RequesterTagPay  string             `json:"requester_tag_pay" gorm:"size:50;not null"`
	RequesterName    string             `json:"requester_name" gorm:"size:200;not null"`
	RequesteeID      string             `json:"requestee_id" gorm:"type:uuid;not null"`
	RequesteeTagPay  string             `json:"requestee_tag_pay" gorm:"size:50;not null"`
	RequesteeName    string             `json:"requestee_name" gorm:"size:200;not null"`
	Amount           float64            `json:"amount" gorm:"type:decimal(15,2);not null"`
	Currency         string             `json:"currency" gorm:"size:10;not null;default:'GBP'"`
	Description      string             `json:"description" gorm:"size:500"`
	Status           MoneyRequestStatus `json:"status" gorm:"size:50;not null;default:'pending'"`
	CreatedAt        time.Time          `json:"created_at" gorm:"autoCreateTime"`
	RespondedAt      *time.Time         `json:"responded_at"`
	ExpiresAt        time.Time          `json:"expires_at"` // Requests expire after 7 days
}

// BeforeCreate hook
func (m *MoneyRequest) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	if m.ExpiresAt.IsZero() {
		m.ExpiresAt = time.Now().Add(7 * 24 * time.Hour) // Expires in 7 days
	}
	return nil
}

// TableName specifies the table name
func (m *MoneyRequest) TableName() string {
	return "money_requests"
}

// TagStatus represents the status of a user tag
type TagStatus string

const (
	TagStatusPending   TagStatus = "pending"
	TagStatusPaid      TagStatus = "paid"
	TagStatusCancelled TagStatus = "cancelled"
)

// UserTag represents a tag that one user creates for another
type UserTag struct {
	ID               string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	TaggerID         string     `json:"tagger_id" gorm:"type:uuid;not null"`          // User who created the tag
	TaggerTagPay     string     `json:"tagger_tag_pay" gorm:"size:50;not null"`       // Tagger's username
	TaggerName       string     `json:"tagger_name" gorm:"size:200;not null"`         // Tagger's name
	TaggedUserID     string     `json:"tagged_user_id" gorm:"type:uuid;not null"`     // User who was tagged
	TaggedUserTagPay string     `json:"tagged_user_tag_pay" gorm:"size:50;not null"`  // Tagged user's username
	TaggedUserName   string     `json:"tagged_user_name" gorm:"size:200;not null"`    // Tagged user's name
	Amount           float64    `json:"amount" gorm:"type:decimal(15,2);not null"`    // Amount to pay
	Currency         string     `json:"currency" gorm:"size:10;not null;default:'GBP'"` // Currency
	Description      string     `json:"description" gorm:"size:500"`                  // Optional description
	Status           TagStatus  `json:"status" gorm:"size:50;not null;default:'pending'"` // Tag status
	CreatedAt        time.Time  `json:"created_at" gorm:"autoCreateTime"`
	PaidAt           *time.Time `json:"paid_at"` // When the tag was paid
}

// BeforeCreate hook
func (t *UserTag) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}

// TableName specifies the table name
func (t *UserTag) TableName() string {
	return "user_tags"
}

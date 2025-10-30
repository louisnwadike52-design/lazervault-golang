package models

import (
	"time"

	"gorm.io/datatypes"
)

// PaymentMethod represents a payment method in the system
type PaymentMethod struct {
	MethodID       string            `json:"method_id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID         string            `json:"user_id" gorm:"type:uuid;not null"`
	Type           PaymentMethodType `json:"type" gorm:"type:varchar(50);not null"`
	DisplayName    string            `json:"display_name" gorm:"size:255;not null"`
	Last4          string            `json:"last4" gorm:"size:4"`
	Brand          string            `json:"brand" gorm:"size:50"`
	IsDefault      bool              `json:"is_default" gorm:"default:false"`
	IsVerified     bool              `json:"is_verified" gorm:"default:false"`
	ExpiresAt      time.Time         `json:"expires_at"`
	CreatedAt      time.Time         `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time         `json:"updated_at" gorm:"autoUpdateTime"`
	Metadata       datatypes.JSON    `json:"metadata" gorm:"type:jsonb"`
	BillingAddress string            `json:"billing_address" gorm:"type:text"`
}

// TableName specifies the table name for PaymentMethod
func (pm *PaymentMethod) TableName() string {
	return "payment_methods"
}

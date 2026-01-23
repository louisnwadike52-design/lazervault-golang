package models

import (
	"time"

	"gorm.io/gorm"
)

// ReferralStatus represents the status of a referral transaction
type ReferralStatus string

const (
	ReferralStatusPending   ReferralStatus = "PENDING"
	ReferralStatusCompleted ReferralStatus = "COMPLETED"
	ReferralStatusFailed    ReferralStatus = "FAILED"
	ReferralStatusCancelled ReferralStatus = "CANCELLED"
)

// ReferralCode represents a user's unique referral code
type ReferralCode struct {
	gorm.Model
	UserID   uint   `json:"user_id" gorm:"not null;index"`
	Code     string `json:"code" gorm:"size:50;not null;uniqueIndex:uni_referral_code"`
	IsActive bool   `json:"is_active" gorm:"default:true"`
	User     User   `json:"-" gorm:"foreignKey:UserID"`
}

// ReferralTransaction represents a referral event between two users
type ReferralTransaction struct {
	gorm.Model
	ReferrerUserID       uint           `json:"referrer_user_id" gorm:"not null;index:idx_referrer_user"`
	RefereeUserID        uint           `json:"referee_user_id" gorm:"not null;index:idx_referee_user"`
	ReferralCodeUsed     string         `json:"referral_code_used" gorm:"size:50;not null"`
	Status               ReferralStatus `json:"status" gorm:"size:20;not null;index:idx_status"`
	ReferrerRewardAmount int            `json:"referrer_reward_amount" gorm:"not null"` // in minor units (cents/pence)
	RefereeRewardAmount  int            `json:"referee_reward_amount" gorm:"not null"`  // in minor units (cents/pence)
	Currency             string         `json:"currency" gorm:"size:3;not null"`        // ISO 4217 currency code
	CompletedAt          *time.Time     `json:"completed_at"`
	FailureReason        *string        `json:"failure_reason" gorm:"type:text"`
	Referrer             User           `json:"-" gorm:"foreignKey:ReferrerUserID"`
	Referee              User           `json:"-" gorm:"foreignKey:RefereeUserID"`
}

// CountryRewardConfig represents reward configuration for different countries
type CountryRewardConfig struct {
	gorm.Model
	CountryCode    string     `json:"country_code" gorm:"size:2;not null;uniqueIndex:uni_country_code"` // ISO 3166-1 alpha-2
	Currency       string     `json:"currency" gorm:"size:3;not null"`                                  // ISO 4217 currency code
	ReferrerReward int        `json:"referrer_reward" gorm:"not null"`                                  // in minor units
	RefereeReward  int        `json:"referee_reward" gorm:"not null"`                                   // in minor units
	IsActive       bool       `json:"is_active" gorm:"default:true"`
	EffectiveFrom  time.Time  `json:"effective_from" gorm:"default:CURRENT_TIMESTAMP"`
	EffectiveUntil *time.Time `json:"effective_until"`
}

// TableName overrides the table name for ReferralCode
func (ReferralCode) TableName() string {
	return "referral_codes"
}

// TableName overrides the table name for ReferralTransaction
func (ReferralTransaction) TableName() string {
	return "referral_transactions"
}

// TableName overrides the table name for CountryRewardConfig
func (CountryRewardConfig) TableName() string {
	return "country_reward_configs"
}

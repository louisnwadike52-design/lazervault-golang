package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type UserPreferences struct {
	ID                 string         `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID             string         `json:"user_id" gorm:"type:uuid;not null;uniqueIndex"`
	PushNotifications  bool           `json:"push_notifications" gorm:"default:true"`
	EmailNotifications bool           `json:"email_notifications" gorm:"default:true"`
	SMSNotifications   bool           `json:"sms_notifications" gorm:"default:false"`
	DarkMode           bool           `json:"dark_mode" gorm:"default:false"`
	Language           string         `json:"language" gorm:"size:10;default:'en'"`
	Currency           string         `json:"currency" gorm:"size:10;default:'GBP'"`
	Country            string         `json:"country" gorm:"size:100;default:'United Kingdom'"`
	PreferredCountries pq.StringArray `json:"preferred_countries" gorm:"type:text[]"`
	ActiveCountry      string         `json:"active_country" gorm:"size:10;default:''"`
	CreatedAt          time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
}

func (p *UserPreferences) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

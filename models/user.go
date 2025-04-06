package models

import (
	"lazervaultGo/onboarding"
	"lazervaultGo/utils"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Name        string     `gorm:"size:255;not null"`
	Email       string     `gorm:"size:255;not null;unique;index:idx_email,priority:1"`
	Password    string     `gorm:"size:255;not null"`
	Role        string     `gorm:"size:255;not null"`
	Verified    bool       `gorm:"not null;default:false"`
	VerifiedAt  *time.Time `gorm:""`
	PhoneNumber string     `gorm:"size:255;not null;index:idx_phone_number,priority:1"`
}

func (User) TableName() string {
	return "users"
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	u.Password, err = utils.HashPassword(u.Password)
	if err != nil {
		return err
	}
	return
}

func (u *User) AfterCreate(tx *gorm.DB) (err error) {
	return onboarding.SendWelcomeEmail(u.Email, "Welcome to Lazervault", "Welcome to Lazervault")
}

// func (u *User) BeforeUpdate(tx *gorm.DB) (err error) {
// 	if u.Password != "" {
// 		u.Password, err = utils.HashPassword(u.Password)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	return
// }

func (u *User) ComparePassword(password string) (isSame bool, err error) {
	err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	if err != nil {
		return false, err
	}
	return true, nil
}

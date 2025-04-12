package models

import (
	"errors"
	"lazervaultGo/onboarding"
	"lazervaultGo/utils"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	// GORM specific errors
	ErrDuplicateEmail        = errors.New("email already exists")
	ErrDuplicatePhone        = errors.New("phone number already exists")
	ErrInvalidNameLength     = errors.New("name must be between 2 and 255 characters")
	ErrInvalidEmail          = errors.New("invalid email format")
	ErrInvalidPassword       = errors.New("invalid password")
	ErrInvalidPasswordFormat = errors.New("password must be at least 8 characters long")
	ErrPasswordMismatch      = errors.New("password mismatch")
	ErrInvalidPhone          = errors.New("invalid phone number format")
	ErrInvalidRole           = errors.New("role must be either 'admin' or 'user'")
	ErrFirstNameRequired     = errors.New("first name is required")
	ErrLastNameRequired      = errors.New("last name is required")
	ErrEmailRequired         = errors.New("email is required")
	ErrPasswordRequired      = errors.New("password is required")
	ErrPhoneNumberRequired   = errors.New("phone number is required")
	ErrRoleRequired          = errors.New("role is required")
	ErrInvalidPhoneNumber    = errors.New("invalid phone number")
)

type User struct {
	gorm.Model
	FirstName   string     `json:"first_name" gorm:"size:255;not null;check:length(first_name) >= 2"`
	LastName    string     `json:"last_name" gorm:"size:255;not null;check:length(last_name) >= 2"`
	Email       string     `json:"email" gorm:"size:255;not null;unique;index:idx_email,priority:1"`
	Password    string     `json:"password" gorm:"size:255;not null;check:length(password) >= 8"`
	PhoneNumber string     `json:"phone_number" gorm:"size:255;not null;unique;index:idx_phone_number,priority:1"`
	Role        string     `json:"role" gorm:"size:255;check:role IN ('admin', 'user')"`
	Verified    bool       `json:"verified" gorm:"default:false"`
	VerifiedAt  *time.Time `json:"verified_at"`
	CreatedAt   time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (User) TableName() string {
	return "users"
}

func (u *User) ToJson() gin.H {
	return gin.H{
		"id":           u.ID,
		"first_name":   u.FirstName,
		"last_name":    u.LastName,
		"email":        u.Email,
		"phone_number": u.PhoneNumber,
		"role":         u.Role,
		"verified":     u.Verified,
		"created_at":   u.CreatedAt,
		"updated_at":   u.UpdatedAt,
	}
}

// BeforeCreate hook for GORM
func (u *User) BeforeCreate(tx *gorm.DB) error {
	// Validate name length
	if len(u.FirstName) < 2 || len(u.FirstName) > 255 {
		return ErrInvalidNameLength
	}
	if len(u.LastName) < 2 || len(u.LastName) > 255 {
		return ErrInvalidNameLength
	}

	// Validate email format and uniqueness
	if !utils.IsValidEmail(u.Email) {
		return ErrInvalidEmail
	}
	var count int64
	if err := tx.Model(&User{}).Where("email = ?", u.Email).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrDuplicateEmail
	}

	// Validate password
	if len(u.Password) < 8 {
		return ErrInvalidPassword
	}
	// Hash password
	hashedPassword, err := utils.HashPassword(u.Password)
	if err != nil {
		return err
	}
	u.Password = hashedPassword

	// Validate phone number
	if !utils.IsValidPhoneNumber(u.PhoneNumber) {
		return ErrInvalidPhone
	}
	if err := tx.Model(&User{}).Where("phone_number = ?", u.PhoneNumber).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrDuplicatePhone
	}

	// Validate role
	if u.Role != "" && u.Role != "admin" && u.Role != "user" {
		return ErrInvalidRole
	}
	// Set default role if not provided
	if u.Role == "" {
		u.Role = "user"
	}

	return nil
}

// BeforeUpdate hook for GORM
func (u *User) BeforeUpdate(tx *gorm.DB) error {
	// Only validate fields that are being updated
	if tx.Statement.Changed("FirstName") {
		if len(u.FirstName) < 2 || len(u.FirstName) > 255 {
			return ErrInvalidNameLength
		}
	}
	if tx.Statement.Changed("LastName") {
		if len(u.LastName) < 2 || len(u.LastName) > 255 {
			return ErrInvalidNameLength
		}
	}
	if tx.Statement.Changed("Email") {
		if !utils.IsValidEmail(u.Email) {
			return ErrInvalidEmail
		}
		var count int64
		if err := tx.Model(&User{}).Where("email = ? AND id != ?", u.Email, u.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrDuplicateEmail
		}
	}

	if tx.Statement.Changed("Password") {
		if len(u.Password) < 8 {
			return ErrInvalidPassword
		}
		hashedPassword, err := utils.HashPassword(u.Password)
		if err != nil {
			return err
		}
		u.Password = hashedPassword
	}

	if tx.Statement.Changed("PhoneNumber") {
		if !utils.IsValidPhoneNumber(u.PhoneNumber) {
			return ErrInvalidPhone
		}
		var count int64
		if err := tx.Model(&User{}).Where("phone_number = ? AND id != ?", u.PhoneNumber, u.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrDuplicatePhone
		}
	}

	if tx.Statement.Changed("Role") {
		if u.Role != "admin" && u.Role != "user" {
			return ErrInvalidRole
		}
	}

	return nil
}

// AfterCreate hook for GORM
func (u *User) AfterCreate(tx *gorm.DB) error {
	return onboarding.SendWelcomeEmail(u.Email, "Welcome to Lazervault", "Welcome to Lazervault")
}

// ComparePassword compares the provided password with the hashed password
func (u *User) ComparePassword(password string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	if err != nil {
		return false, ErrPasswordMismatch
	}
	return true, nil
}

func (User) FindById(db *gorm.DB, id uint) (*User, error) {
	var user User
	if err := db.Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (User) GetUserByEmail(db *gorm.DB, email string) (*User, error) {
	var user User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

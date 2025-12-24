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
	gorm.Model        // Includes ID, CreatedAt, UpdatedAt, DeletedAt
	FirstName   string     `json:"first_name" gorm:"size:255"`
	LastName    string     `json:"last_name" gorm:"size:255"`
	Username    *string    `json:"username,omitempty" gorm:"size:255;uniqueIndex:idx_username"`
	Email       string     `json:"email" gorm:"size:255;not null;uniqueIndex:uni_users_email"`
	Password    string     `json:"-" gorm:"size:255"`
	PhoneNumber string     `json:"phone_number" gorm:"size:255;uniqueIndex:uni_users_phone_number"`
	UUID        string     `json:"user_id" gorm:"type:uuid;default:uuid_generate_v4();column:user_id"`
	Role        string     `json:"role" gorm:"size:255;default:'user'"`
	Verified    bool       `json:"verified" gorm:"default:false"`
	VerifiedAt  *time.Time `json:"verified_at"`
	ProfilePicture *string `json:"profile_picture,omitempty" gorm:"type:text"`

	// Partial User Fields (for invitations)
	IsPartial bool  `json:"is_partial" gorm:"default:false"`
	InvitedBy *uint `json:"invited_by,omitempty" gorm:"index:idx_invited_by"`

	// Social Login Fields
	GoogleID *string `json:"google_id,omitempty" gorm:"size:255;uniqueIndex:idx_google_id"`
	AppleID  *string `json:"apple_id,omitempty" gorm:"size:255;uniqueIndex:idx_apple_id"`

	// Password Reset Fields
	ResetPasswordToken          *string    `json:"-" gorm:"type:text;index:idx_users_reset_password_token"`
	ResetPasswordTokenExpiresAt *time.Time `json:"-"`

	// Transaction PIN (Store Hashed)
	TransactionPin *string `json:"-" gorm:"size:255"`

	// Login Passcode (Store Hashed) - FIXED: Now a real DB column
	LoginPasscode *string `json:"-" gorm:"size:255"`

	// Facial Recognition Fields
	FacialRecognitionEnabled bool       `json:"facial_recognition_enabled" gorm:"default:false"`
	FaceRegisteredAt         *time.Time `json:"face_registered_at"`

	// User Preferences (moved to User model for simplicity)
	Language string `json:"language" gorm:"size:10;default:'en'"`
	Currency string `json:"currency" gorm:"size:10;default:'GBP'"`
	Country  string `json:"country" gorm:"size:100;default:'United Kingdom'"`
}

func (User) TableName() string {
	return "users"
}

func (u *User) ToJson() gin.H {
	return gin.H{
		"id":           u.ID,
		"first_name":   u.FirstName,
		"last_name":    u.LastName,
		"name":         u.FirstName + " " + u.LastName,
		"email":        u.Email,
		"phone_number": u.PhoneNumber,
		"user_id":      u.UUID,
		"role":         u.Role,
		"verified":     u.Verified,
		"created_at":   u.CreatedAt,
		"updated_at":   u.UpdatedAt,
	}
}

// BeforeCreate hook for GORM
func (u *User) BeforeCreate(tx *gorm.DB) error {
	// For partial users (invitations), skip most validations
	if u.IsPartial {
		// Only validate email/phone uniqueness and format for partial users
		if u.Email != "" {
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
		}
		if u.PhoneNumber != "" {
			if !utils.IsValidPhoneNumber(u.PhoneNumber) {
				return ErrInvalidPhone
			}
			var count int64
			if err := tx.Model(&User{}).Where("phone_number = ?", u.PhoneNumber).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrDuplicatePhone
			}
		}
		// Set default role for partial users
		if u.Role == "" {
			u.Role = "user"
		}
		return nil
	}

	// Full user validation (existing logic)
	// Validate name length
	if len(u.FirstName) < 2 || len(u.FirstName) > 255 {
		return ErrInvalidNameLength
	}
	if len(u.LastName) < 2 || len(u.LastName) > 255 {
		return ErrInvalidNameLength
	}

	// Validate username if provided
	if u.Username != nil && *u.Username != "" {
		if len(*u.Username) < 3 || len(*u.Username) > 50 {
			return errors.New("username must be between 3 and 50 characters")
		}
		var count int64
		if err := tx.Model(&User{}).Where("username = ?", *u.Username).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("username already exists")
		}
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
	if u.Password != "" { // Check if password is provided
		if len(u.Password) < 8 {
			return ErrInvalidPasswordFormat // Use specific error
		}
		// Hash password
		hashedPassword, err := utils.HashPassword(u.Password)
		if err != nil {
			return err
		}
		u.Password = hashedPassword
	} else if u.GoogleID == nil && u.AppleID == nil {
		// If not a social sign-up, password is required
		return ErrPasswordRequired
	}

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
		if u.Password != "" {
			if len(u.Password) < 8 {
				return ErrInvalidPasswordFormat
			}
			hashedPassword, err := utils.HashPassword(u.Password)
			if err != nil {
				return err
			}
			u.Password = hashedPassword
		}
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
	// Send welcome email (assuming this is desired after user creation)
	return onboarding.SendWelcomeEmail(u.Email, "Welcome to Lazervault", "Welcome to Lazervault")
}

// ComparePassword compares the provided password with the user's hashed password
func (u *User) ComparePassword(password string) (bool, error) {
	if u.Password == "" {
		return false, ErrPasswordMismatch
	}
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	if err != nil {
		return false, ErrPasswordMismatch
	}
	return true, nil
}

// CompareLoginPasscode compares the provided passcode with the user's hashed login passcode
func (u *User) CompareLoginPasscode(passcode string) (bool, error) {
	if u.LoginPasscode == nil || *u.LoginPasscode == "" {
		// No passcode set
		return false, errors.New("no login passcode set")
	}
	err := bcrypt.CompareHashAndPassword([]byte(*u.LoginPasscode), []byte(passcode))
	if err != nil {
		return false, errors.New("passcode mismatch")
	}
	return true, nil
}

// SetLoginPasscode hashes and sets the login passcode for the user
func (u *User) SetLoginPasscode(passcode string) error {
	if len(passcode) < 4 || len(passcode) > 6 {
		return errors.New("passcode must be between 4 and 6 digits")
	}
	hashedPasscode, err := utils.HashPassword(passcode)
	if err != nil {
		return err
	}
	u.LoginPasscode = &hashedPasscode
	return nil
}

func (User) FindById(db *gorm.DB, id uint) (*User, error) {
	var user User
	if err := db.Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by their email address
func (User) GetUserByEmail(db *gorm.DB, email string) (*User, error) {
	var user User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

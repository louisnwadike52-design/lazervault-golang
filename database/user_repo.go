package database

import (
	"errors"
	"lazervaultGo/models"
	"time"

	"gorm.io/gorm"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrResetTokenNotFound = errors.New("password reset token not found or invalid")
	ErrResetTokenExpired  = errors.New("password reset token expired")
)

// FindUserByEmail retrieves a user by email.
func FindUserByEmail(db *gorm.DB, email string) (*models.User, error) {
	var user models.User
	if err := db.Preload("Balance").Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// FindUserByID retrieves a user by ID.
func FindUserByID(db *gorm.DB, id uint) (*models.User, error) {
	var user models.User
	if err := db.Preload("Balance").Where("id = ?", id).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// FindUserByGoogleID retrieves a user by their Google ID.
func FindUserByGoogleID(db *gorm.DB, googleID string) (*models.User, error) {
	var user models.User
	if err := db.Preload("Balance").Where("google_id = ?", googleID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// FindUserByAppleID retrieves a user by their Apple ID.
func FindUserByAppleID(db *gorm.DB, appleID string) (*models.User, error) {
	var user models.User
	if err := db.Preload("Balance").Where("apple_id = ?", appleID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// CreateUser creates a new user record.
func CreateUser(db *gorm.DB, user *models.User) error {
	// The BeforeCreate hook in user.go handles hashing password if present
	return db.Create(user).Error
}

// UpdateUser updates an existing user record.
func UpdateUser(db *gorm.DB, user *models.User) error {
	// The BeforeUpdate hook handles hashing password if changed
	return db.Save(user).Error
}

// UpdateUserPassword updates only the password hash for a user.
func UpdateUserPassword(db *gorm.DB, userID uint, newPasswordHash string) error {
	result := db.Model(&models.User{}).Where("id = ?", userID).Update("password", newPasswordHash)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound // Or specific error
	}
	return nil
}

// --- Password Reset Token Functions ---

// CreatePasswordResetToken creates a new reset token for a user.
func CreatePasswordResetToken(db *gorm.DB, token *models.PasswordResetToken) error {
	// Optionally delete existing tokens for the user first
	db.Where("user_id = ?", token.UserID).Delete(&models.PasswordResetToken{})
	return db.Create(token).Error
}

// FindValidPasswordResetToken finds a token that matches and hasn't expired.
func FindValidPasswordResetToken(db *gorm.DB, tokenString string) (*models.PasswordResetToken, error) {
	var token models.PasswordResetToken
	err := db.Joins("User"). // Eager load User if needed later
					Where("token = ?", tokenString).
					First(&token).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResetTokenNotFound
		}
		return nil, err // Other DB error
	}

	// Check expiry
	if token.ExpiresAt.Before(time.Now()) {
		// Optionally delete expired token
		db.Delete(&token)
		return nil, ErrResetTokenExpired
	}

	return &token, nil
}

// DeletePasswordResetToken deletes a specific reset token.
func DeletePasswordResetToken(db *gorm.DB, tokenID uint) error {
	result := db.Delete(&models.PasswordResetToken{}, tokenID)
	if result.Error != nil {
		return result.Error
	}
	// Optionally check result.RowsAffected
	return nil
}

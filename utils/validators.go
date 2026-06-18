package utils

import (
	"errors"
	"net/mail"
	"regexp"
)

var (
	ErrInvalidPasswordFormat = errors.New("password must be at least 8 characters long and contain uppercase, lowercase, number, and special character")
)

// IsValidEmail checks if the email format is valid according to RFC 5322.
func IsValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func IsValidPhoneNumber(phone string) bool {
	phoneRegex := regexp.MustCompile(`^\+?[1-9]\d{1,14}$`)
	return phoneRegex.MatchString(phone)
}

// IsValidPassword checks password complexity requirements:
// - At least 8 characters long
// - At least one uppercase letter
// - At least one lowercase letter
// - At least one digit
// - At least one special character
// - Maximum 128 characters
func IsValidPassword(password string) bool {
	if len(password) < 8 || len(password) > 128 {
		return false
	}

	// Check for at least one uppercase letter
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	if !hasUpper {
		return false
	}

	// Check for at least one lowercase letter
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	if !hasLower {
		return false
	}

	// Check for at least one digit
	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !hasDigit {
		return false
	}

	// Check for at least one special character
	hasSpecial := regexp.MustCompile(`[!@#$%^&*(),.?":{}|<>_\-+=\[\]\\\/;` + "`" + `~]`).MatchString(password)
	if !hasSpecial {
		return false
	}

	return true
}

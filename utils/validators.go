package utils

import (
	"errors"
	"net/mail"
	"regexp"
)

var (
	ErrInvalidPasswordFormat = errors.New("password must be at least 8 characters long")
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

func IsValidPassword(password string) bool {
	passwordRegex := regexp.MustCompile(`^[a-zA-Z0-9]{8,}$`)
	return passwordRegex.MatchString(password)
}

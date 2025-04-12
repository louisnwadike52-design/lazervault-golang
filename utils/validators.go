package utils

import (
	"errors"
	"regexp"
)

var (
	ErrInvalidPasswordFormat = errors.New("password must be at least 8 characters long")
)

func IsValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)
	return emailRegex.MatchString(email)
}

func IsValidPhoneNumber(phone string) bool {
	phoneRegex := regexp.MustCompile(`^\+?[1-9]\d{1,14}$`)
	return phoneRegex.MatchString(phone)
}

func IsValidPassword(password string) bool {
	passwordRegex := regexp.MustCompile(`^[a-zA-Z0-9]{8,}$`)
	return passwordRegex.MatchString(password)
}

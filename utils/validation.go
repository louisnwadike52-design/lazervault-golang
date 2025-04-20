package utils

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// IsValidLuhn checks if a number string is valid according to the Luhn algorithm.
func IsValidLuhn(number string) bool {
	var sum int
	number = strings.ReplaceAll(number, " ", "") // Remove spaces
	length := len(number)
	parity := length % 2

	for i, digitRune := range number {
		digit, err := strconv.Atoi(string(digitRune))
		if err != nil {
			return false // Not a digit
		}

		if i%2 == parity {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
	}

	return sum%10 == 0
}

// IsValidExpiryFormat checks if the expiry date string is in MM/YY format.
var expiryRegex = regexp.MustCompile(`^(0[1-9]|1[0-2])\/?([0-9]{2})$`)

func IsValidExpiryFormat(expiry string) bool {
	return expiryRegex.MatchString(expiry)
}

// IsExpiryDateInFuture checks if the MM/YY expiry date is in the future.
// Assumes format is already validated by IsValidExpiryFormat.
func IsExpiryDateInFuture(expiry string) bool {
	matches := expiryRegex.FindStringSubmatch(expiry)
	if len(matches) != 3 {
		return false // Should not happen if format is pre-validated
	}

	monthStr, yearStr := matches[1], matches[2]
	month, _ := strconv.Atoi(monthStr)
	year, _ := strconv.Atoi("20" + yearStr) // Convert YY to 20YY

	now := time.Now()
	// Expiry is valid until the *end* of the expiry month.
	// Create a date for the first day of the month *after* the expiry month.
	expiryBoundary := time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC)

	return expiryBoundary.After(now)
}

// DetectCardBrand attempts to detect card brand based on prefix.
// This is a simplified example and not exhaustive.
func DetectCardBrand(cardNumber string) string {
	cardNumber = strings.ReplaceAll(cardNumber, " ", "")
	if len(cardNumber) < 1 {
		return "Unknown"
	}

	switch cardNumber[0] {
	case '4':
		return "Visa"
	case '5':
		if len(cardNumber) > 1 {
			switch cardNumber[1] {
			case '1', '2', '3', '4', '5':
				return "Mastercard"
			}
		}
	case '3':
		if len(cardNumber) > 1 {
			switch cardNumber[1] {
			case '4', '7':
				return "American Express"
			}
		}
	case '6':
		// Discover starts with 6011, 622126-622925, 644-649, 65
		if strings.HasPrefix(cardNumber, "6011") || strings.HasPrefix(cardNumber, "65") {
			return "Discover"
		}
		// Add more Discover ranges if needed
	}
	return "Unknown"
}

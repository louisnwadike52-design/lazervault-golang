package services

import "errors"

// Common errors shared across services
var (
	// Authentication and Authorization
	ErrInvalidUserID      = errors.New("invalid user ID")
	ErrUnauthorizedAccess = errors.New("unauthorized access")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
	ErrPermissionDenied   = errors.New("permission denied")

	// Payment and Transaction
	ErrInvalidPaymentMethod = errors.New("invalid payment method")
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrPaymentFailed        = errors.New("payment failed")
	ErrTransactionFailed    = errors.New("transaction failed")

	// Resource Access
	ErrResourceNotFound    = errors.New("resource not found")
	ErrResourceExists      = errors.New("resource already exists")
	ErrResourceUnavailable = errors.New("resource unavailable")

	// Validation
	ErrInvalidInput    = errors.New("invalid input")
	ErrInvalidFormat   = errors.New("invalid format")
	ErrMissingRequired = errors.New("missing required field")

	// System
	ErrInternalServer     = errors.New("internal server error")
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrDatabaseError      = errors.New("database error")

	// Chat Service
	ErrMessageNotFound    = errors.New("message not found")
	ErrReplyToMsgNotFound = errors.New("reply-to message not found")
	ErrInvalidMessageType = errors.New("invalid message type")
	ErrContentRequired    = errors.New("message content is required")

	// Exchange Service
	ErrInvalidCurrency     = errors.New("invalid or unsupported currency code")
	ErrInvalidAmount       = errors.New("invalid transfer amount")
	ErrRateNotFound        = errors.New("exchange rate not available for the currency pair")
	ErrTransferFailed      = errors.New("failed to initiate transfer")
	ErrInvalidReceiver     = errors.New("invalid receiver details")
	ErrTransactionNotFound = errors.New("exchange transaction not found")

	// Insurance Service
	ErrInsuranceNotFound                = errors.New("insurance not found")
	ErrPaymentNotFound                  = errors.New("payment not found")
	ErrClaimNotFound                    = errors.New("claim not found")
	ErrInvalidInsuranceType             = errors.New("invalid insurance type")
	ErrInvalidPaymentStatus             = errors.New("invalid payment status")
	ErrInvalidClaimStatus               = errors.New("invalid claim status")
	ErrInsurancePaymentAlreadyProcessed = errors.New("insurance payment already processed")
	ErrClaimAlreadyProcessed            = errors.New("claim already processed")
	ErrInsuranceInvalidAmount           = errors.New("invalid insurance amount")
	ErrInvalidDateRange                 = errors.New("invalid date range")

	// Invoice Service
	ErrInvoiceNotFound       = errors.New("invoice not found")
	ErrInvalidInvoiceData    = errors.New("invalid invoice data")
	ErrInvoiceItemInvalid    = errors.New("invoice contains invalid item data")
	ErrInvoiceCreationFailed = errors.New("failed to create invoice")
)

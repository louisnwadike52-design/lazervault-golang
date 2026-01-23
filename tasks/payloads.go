package tasks

import (
	"encoding/json"
	// "time" // time is not used directly here for options
	// "github.com/hibiken/asynq" // asynq is not used directly here
)

// --- Task Payloads ---

// PayloadSendVerifyEmail contains the data needed for the email verification task.
type PayloadSendVerifyEmail struct {
	UserID     uint   `json:"user_id"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	SecretCode string `json:"secret_code"`
}

// PayloadProcessTransfer contains the data needed for processing an internal transfer.
type PayloadProcessTransfer struct {
	TransferID string `json:"transfer_id"`
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (p *PayloadProcessTransfer) MarshalBinary() ([]byte, error) {
	return json.Marshal(p)
}

// ProcessTransferPayload contains the data for processing a transfer by ID
type ProcessTransferPayload struct {
	TransferID uint `json:"transfer_id"`
}

// MarshalBinary implements encoding.BinaryMarshaler
func (p *ProcessTransferPayload) MarshalBinary() ([]byte, error) {
	return json.Marshal(p)
}

// ScheduledTransferCheckPayload contains the data for scheduled transfer check
type ScheduledTransferCheckPayload struct {
	CheckedAt string `json:"checked_at"` // ISO 8601 timestamp
}

// PayloadSendPasswordResetOTP contains data for sending password reset OTP via SMS.
type PayloadSendPasswordResetOTP struct {
	PhoneNumber string `json:"phone_number,omitempty"` // If sending via SMS
	Email       string `json:"email,omitempty"`        // If sending via Email
	OTPCode     string `json:"otp_code"`
}

// PayloadSendPasswordResetEmailOTP contains data for sending password reset OTP via Email.
type PayloadSendPasswordResetEmailOTP struct {
	UserID   uint   `json:"user_id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	OTPCode  string `json:"otp_code"`
}

// DepositProcessPayload defines payload for deposit processing.
type DepositProcessPayload struct {
	DepositID string `json:"deposit_id"`
}

func NewDepositProcessTask(depositID string) ([]byte, error) {
	payload := DepositProcessPayload{DepositID: depositID}
	return json.Marshal(payload)
}

// EmailSendDepositReversalPayload defines payload for deposit reversal email.
type EmailSendDepositReversalPayload struct {
	UserEmail     string `json:"user_email"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	FailureReason string `json:"failure_reason"`
}

func NewDepositReversalEmailTask(email string, amount int64, currency string, reason string) ([]byte, error) {
	payload := EmailSendDepositReversalPayload{
		UserEmail:     email,
		Amount:        amount,
		Currency:      currency,
		FailureReason: reason,
	}
	return json.Marshal(payload)
}

// WithdrawalProcessPayload defines payload for withdrawal processing.
type WithdrawalProcessPayload struct {
	WithdrawalID string `json:"withdrawal_id"`
}

func NewWithdrawalProcessTask(withdrawalID string) ([]byte, error) {
	payload := WithdrawalProcessPayload{WithdrawalID: withdrawalID}
	return json.Marshal(payload)
}

// EmailSendWithdrawalConfirmationPayload defines payload for withdrawal confirmation email.
type EmailSendWithdrawalConfirmationPayload struct {
	UserEmail           string `json:"user_email"`
	Amount              int64  `json:"amount"`
	Currency            string `json:"currency"`
	TargetBankName      string `json:"target_bank_name"`
	TargetAccountNumber string `json:"target_account_number"` // Masked number
}

func NewWithdrawalConfirmationEmailTask(email string, amount int64, currency, bankName, accNum string) ([]byte, error) {
	maskedAccNum := accNum
	if len(accNum) > 4 {
		maskedAccNum = "••••" + accNum[len(accNum)-4:]
	}
	payload := EmailSendWithdrawalConfirmationPayload{
		UserEmail:           email,
		Amount:              amount,
		Currency:            currency,
		TargetBankName:      bankName,
		TargetAccountNumber: maskedAccNum,
	}
	return json.Marshal(payload)
}

// EmailSendWithdrawalFailurePayload defines payload for withdrawal failure email.
type EmailSendWithdrawalFailurePayload struct {
	UserEmail           string `json:"user_email"`
	Amount              int64  `json:"amount"`
	Currency            string `json:"currency"`
	TargetBankName      string `json:"target_bank_name"`
	TargetAccountNumber string `json:"target_account_number"`
	FailureReason       string `json:"failure_reason"`
}

func NewWithdrawalFailureEmailTask(email string, amount int64, currency, bankName, accNum, reason string) ([]byte, error) {
	payload := EmailSendWithdrawalFailurePayload{
		UserEmail:           email,
		Amount:              amount,
		Currency:            currency,
		TargetBankName:      bankName,
		TargetAccountNumber: accNum,
		FailureReason:       reason,
	}
	return json.Marshal(payload)
}

// PayloadProcessExternalTransfer defines payload for external transfers.
type PayloadProcessExternalTransfer struct {
	TransferID string `json:"transfer_id"`
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (p *PayloadProcessExternalTransfer) MarshalBinary() ([]byte, error) {
	return json.Marshal(p)
}

// --- Generate Transaction Data File Task ---
type GenerateTxDataFilePayload struct {
	UserID uint `json:"user_id"` // User ID (primary key) for whom to generate the file
}

func NewGenerateTxDataFileTask(userID uint) ([]byte, error) {
	payload := GenerateTxDataFilePayload{UserID: userID}
	return json.Marshal(payload)
}

// EmailSendInvoicePayload defines payload for sending invoice email
type EmailSendInvoicePayload struct {
	UserID         string  `json:"user_id"`
	UserEmail      string  `json:"user_email"`
	UserName       string  `json:"user_name"`
	InvoiceID      string  `json:"invoice_id"`
	InvoiceNumber  string  `json:"invoice_number"`
	RecipientEmail string  `json:"recipient_email"`
	RecipientName  string  `json:"recipient_name"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	DueDate        string  `json:"due_date"` // ISO 8601 format
	Description    string  `json:"description"`
}

func NewSendInvoiceEmailTask(userID, userEmail, userName, invoiceID, invoiceNumber, recipientEmail, recipientName string, amount float64, currency, dueDate, description string) ([]byte, error) {
	payload := EmailSendInvoicePayload{
		UserID:         userID,
		UserEmail:      userEmail,
		UserName:       userName,
		InvoiceID:      invoiceID,
		InvoiceNumber:  invoiceNumber,
		RecipientEmail: recipientEmail,
		RecipientName:  recipientName,
		Amount:         amount,
		Currency:       currency,
		DueDate:        dueDate,
		Description:    description,
	}
	return json.Marshal(payload)
}

// EmailSendPaymentConfirmationPayload defines payload for payment confirmation email
type EmailSendPaymentConfirmationPayload struct {
	UserEmail        string  `json:"user_email"`
	UserName         string  `json:"user_name"`
	InvoiceID        string  `json:"invoice_id"`
	InvoiceNumber    string  `json:"invoice_number"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	TransactionID    string  `json:"transaction_id"`
	ConfirmationCode string  `json:"confirmation_code"`
	ProcessedAt      string  `json:"processed_at"` // ISO 8601 format
	PaymentMethod    string  `json:"payment_method"`
	FeeAmount        float64 `json:"fee_amount"`
}

func NewPaymentConfirmationEmailTask(userEmail, userName, invoiceID, invoiceNumber string, amount float64, currency, transactionID, confirmationCode, processedAt, paymentMethod string, feeAmount float64) ([]byte, error) {
	payload := EmailSendPaymentConfirmationPayload{
		UserEmail:        userEmail,
		UserName:         userName,
		InvoiceID:        invoiceID,
		InvoiceNumber:    invoiceNumber,
		Amount:           amount,
		Currency:         currency,
		TransactionID:    transactionID,
		ConfirmationCode: confirmationCode,
		ProcessedAt:      processedAt,
		PaymentMethod:    paymentMethod,
		FeeAmount:        feeAmount,
	}
	return json.Marshal(payload)
}

// --- Electricity Bill Payment Tasks ---

// BillPaymentProcessPayload defines payload for bill payment processing
type BillPaymentProcessPayload struct {
	PaymentID       string  `json:"payment_id"`
	ProviderCode    string  `json:"provider_code"`
	MeterNumber     string  `json:"meter_number"`
	Amount          float64 `json:"amount"`
	PaymentGateway  string  `json:"payment_gateway"`
	ReferenceNumber string  `json:"reference_number"`
	IsAutoRecharge  bool    `json:"is_auto_recharge"`
	AutoRechargeID  string  `json:"auto_recharge_id,omitempty"`
}

func NewBillPaymentProcessTask(paymentID, providerCode, meterNumber string, amount float64, gateway, reference string) ([]byte, error) {
	payload := BillPaymentProcessPayload{
		PaymentID:       paymentID,
		ProviderCode:    providerCode,
		MeterNumber:     meterNumber,
		Amount:          amount,
		PaymentGateway:  gateway,
		ReferenceNumber: reference,
		IsAutoRecharge:  false,
	}
	return json.Marshal(payload)
}

// AutoRechargeCheckPayload defines payload for auto-recharge check
type AutoRechargeCheckPayload struct {
	CheckTime int64 `json:"check_time"`
}

func NewAutoRechargeCheckTask(checkTime int64) ([]byte, error) {
	payload := AutoRechargeCheckPayload{CheckTime: checkTime}
	return json.Marshal(payload)
}

// ReminderNotificationPayload defines payload for reminder notification
type ReminderNotificationPayload struct {
	ReminderID string `json:"reminder_id"`
	UserID     string `json:"user_id"`
}

func NewReminderNotificationTask(reminderID, userID string) ([]byte, error) {
	payload := ReminderNotificationPayload{
		ReminderID: reminderID,
		UserID:     userID,
	}
	return json.Marshal(payload)
}

// ProviderSyncPayload defines payload for provider sync
type ProviderSyncPayload struct {
	PaymentGateway string `json:"payment_gateway"`
	Country        string `json:"country"`
}

func NewProviderSyncTask(gateway, country string) ([]byte, error) {
	payload := ProviderSyncPayload{
		PaymentGateway: gateway,
		Country:        country,
	}
	return json.Marshal(payload)
}

// Note: Task creation helpers returning *asynq.Task are removed as requested.
// Calling code will now need to use the `New...Task` helpers above to get
// the payload bytes and then construct the *asynq.Task manually, e.g.:
//
// payloadBytes, err := tasks.NewDepositProcessTask(depositID)
// task := asynq.NewTask(tasks.TypeDepositProcessing, payloadBytes, opts...)

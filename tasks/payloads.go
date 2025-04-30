package tasks

import "encoding/json"

// --- Task Payloads ---

// PayloadSendVerifyEmail contains the data needed for the email verification task.
type PayloadSendVerifyEmail struct {
	UserID     uint   `json:"user_id"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	SecretCode string `json:"secret_code"`
}

// PayloadProcessTransfer contains the data needed for processing a transfer.
// Define this properly based on your transfer logic if needed.
type PayloadProcessTransfer struct {
	TransferID string `json:"transfer_id"`
	// Add other necessary fields
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (p *PayloadProcessTransfer) MarshalBinary() ([]byte, error) {
	return json.Marshal(p)
}

// PayloadSendPasswordResetOTP contains data for sending password reset OTP via SMS.
type PayloadSendPasswordResetOTP struct {
	PhoneNumber string `json:"phone_number"`
	OTPCode     string `json:"otp_code"` // Correct field for SMS OTP flow
}

// --- Deposit Process Task ---

type DepositProcessPayload struct {
	DepositID string `json:"deposit_id"`
}

func NewDepositProcessTask(depositID string) ([]byte, error) {
	payload := DepositProcessPayload{DepositID: depositID}
	return json.Marshal(payload)
}

// --- Email Send Deposit Reversal Task ---

type EmailSendDepositReversalPayload struct {
	UserEmail     string `json:"user_email"`
	Amount        int64  `json:"amount"` // Ensure int64
	Currency      string `json:"currency"`
	FailureReason string `json:"failure_reason"`
}

// Ensure signature accepts int64 amount
func NewDepositReversalEmailTask(email string, amount int64, currency string, reason string) ([]byte, error) {
	payload := EmailSendDepositReversalPayload{
		UserEmail:     email,
		Amount:        amount,
		Currency:      currency,
		FailureReason: reason,
	}
	return json.Marshal(payload)
}

// --- Withdrawal Process Task ---

type WithdrawalProcessPayload struct {
	WithdrawalID string `json:"withdrawal_id"`
}

func NewWithdrawalProcessTask(withdrawalID string) ([]byte, error) {
	payload := WithdrawalProcessPayload{WithdrawalID: withdrawalID}
	return json.Marshal(payload)
}

// --- Email Send Withdrawal Confirmation Task ---

type EmailSendWithdrawalConfirmationPayload struct {
	UserEmail           string `json:"user_email"`
	Amount              int64  `json:"amount"` // Minor units
	Currency            string `json:"currency"`
	TargetBankName      string `json:"target_bank_name"`
	TargetAccountNumber string `json:"target_account_number"` // Masked number
}

func NewWithdrawalConfirmationEmailTask(email string, amount int64, currency, bankName, accNum string) ([]byte, error) {
	// Basic masking for account number display
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

// --- Email Send Withdrawal Failure Task ---

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

// PayloadProcessExternalTransfer defines the payload for external transfers
type PayloadProcessExternalTransfer struct {
	TransferID string `json:"transfer_id"`
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (p *PayloadProcessExternalTransfer) MarshalBinary() ([]byte, error) {
	return json.Marshal(p)
}

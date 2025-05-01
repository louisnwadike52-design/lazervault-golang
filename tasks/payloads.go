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

// PayloadSendPasswordResetOTP contains data for sending password reset OTP.
type PayloadSendPasswordResetOTP struct {
	PhoneNumber string `json:"phone_number,omitempty"` // If sending via SMS
	Email       string `json:"email,omitempty"`        // If sending via Email
	OTPCode     string `json:"otp_code"`
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

// Note: Task creation helpers returning *asynq.Task are removed as requested.
// Calling code will now need to use the `New...Task` helpers above to get
// the payload bytes and then construct the *asynq.Task manually, e.g.:
//
// payloadBytes, err := tasks.NewDepositProcessTask(depositID)
// task := asynq.NewTask(tasks.TypeDepositProcessing, payloadBytes, opts...)

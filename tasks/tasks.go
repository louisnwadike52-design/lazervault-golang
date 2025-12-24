package tasks

import (
	"encoding/json"
)

// Task type constants - Define ALL task types here
const (
	// Email Tasks
	TaskSendVerifyEmail = "task:send_verify_email"
	TaskSendEmail       = "task:send_email" // Generic email task type
	// Specific Email Notifications (Can use TaskSendEmail type)
	TypeEmailSendDepositReversal = "email:send:deposit_reversal"
	TypeEmailSendWithdrawalConf  = "email:send:withdrawal_confirm"
	TypeEmailSendWithdrawalFail  = "email:send:withdrawal_failure"
	TypeEmailSendInvoice         = "email:send:invoice"
	TypeEmailSendPaymentReceipt  = "email:send:payment_receipt"
	TypeEmailSendPaymentConfirm  = "email:send:payment_confirm"

	// OTP Tasks
	TaskSendPasswordResetOTP      = "task:send_password_reset_otp"
	TaskSendPasswordResetEmailOTP = "task:send_password_reset_email_otp"

	// Transaction Processing Tasks
	TypeDepositProcessing       = "deposit:process"
	TypeWithdrawalProcessing    = "withdrawal:process"
	TaskProcessTransfer         = "transfer:process"               // Internal Transfer
	TaskProcessExternalTransfer = "task:process_external_transfer" // External Transfer

	// Data Generation Tasks
	TypeGenerateTxDataFile = "txfile:generate"

	// AI Chat History Tasks
	TypeUpdateChatHistoryAndIndex = "ai_chat:update_history_and_index"

	// Transaction File Tasks
	TypeUpdateTxFileAndIndex = "txfile:update_and_index" // New task type

	// Scheduled Transfer Tasks
	TypeScheduledTransferCheck = "scheduled:transfer:check"

	// Auto-Save Tasks
	TypeScheduledAutoSaveCheck = "scheduled:autosave:check"
)

// Queue name constants
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// --- Task Payloads ---

// Payload for updating chat history and triggering AI indexing
type UpdateChatHistoryAndIndexPayload struct {
	UserID   uint   `json:"user_id"`
	Query    string `json:"query"`
	Response string `json:"response"`
}

// Payload for updating transaction file and triggering AI indexing
type UpdateTxFileAndIndexPayload struct {
	UserID uint   `json:"user_id"`
	TxData string `json:"tx_data"` // JSON string of recent transactions
}

// --- Task Constructors ---

// NewUpdateChatHistoryAndIndexTask creates a new task payload
func NewUpdateChatHistoryAndIndexTask(userID uint, query, response string) ([]byte, error) {
	payload := UpdateChatHistoryAndIndexPayload{
		UserID:   userID,
		Query:    query,
		Response: response,
	}
	return json.Marshal(payload)
}

// NewUpdateTxFileAndIndexTask creates a new task payload for transaction file update
func NewUpdateTxFileAndIndexTask(userID uint, txData string) ([]byte, error) {
	payload := UpdateTxFileAndIndexPayload{
		UserID: userID,
		TxData: txData,
	}
	return json.Marshal(payload)
}

// Payload for scheduled auto-save check
type ScheduledAutoSaveCheckPayload struct {
	CheckedAt string `json:"checked_at"` // ISO 8601 timestamp
}

// NewScheduledAutoSaveCheckTask creates a new task payload for scheduled auto-save check
func NewScheduledAutoSaveCheckTask(checkedAt string) ([]byte, error) {
	payload := ScheduledAutoSaveCheckPayload{
		CheckedAt: checkedAt,
	}
	return json.Marshal(payload)
}

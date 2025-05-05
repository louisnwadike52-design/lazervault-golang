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

	// OTP Tasks
	TaskSendPasswordResetOTP = "task:send_password_reset_otp"

	// Transaction Processing Tasks
	TypeDepositProcessing       = "deposit:process"
	TypeWithdrawalProcessing    = "withdrawal:process"
	TaskProcessTransfer         = "transfer:process"               // Internal Transfer
	TaskProcessExternalTransfer = "task:process_external_transfer" // External Transfer

	// Data Generation Tasks
	TypeGenerateTxDataFile = "txfile:generate"

	// AI Chat History Tasks
	TypeUpdateChatHistoryAndIndex = "ai_chat:update_history_and_index"
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

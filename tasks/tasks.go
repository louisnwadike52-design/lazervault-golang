package tasks

const (
	TaskSendVerifyEmail      = "task:send_verify_email"
	TaskProcessTransfer      = "task:process_transfer"
	TaskSendPasswordResetOTP = "task:send_password_reset_otp"
)

// Task type constants
const (
	TypeDepositProcess           = "deposit:process"
	TypeEmailSendDepositReversal = "email:send:deposit_reversal"
	TypeWithdrawalProcess        = "withdrawal:process"
	TypeEmailSendWithdrawalConf  = "email:send:withdrawal_confirm"
	TypeEmailSendWithdrawalFail  = "email:send:withdrawal_failure"
	// Add other task types here
)

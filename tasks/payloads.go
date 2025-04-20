package tasks

// --- Task Payloads ---

// PayloadSendVerifyEmail contains the data needed for the email verification task.
type PayloadSendVerifyEmail struct {
	Email string `json:"email"`
	Code  string `json:"code"` // Ensure this field exists
}

// PayloadProcessTransfer contains the data needed for processing a transfer.
// Define this properly based on your transfer logic if needed.
type PayloadProcessTransfer struct {
	TransferID string `json:"transfer_id"`
	// Add other necessary fields
}

// PayloadSendPasswordResetOTP contains data for sending password reset OTP via SMS.
type PayloadSendPasswordResetOTP struct {
	PhoneNumber string `json:"phone_number"`
	OTPCode     string `json:"otp_code"`
}

// Add other payload structs here

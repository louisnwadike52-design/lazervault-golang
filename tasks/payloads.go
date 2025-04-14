package tasks

// Payload for processing transfers
type PayloadProcessTransfer struct {
	TransferID uint `json:"transfer_id"`
}

// Payload for sending verification emails
type PayloadSendVerifyEmail struct {
	Email string `json:"email"`
}

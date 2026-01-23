package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"lazervaultGo/mail"
	"lazervaultGo/models"
	"lazervaultGo/services"
	"lazervaultGo/tasks"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// HandleTransferProcessTask processes the transfer task.
func HandleTransferProcessTask(ctx context.Context, t *asynq.Task, db *gorm.DB, mailer mail.EmailSender, distributor tasks.TaskDistributor, txService *services.TransactionService, txFileService *services.GenerateTxDataService) error {
	var payload tasks.PayloadProcessTransfer
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal transfer process payload: %w", asynq.SkipRetry)
	}

	transferID := payload.TransferID

	// Get transfer record
	var transfer models.Transfer
	if err := db.WithContext(ctx).Where("id = ?", transferID).First(&transfer).Error; err != nil {
		return fmt.Errorf("failed to get transfer record: %w", err)
	}

	// Create transaction record for outgoing transfer
	outgoingRecord := services.TransactionRecord{
		UserID:              uint64(transfer.FromUserID),
		Timestamp:           transfer.CreatedAt,
		Type:                "TRANSFER_OUT",
		Amount:              float64(transfer.Amount) / 100.0,
		Currency:            "USD", // Default currency
		Status:              string(transfer.Status),
		Description:         transfer.Reference,
		Reference:           transfer.Reference,
		FromAccountID:       &transfer.FromAccountID,
		ToAccountID:         transfer.ToAccountID,
		FailureReason:       &transfer.FailureReason,
		CompletedAt:         transfer.CompletedAt,
		ProcessingAt:        &transfer.UpdatedAt,
		FailedAt:            transfer.FailedAt,
		TransferFee:         Float64Ptr(float64(transfer.Fee) / 100.0),
		TransferTotalAmount: Float64Ptr(float64(transfer.TotalAmount) / 100.0),
		TransferCategory:    &transfer.Category,
		TransferScheduledAt: transfer.ScheduledAt,
		RecipientID:         transfer.RecipientID,
	}

	// Create transaction record in database
	if err := txService.CreateTransaction(ctx, outgoingRecord); err != nil {
		fmt.Printf("CRITICAL ERROR: Failed to create transaction record for outgoing transfer %s: %v\n", transfer.Reference, err)
	}

	// Append the transaction to the file
	if err := txFileService.AppendTransactionToFile(ctx, transfer.FromUserID, outgoingRecord); err != nil {
		fmt.Printf("CRITICAL ERROR: Failed to append outgoing transfer %s to tx file for user %d: %v\n", transfer.Reference, transfer.FromUserID, err)
	}

	// If this is an internal transfer, create a record for the recipient
	if transfer.ToUserID != nil {
		incomingRecord := services.TransactionRecord{
			UserID:              uint64(*transfer.ToUserID),
			Timestamp:           transfer.CreatedAt,
			Type:                "TRANSFER_IN",
			Amount:              float64(transfer.Amount) / 100.0,
			Currency:            "USD", // Default currency
			Status:              string(transfer.Status),
			Description:         "Transfer from " + transfer.Reference,
			Reference:           transfer.Reference,
			FromAccountID:       &transfer.FromAccountID,
			ToAccountID:         transfer.ToAccountID,
			FailureReason:       &transfer.FailureReason,
			CompletedAt:         transfer.CompletedAt,
			ProcessingAt:        &transfer.UpdatedAt,
			FailedAt:            transfer.FailedAt,
			TransferFee:         Float64Ptr(float64(transfer.Fee) / 100.0),
			TransferTotalAmount: Float64Ptr(float64(transfer.TotalAmount) / 100.0),
			TransferCategory:    &transfer.Category,
			TransferScheduledAt: transfer.ScheduledAt,
		}

		// Create transaction record in database for recipient
		if err := txService.CreateTransaction(ctx, incomingRecord); err != nil {
			fmt.Printf("CRITICAL ERROR: Failed to create transaction record for incoming transfer %s: %v\n", transfer.Reference, err)
		}

		// Append the transaction to the recipient's file
		if err := txFileService.AppendTransactionToFile(ctx, *transfer.ToUserID, incomingRecord); err != nil {
			fmt.Printf("CRITICAL ERROR: Failed to append incoming transfer %s to tx file for user %d: %v\n", transfer.Reference, *transfer.ToUserID, err)
		}
	}

	return nil // Task processed
}

// Helper function to convert int64 to *float64
func Float64Ptr(v float64) *float64 {
	return &v
}

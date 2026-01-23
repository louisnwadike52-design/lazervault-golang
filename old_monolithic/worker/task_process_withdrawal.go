package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/mail"
	"lazervaultGo/models"
	"lazervaultGo/services"
	"lazervaultGo/tasks"
	"math/rand"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// HandleWithdrawalProcessTask processes the withdrawal task.
func HandleWithdrawalProcessTask(ctx context.Context, t *asynq.Task, db *gorm.DB, mailer mail.EmailSender, distributor tasks.TaskDistributor, txService *services.TransactionService, txFileService *services.GenerateTxDataService) error {
	var payload tasks.WithdrawalProcessPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal withdrawal process payload: %w", asynq.SkipRetry)
	}

	withdrawalID := payload.WithdrawalID
	failureReason := ""
	processingStartTime := time.Now() // Keep for potential use

	fmt.Printf("Processing withdrawal task for ID: %s\n", withdrawalID)

	// --- Get Withdrawal Record ---
	var withdrawal models.Withdrawal
	if err := db.WithContext(ctx).Where("id = ?", withdrawalID).First(&withdrawal).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fmt.Printf("Withdrawal record %s not found: %v\n", withdrawalID, err)
			return asynq.SkipRetry
		}
		return fmt.Errorf("failed to get withdrawal record %s: %w", withdrawalID, err)
	}

	// Status should be PENDING when picked up by worker
	if withdrawal.Status != models.WithdrawalStatusPending {
		fmt.Printf("Withdrawal %s already processed or in unexpected state: %s\n", withdrawalID, withdrawal.Status)
		return asynq.SkipRetry
	}

	// --- Mark as Processing --- (Optional but good practice)
	withdrawal.Status = models.WithdrawalStatusProcessing
	withdrawal.ProcessingAt = &processingStartTime
	if err := db.WithContext(ctx).Save(&withdrawal).Error; err != nil {
		return fmt.Errorf("failed to mark withdrawal %s as processing: %w", withdrawalID, err)
	}

	// --- Simulate External Payout Call ---
	// TODO: Replace simulation with actual payout gateway integration
	time.Sleep(time.Duration(rand.Intn(5)+2) * time.Second)
	success := rand.Intn(10) >= 1 // 90% success
	var externalTxID *string
	if !success {
		failureReason = "Simulated payout gateway failure: Timeout"
		fmt.Printf("Simulated external payout failed for withdrawal %s: %s\n", withdrawalID, failureReason)
	} else {
		tempID := fmt.Sprintf("payout_%s", withdrawalID)
		externalTxID = &tempID
		fmt.Printf("Simulated external payout succeeded for withdrawal %s (Ext ID: %s)\n", withdrawalID, *externalTxID)
	}

	// --- Handle Outcome ---
	now := time.Now()
	var finalUserID uint = withdrawal.UserID // Store UserID for task enqueuing

	if success {
		// --- Success Case: Mark Withdrawal Completed ---
		// Funds were already deducted during initiation.
		withdrawal.Status = models.WithdrawalStatusCompleted
		withdrawal.CompletedAt = &now
		withdrawal.ExternalTransactionID = externalTxID
		if err := db.WithContext(ctx).Save(&withdrawal).Error; err != nil {
			// Log error, but consider allowing retry
			fmt.Printf("ERROR marking withdrawal %s completed: %v\n", withdrawalID, err)
			return fmt.Errorf("failed to mark withdrawal %s as completed: %w", withdrawalID, err)
		}

		fmt.Printf("Withdrawal %s completed successfully.\n", withdrawalID)

		// Enqueue confirmation email
		var user models.User
		if err := db.WithContext(ctx).Select("email").First(&user, withdrawal.UserID).Error; err == nil {
			emailPayload, err := tasks.NewWithdrawalConfirmationEmailTask(user.Email, withdrawal.Amount, withdrawal.Currency, withdrawal.TargetBankName, withdrawal.TargetAccountNumber)
			if err != nil {
				fmt.Printf("Error creating withdrawal confirmation email payload for user %d, withdrawal %s: %v\n", withdrawal.UserID, withdrawalID, err)
			} else {
				opts := []asynq.Option{asynq.MaxRetry(3)}
				if err := distributor.DistributeTask(ctx, tasks.TypeEmailSendWithdrawalConf, emailPayload, opts...); err != nil {
					fmt.Printf("Error distributing withdrawal confirmation email task for user %d, withdrawal %s: %v\n", withdrawal.UserID, withdrawalID, err)
				}
			}
		} else {
			fmt.Printf("Error retrieving user email for withdrawal confirmation (User ID: %d, Withdrawal ID: %s): %v\n", withdrawal.UserID, withdrawalID, err)
		}

	} else {
		// --- Failure Case: Revert Balance and Mark Withdrawal Failed ---
		fmt.Printf("Processing failure for withdrawal %s... Attempting to revert balance.\n", withdrawalID)
		txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// 1. Get Source Account (Lock for update)
			var sourceAccount models.Account
			if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", withdrawal.SourceAccountID).First(&sourceAccount).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					fmt.Printf("WARN: Source account %d not found during withdrawal %s failure processing. Cannot revert balance.\n", withdrawal.SourceAccountID, withdrawalID)
				} else {
					return fmt.Errorf("db error finding source account %d for withdrawal %s failure: %w", withdrawal.SourceAccountID, withdrawalID, err)
				}
			} else {
				// 2. Revert Balance (Add back the deducted amount)
				totalDeducted := withdrawal.Amount // Use the amount from the withdrawal record
				sourceAccount.Balance += totalDeducted
				if err := tx.Save(&sourceAccount).Error; err != nil {
					return fmt.Errorf("failed to revert account balance for failed withdrawal %s: %w", withdrawalID, err)
				}
				fmt.Printf("Balance reverted for account %d due to failed withdrawal %s.\n", sourceAccount.ID, withdrawalID)
			}

			// 3. Update Withdrawal Status to FAILED
			withdrawal.Status = models.WithdrawalStatusFailed
			withdrawal.FailedAt = &now
			withdrawal.FailureReason = failureReason // Assign string directly
			if err := tx.Save(&withdrawal).Error; err != nil {
				return fmt.Errorf("failed to mark withdrawal %s as failed: %w", withdrawalID, err)
			}

			return nil // Commit transaction
		})

		if txErr != nil {
			fmt.Printf("Withdrawal failure processing transaction failed for %s: %v\n", withdrawalID, txErr)
			return fmt.Errorf("withdrawal failure processing transaction failed for %s: %w", withdrawalID, txErr) // Retry
		}

		fmt.Printf("Withdrawal %s failed. Balance reverted (if possible). Status updated.\n", withdrawalID)

		// Enqueue failure notification email task
		var user models.User
		if err := db.WithContext(ctx).Select("email").First(&user, withdrawal.UserID).Error; err == nil {
			emailPayload, err := tasks.NewWithdrawalFailureEmailTask(user.Email, withdrawal.Amount, withdrawal.Currency, withdrawal.TargetBankName, withdrawal.TargetAccountNumber, failureReason)
			if err != nil {
				fmt.Printf("Error creating withdrawal failure email payload for user %d, withdrawal %s: %v\n", withdrawal.UserID, withdrawalID, err)
			} else {
				opts := []asynq.Option{asynq.MaxRetry(3)}
				if err := distributor.DistributeTask(ctx, tasks.TypeEmailSendWithdrawalFail, emailPayload, opts...); err != nil {
					fmt.Printf("Error distributing withdrawal failure email task for user %d, withdrawal %s: %v\n", withdrawal.UserID, withdrawalID, err)
				}
			}
		} else {
			fmt.Printf("Error retrieving user email for withdrawal failure notification (User ID: %d, Withdrawal ID: %s): %v\n", withdrawal.UserID, withdrawalID, err)
		}
	}

	// --- Enqueue Tx File Update Task (AFTER success or failure processing) ---
	// Need UserID which should be available on the withdrawal model
	// finalUserID is already declared above, so we'll use it directly

	// Create a transaction record for the withdrawal
	record := services.TransactionRecord{
		UserID:                        uint64(finalUserID),
		Timestamp:                     withdrawal.CreatedAt,
		Type:                          "WITHDRAWAL",
		Amount:                        float64(withdrawal.Amount) / 100.0,
		Currency:                      withdrawal.Currency,
		Status:                        string(withdrawal.Status),
		Description:                   "Withdrawal to " + withdrawal.TargetBankName,
		Reference:                     withdrawal.ID,
		FromAccountID:                 &withdrawal.SourceAccountID,
		FailureReason:                 &withdrawal.FailureReason,
		CompletedAt:                   withdrawal.CompletedAt,
		ProcessingAt:                  withdrawal.ProcessingAt,
		FailedAt:                      withdrawal.FailedAt,
		ExternalTransactionID:         withdrawal.ExternalTransactionID,
		WithdrawalTargetBankName:      &withdrawal.TargetBankName,
		WithdrawalTargetAccountNumber: &withdrawal.TargetAccountNumber,
		WithdrawalTargetSortCode:      &withdrawal.TargetSortCode,
	}

	// Create transaction record in database
	if err := txService.CreateTransaction(ctx, record); err != nil {
		fmt.Printf("CRITICAL ERROR: Failed to create transaction record for withdrawal %s: %v\n", withdrawal.ID, err)
	}

	// Append the transaction to the file
	if err := txFileService.AppendTransactionToFile(ctx, finalUserID, record); err != nil {
		// Log error but don't fail the withdrawal task itself
		fmt.Printf("CRITICAL ERROR: Failed to append withdrawal %s to tx file for user %d: %v\n", withdrawal.ID, finalUserID, err)
	}

	return nil // Task processed
}

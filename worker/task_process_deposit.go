package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/mail"
	"lazervaultGo/models"
	"lazervaultGo/tasks"
	"math/rand"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// HandleDepositProcessTask processes the deposit task.
func HandleDepositProcessTask(ctx context.Context, t *asynq.Task, db *gorm.DB, mailer mail.EmailSender, distributor tasks.TaskDistributor) error {
	var payload tasks.DepositProcessPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal deposit process payload: %w", asynq.SkipRetry)
	}

	depositID := payload.DepositID
	failureReason := ""
	processingStartTime := time.Now()

	fmt.Printf("Processing deposit task for ID: %s\n", depositID)

	// --- Get Deposit Record ---
	var deposit models.Deposit
	if err := db.WithContext(ctx).Where("id = ?", depositID).First(&deposit).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fmt.Printf("Deposit record %s not found (maybe already failed/processed?): %v\n", depositID, err)
			return asynq.SkipRetry // Don't retry if record is gone
		}
		return fmt.Errorf("failed to get deposit record %s: %w", depositID, err) // Retry on other DB errors
	}

	// Check if already processed or in a non-pending state
	if deposit.Status != models.DepositStatusPending {
		fmt.Printf("Deposit %s already processed or in unexpected state: %s\n", depositID, deposit.Status)
		return asynq.SkipRetry
	}

	// --- Mark as Processing ---
	deposit.Status = models.DepositStatusProcessing
	deposit.ProcessingAt = &processingStartTime
	if err := db.WithContext(ctx).Save(&deposit).Error; err != nil {
		return fmt.Errorf("failed to mark deposit %s as processing: %w", depositID, err) // Retry
	}

	// --- Simulate External Payment Call ---
	// TODO: Replace simulation with actual payment gateway integration
	time.Sleep(time.Duration(rand.Intn(5)+1) * time.Second) // Simulate 1-5 seconds processing time
	success := rand.Intn(10) >= 2                           // Simulate 80% success rate
	var externalTxID *string                                // Optional: Store external ID if successful
	if !success {
		failureReason = "Simulated payment gateway failure: Insufficient funds"
		fmt.Printf("Simulated external call failed for deposit %s: %s\n", depositID, failureReason)
	} else {
		tempID := fmt.Sprintf("ext_%s", depositID) // Example external ID
		externalTxID = &tempID
		fmt.Printf("Simulated external call succeeded for deposit %s (Ext ID: %s)\n", depositID, *externalTxID)
	}

	// --- Handle Outcome ---
	now := time.Now()

	if success {
		// --- Success Case: Update Balance and Mark Completed ---
		txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// 1. Get Account (use a simplified getter, assumes account exists)
			var account models.Account
			if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", deposit.TargetAccountID).First(&account).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("target account %d not found during deposit completion", deposit.TargetAccountID)
				}
				return fmt.Errorf("db error finding target account %d: %w", deposit.TargetAccountID, err)
			}

			// Basic validation (redundant checks from service but good practice)
			if account.OwnerUserID != deposit.UserID {
				return fmt.Errorf("account %d owner mismatch during deposit completion", deposit.TargetAccountID)
			}
			if account.Currency != deposit.Currency {
				return fmt.Errorf("account %d currency mismatch (%s vs %s) during deposit completion", deposit.TargetAccountID, account.Currency, deposit.Currency)
			}
			if account.Status != models.AccountStatusActive {
				return fmt.Errorf("account %d is not active during deposit completion (status: %s)", deposit.TargetAccountID, account.Status)
			}

			// 2. Update Balance (using int64)
			account.Balance += deposit.Amount // Direct addition of int64 amounts
			if err := tx.Save(&account).Error; err != nil {
				return fmt.Errorf("failed to update account balance for deposit %s: %w", depositID, err)
			}

			// 3. Update Deposit Status
			deposit.Status = models.DepositStatusCompleted
			deposit.CompletedAt = &now
			deposit.ExternalTransactionID = externalTxID // Save external ID if successful
			if err := tx.Save(&deposit).Error; err != nil {
				return fmt.Errorf("failed to mark deposit %s as completed: %w", depositID, err)
			}

			return nil // Commit transaction
		})

		if txErr != nil {
			fmt.Printf("Deposit completion transaction failed for %s: %v\n", depositID, txErr)
			// Returning error to let Asynq retry, but monitor closely.
			return fmt.Errorf("deposit completion transaction failed for %s: %w", depositID, txErr)
		}

		fmt.Printf("Deposit %s completed successfully.\n", depositID)
		// Optional: Enqueue success notification task

	} else {
		// --- Failure Case: Update Deposit Status to FAILED and Enqueue Reversal Email ---
		fmt.Printf("Processing failure for deposit %s...\n", depositID)

		// Update the original deposit record directly
		deposit.Status = models.DepositStatusFailed
		deposit.FailedAt = &now
		deposit.FailureReason = &failureReason // Assign the reason (pointer type in model)

		if err := db.WithContext(ctx).Save(&deposit).Error; err != nil {
			// Log error, but still try to send email. Worker might retry DB update.
			fmt.Printf("Error marking deposit %s as failed: %v\n", depositID, err)
			// Return error to potentially retry the DB update
			return fmt.Errorf("failed to mark deposit %s as failed: %w", depositID, err)
		}

		// Enqueue Reversal Email
		var user models.User
		if err := db.WithContext(ctx).Select("email").First(&user, deposit.UserID).Error; err == nil {
			emailPayload, err := tasks.NewDepositReversalEmailTask(user.Email, deposit.Amount, deposit.Currency, failureReason)
			if err != nil {
				fmt.Printf("Error creating deposit reversal email payload for user %d, deposit %s: %v\n", deposit.UserID, depositID, err)
			} else {
				opts := []asynq.Option{asynq.MaxRetry(3)}
				if err := distributor.DistributeTask(ctx, tasks.TypeEmailSendDepositReversal, emailPayload, opts...); err != nil {
					fmt.Printf("Error distributing deposit reversal email task for user %d, deposit %s: %v\n", deposit.UserID, depositID, err)
				}
			}
		} else {
			fmt.Printf("Error retrieving user email for deposit reversal notification (User ID: %d, Deposit ID: %s): %v\n", deposit.UserID, depositID, err)
		}

		fmt.Printf("Deposit %s failed. Status updated. Reversal email task enqueued (if possible).\n", depositID)
	}

	return nil // Task processed
}

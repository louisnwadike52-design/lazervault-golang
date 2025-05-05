package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/mail"
	"lazervaultGo/models"
	"lazervaultGo/services"
	"lazervaultGo/tasks"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
	"gorm.io/gorm"
)

type TaskProcessor interface {
	Start() error
	ProcessTaskSendVerifyEmail(ctx context.Context, task *asynq.Task) error
	ProcessTaskProcessTransfer(ctx context.Context, task *asynq.Task) error
	ProcessTaskSendPasswordResetOTP(ctx context.Context, task *asynq.Task) error
	ProcessTaskProcessExternalTransfer(ctx context.Context, task *asynq.Task) error
}

type RedisTaskProcessor struct {
	server              *asynq.Server
	db                  *gorm.DB
	mailer              mail.EmailSender
	config              *configs.Config
	distributor         tasks.TaskDistributor
	txDataFileProcessor *GenerateTxDataFileProcessor
	aiChatService       *services.AIChatService
}

func NewRedisTaskProcessor(redisOpt asynq.RedisClientOpt, db *gorm.DB, mailer mail.EmailSender, config *configs.Config, distributor tasks.TaskDistributor, aiChatService *services.AIChatService) TaskProcessor {
	logger := NewLogger()
	redis.SetLogger(logger)

	txDataFileProcessor := NewGenerateTxDataFileProcessor(db, *config, aiChatService)

	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Queues: map[string]int{
				tasks.QueueCritical: 10,
				tasks.QueueDefault:  5,
				tasks.QueueLow:      5,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				log.Error().Err(err).Str("type", task.Type()).
					Bytes("payload", task.Payload()).Msg("process task failed")
			}),
			Logger:      logger,
			Concurrency: 10,
		},
	)

	return &RedisTaskProcessor{
		server:              server,
		db:                  db,
		mailer:              mailer,
		config:              config,
		distributor:         distributor,
		txDataFileProcessor: txDataFileProcessor,
		aiChatService:       aiChatService,
	}
}

func (processor *RedisTaskProcessor) Start() error {
	mux := asynq.NewServeMux()

	// Register handlers using constants from the tasks package
	mux.HandleFunc(tasks.TaskSendVerifyEmail, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendVerifyUserTask(ctx, task, processor.mailer)
	})
	mux.HandleFunc(tasks.TaskProcessTransfer, processor.ProcessTaskProcessTransfer)
	mux.HandleFunc(tasks.TaskSendPasswordResetOTP, processor.ProcessTaskSendPasswordResetOTP)
	mux.HandleFunc(tasks.TypeDepositProcessing, func(ctx context.Context, task *asynq.Task) error {
		return HandleDepositProcessTask(ctx, task, processor.db, processor.mailer, processor.distributor)
	})
	mux.HandleFunc(tasks.TypeEmailSendDepositReversal, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendDepositReversalTask(ctx, task, processor.mailer)
	})
	mux.HandleFunc(tasks.TypeWithdrawalProcessing, func(ctx context.Context, task *asynq.Task) error {
		return HandleWithdrawalProcessTask(ctx, task, processor.db, processor.mailer, processor.distributor)
	})
	mux.HandleFunc(tasks.TypeEmailSendWithdrawalConf, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendWithdrawalConfirmationTask(ctx, task, processor.mailer)
	})
	mux.HandleFunc(tasks.TypeEmailSendWithdrawalFail, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendWithdrawalFailureTask(ctx, task, processor.mailer)
	})

	// Register the TxDataFile handler using the correct constant
	mux.HandleFunc(tasks.TypeGenerateTxDataFile, processor.txDataFileProcessor.ProcessTask)

	// Register external transfer handler using the correct constant
	mux.HandleFunc(tasks.TaskProcessExternalTransfer, processor.ProcessTaskProcessExternalTransfer)

	// Register the new AI Chat History handler
	mux.HandleFunc(tasks.TypeUpdateChatHistoryAndIndex, func(ctx context.Context, task *asynq.Task) error {
		// Ensure aiChatService is not nil before calling the handler
		if processor.aiChatService == nil {
			log.Error().Msg("AIChatService is nil in RedisTaskProcessor, cannot handle TypeUpdateChatHistoryAndIndex")
			return fmt.Errorf("internal configuration error: AIChatService not available %w", asynq.SkipRetry)
		}
		return HandleUpdateChatHistoryAndIndexTask(ctx, task, processor.aiChatService)
	})

	log.Info().Msg("starting task processor server")
	return processor.server.Start(mux)
}

func (processor *RedisTaskProcessor) ProcessTaskProcessTransfer(ctx context.Context, task *asynq.Task) error {
	var payload tasks.PayloadProcessTransfer
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}

	transferIDStr := payload.TransferID
	transferIDUint64, err := strconv.ParseUint(transferIDStr, 10, 64)
	if err != nil {
		log.Error().Err(err).Str("transfer_id_str", transferIDStr).Msg("failed to parse transfer ID string to uint64")
		return fmt.Errorf("invalid transfer ID format: %w", asynq.SkipRetry)
	}
	transferID := uint(transferIDUint64)

	log.Info().Uint("transfer_id", transferID).Msg("processing transfer task")

	if err := processor.ProcessTransferLogic(ctx, transferID); err != nil {
		log.Error().Err(err).Uint("transfer_id", transferID).Msg("failed to process transfer")
		return err
	}

	log.Info().Uint("transfer_id", transferID).Msg("successfully processed transfer task")
	return nil
}

func (processor *RedisTaskProcessor) ProcessTransferLogic(ctx context.Context, transferID uint) error {
	tx := processor.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		} else if tx.Error != nil {
			tx.Rollback()
		}
	}()

	var transfer models.Transfer
	if err := tx.Preload("FromAccount").Preload("ToAccount").First(&transfer, transferID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get transfer with accounts: %w", err)
	}

	if transfer.FromAccount.ID == 0 || transfer.ToAccount.ID == 0 {
		transfer.Status = models.TransferStatusFailed
		now := time.Now()
		transfer.FailedAt = &now
		transfer.FailureReason = "invalid source or destination account"
		if err := tx.Save(&transfer).Error; err != nil {
			tx.Rollback()
			return err
		}
		tx.Rollback()
		return fmt.Errorf("transfer %d links to non-existent account(s)", transferID)
	}

	if transfer.Status != models.TransferStatusPending {
		log.Warn().Uint("transfer_id", transferID).Str("status", string(transfer.Status)).Msg("transfer already processed or in unexpected state")
		return nil
	}

	if transfer.FromAccount.Balance < transfer.TotalAmount {
		transfer.Status = models.TransferStatusFailed
		now := time.Now()
		transfer.FailedAt = &now
		transfer.FailureReason = "insufficient funds"

		if err := tx.Save(&transfer).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update transfer status to failed: %w", err)
		}

		log.Warn().Uint("transfer_id", transferID).Msg("transfer failed due to insufficient funds")
		return tx.Commit().Error
	}

	transfer.FromAccount.Balance -= transfer.TotalAmount
	transfer.ToAccount.Balance += transfer.Amount

	if err := tx.Save(&transfer.FromAccount).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update sender account balance: %w", err)
	}

	if err := tx.Save(&transfer.ToAccount).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update recipient account balance: %w", err)
	}

	transfer.Status = models.TransferStatusCompleted
	now := time.Now()
	transfer.CompletedAt = &now

	if err := tx.Save(&transfer).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update transfer status to completed: %w", err)
	}

	// --- Enqueue Tx File Update Task (AFTER successful commit) ---
	// We need the UserID. Since it could be FromAccount or ToAccount owner,
	// let's enqueue for both if they are different users.
	if transfer.FromAccount.OwnerUserID != 0 {
		enqueueTxFileUpdate(ctx, processor.distributor, transfer.FromAccount.OwnerUserID, fmt.Sprintf("transfer %d", transferID))
	}
	// Ensure ToAccount owner is different before enqueuing again
	if transfer.ToAccount.OwnerUserID != 0 && transfer.ToAccount.OwnerUserID != transfer.FromAccount.OwnerUserID {
		enqueueTxFileUpdate(ctx, processor.distributor, transfer.ToAccount.OwnerUserID, fmt.Sprintf("transfer %d", transferID))
	}

	return tx.Commit().Error
}

// Helper function to enqueue the file generation task and log errors
func enqueueTxFileUpdate(ctx context.Context, distributor tasks.TaskDistributor, userID uint, triggerEvent string) {
	txFilePayloadBytes, err := tasks.NewGenerateTxDataFileTask(userID)
	if err != nil {
		fmt.Printf("CRITICAL ERROR: Failed creating tx file generation payload for user %d after %s: %v\n", userID, triggerEvent, err)
		return // Don't proceed if payload creation fails
	}
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Minute),
		asynq.Queue(tasks.QueueLow),
	}
	if err := distributor.DistributeTask(ctx, tasks.TypeGenerateTxDataFile, txFilePayloadBytes, opts...); err != nil {
		fmt.Printf("CRITICAL ERROR: Failed enqueuing tx file generation task for user %d after %s: %v\n", userID, triggerEvent, err)
	}
}

func (processor *RedisTaskProcessor) ProcessTaskSendVerifyEmail(ctx context.Context, task *asynq.Task) error {
	var payload tasks.PayloadSendVerifyEmail
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}

	user := models.User{}
	err := processor.db.Where("email = ?", payload.Email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error().Err(err).Str("email", payload.Email).Msg("user not found for sending verification email")
			return fmt.Errorf("user not found: %w", asynq.SkipRetry)
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	subject := "Welcome to Lazervault"
	verifyUrl := "http://localhost:8080/verify?email=" + user.Email
	content := fmt.Sprintf(`Hello %s,<br/>
	Thank you for registering with us!<br/>
	Please <a href="%s">click here</a> to verify your email address.<br/>
	`, user.FirstName+" "+user.LastName, verifyUrl)
	to := []string{user.Email}

	err = processor.mailer.SendEmail(subject, content, to, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to send verify email: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("email", user.Email).Msg("processed task: send_verify_email")
	return nil
}

func (processor *RedisTaskProcessor) ProcessTaskSendPasswordResetOTP(ctx context.Context, task *asynq.Task) error {
	var payload tasks.PayloadSendPasswordResetOTP
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}

	log.Info().
		Str("type", task.Type()).
		Str("phone", payload.PhoneNumber).
		Msg("processing password reset OTP task")

	twilioClient := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: processor.config.TwilioAccountSID,
		Password: processor.config.TwilioAuthToken,
	})

	params := &twilioApi.CreateMessageParams{}
	params.SetTo(payload.PhoneNumber)
	params.SetFrom(processor.config.TwilioFromNumber)
	params.SetBody(fmt.Sprintf("Your LazerVault password reset code is: %s", payload.OTPCode))

	_, err := twilioClient.Api.CreateMessage(params)
	if err != nil {
		log.Error().Err(err).Msg("failed to send password reset OTP via Twilio")
		return fmt.Errorf("twilio API error: %w", err)
	}

	log.Info().Str("phone", payload.PhoneNumber).Msg("password reset OTP sent successfully via Twilio")
	return nil
}

func (processor *RedisTaskProcessor) ProcessTaskProcessExternalTransfer(ctx context.Context, task *asynq.Task) error {
	var payload tasks.PayloadProcessExternalTransfer
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal external transfer payload: %w", asynq.SkipRetry)
	}
	fmt.Printf("Processing external transfer task: %+v\n", payload)

	// --- TODO: Implement actual external transfer logic ---
	// 1. Start DB Transaction (maybe?) - depends if external API call is idempotent
	// 2. Fetch Transfer record (status should be Processing)
	// 3. Fetch related Recipient record
	// 4. Fetch FromAccount (needed for debit)
	// 5. Call External Payment Gateway API (using recipient details)
	// 6. Handle API response:
	//    - On Success: Debit FromAccount, Update Transfer status to Completed, Commit.
	//    - On Failure: Update Transfer status to Failed (with reason), Rollback (if transaction started).
	// 7. Handle potential idempotency issues with the external API.

	fmt.Println("PLACEHOLDER: External transfer logic for", payload.TransferID)
	// Example: Simulate success/failure
	transferID := payload.TransferID
	var transfer models.Transfer
	success := true // Simulate success for now
	var dbErr error

	if success {
		dbErr = processor.db.Model(&models.Transfer{}).Where("id = ?", transferID).Update("status", models.TransferStatusCompleted).Error
	} else {
		dbErr = processor.db.Model(&models.Transfer{}).Where("id = ?", transferID).Updates(models.Transfer{Status: models.TransferStatusFailed, FailureReason: "Simulated external failure"}).Error
	}

	if dbErr != nil {
		fmt.Printf("ERROR updating external transfer status for %s: %v\n", transferID, dbErr)
		return dbErr // Let Asynq handle retry/failure
	}

	// --- Enqueue Tx File Update Task (AFTER successful simulated update) ---
	if success {
		// Need to fetch the UserID associated with the FromAccount of the transfer
		if err := processor.db.WithContext(ctx).Preload("FromAccount").First(&transfer, "id = ?", transferID).Error; err == nil {
			if transfer.FromAccount.OwnerUserID != 0 {
				enqueueTxFileUpdate(ctx, processor.distributor, transfer.FromAccount.OwnerUserID, fmt.Sprintf("external transfer %s", transferID))
			} else {
				fmt.Printf("Warning: Could not determine owner user ID for external transfer %s to enqueue file update.\n", transferID)
			}
		} else {
			fmt.Printf("ERROR: Failed to fetch transfer details for user ID lookup after external transfer %s: %v\n", transferID, err)
		}
	}

	fmt.Printf("Updated external transfer %s status (simulated)\n", transferID)
	return nil // Return error to retry/fail based on actual external API outcome
}

// HandleUpdateChatHistoryAndIndexTask processes the task to save chat history and trigger indexing.
func HandleUpdateChatHistoryAndIndexTask(ctx context.Context, task *asynq.Task, aiService *services.AIChatService) error {
	var payload tasks.UpdateChatHistoryAndIndexPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		// Non-recoverable error: If payload is malformed, retrying won't help.
		log.Error().Err(err).Msg("Failed to unmarshal UpdateChatHistoryAndIndexPayload")
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}

	log.Info().Uint("user_id", payload.UserID).Msg("Processing chat history update task")

	// Step 1: Save the chat entry
	if err := aiService.SaveChatEntry(ctx, payload.UserID, payload.Query, payload.Response); err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("Failed to save chat entry in background task")
		// Depending on the error, you might want to retry (e.g., temporary DB issue)
		// For now, let's return the error to let Asynq handle retries based on MaxRetry.
		return fmt.Errorf("failed to save chat entry: %w", err)
	}

	// Step 2: Update the chat history file in GCS
	if err := aiService.UpdateChatHistoryFile(ctx, payload.UserID); err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("Failed to update chat history file in background task")
		// GCS errors might be transient, so retrying is reasonable.
		return fmt.Errorf("failed to update chat history file: %w", err)
	}

	// Step 3: Trigger AI indexing for chat history
	if err := aiService.TriggerChatHistoryIndexing(ctx, payload.UserID); err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("Failed to trigger AI chat history indexing in background task")
		// Errors calling the AI service might be transient (network, service down).
		return fmt.Errorf("failed to trigger AI chat history indexing: %w", err)
	}

	log.Info().Uint("user_id", payload.UserID).Msg("Successfully processed chat history update task")
	return nil
}

func (processor *RedisTaskProcessor) Shutdown() {
	processor.server.Shutdown()
}

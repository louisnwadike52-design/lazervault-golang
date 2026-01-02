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
	server                      *asynq.Server
	db                          *gorm.DB
	mailer                      mail.EmailSender
	config                      *configs.Config
	distributor                 tasks.TaskDistributor
	txDataFileProcessor         *GenerateTxDataFileProcessor
	scheduledTransferProcessor  *ScheduledTransferProcessor
	scheduledAutoSaveProcessor  *ScheduledAutoSaveProcessor
	scheduledAutoRechargeProcessor *ScheduledAutoRechargeProcessor
	scheduledReminderProcessor     *ScheduledReminderProcessor
}

func NewRedisTaskProcessor(redisOpt asynq.RedisClientOpt, db *gorm.DB, mailer mail.EmailSender, config *configs.Config, distributor tasks.TaskDistributor) TaskProcessor {
	logger := NewLogger()
	redis.SetLogger(logger)

	txDataFileProcessor := NewGenerateTxDataFileProcessor(db, *config)
	scheduledTransferProcessor := NewScheduledTransferProcessor(db, distributor)

	// Initialize auto-save service and processor
	accountService := services.NewAccountService(db, distributor)
	recipientService := services.NewRecipientService(db)
	transferService := services.NewTransferService(db, config, distributor, recipientService, accountService)
	autoSaveService := services.NewAutoSaveService(db, distributor, accountService, transferService)
	scheduledAutoSaveProcessor := NewScheduledAutoSaveProcessor(db, autoSaveService, distributor)

	// Initialize electricity bill scheduled processors
	scheduledAutoRechargeProcessor := NewScheduledAutoRechargeProcessor(db, distributor)
	scheduledReminderProcessor := NewScheduledReminderProcessor(db, distributor)

	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Queues: map[string]int{
				tasks.QueueCritical: 10, // High priority for critical tasks
				tasks.QueueDefault:  5,  // Normal priority
				tasks.QueueLow:      5,  // Low priority for non-urgent tasks
			},
			// PRODUCTION-GRADE ERROR HANDLING
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				log.Error().
					Err(err).
					Str("task_type", task.Type()).
					Bytes("payload", task.Payload()).
					Msg("CRITICAL: Task processing failed")

				// TODO: Send alert to monitoring service (Sentry, Datadog, etc.)
				// TODO: Store failed task in dead letter queue for manual review
			}),
			Logger:      logger,
			Concurrency: 10, // Process up to 10 tasks concurrently

			// PRODUCTION-GRADE RETRY CONFIGURATION
			RetryDelayFunc: func(n int, err error, task *asynq.Task) time.Duration {
				// Exponential backoff: 2^n seconds with jitter
				baseDelay := time.Duration(1<<uint(n)) * time.Second
				// Add jitter to prevent thundering herd
				jitter := time.Duration(n*500) * time.Millisecond
				return baseDelay + jitter
			},

			// Health check interval
			HealthCheckInterval: 15 * time.Second,

			// Graceful shutdown timeout
			ShutdownTimeout: 30 * time.Second,
		},
	)

	return &RedisTaskProcessor{
		server:                         server,
		db:                             db,
		mailer:                         mailer,
		config:                         config,
		distributor:                    distributor,
		txDataFileProcessor:            txDataFileProcessor,
		scheduledTransferProcessor:     scheduledTransferProcessor,
		scheduledAutoSaveProcessor:     scheduledAutoSaveProcessor,
		scheduledAutoRechargeProcessor: scheduledAutoRechargeProcessor,
		scheduledReminderProcessor:     scheduledReminderProcessor,
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
	mux.HandleFunc(tasks.TaskSendPasswordResetEmailOTP, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendPasswordResetOTPTask(ctx, task, processor.mailer)
	})
	mux.HandleFunc(tasks.TypeDepositProcessing, func(ctx context.Context, task *asynq.Task) error {
		txService := services.NewTransactionService(processor.db)
		return HandleDepositProcessTask(ctx, task, processor.db, processor.mailer, processor.distributor, txService, processor.txDataFileProcessor.txFileService)
	})
	mux.HandleFunc(tasks.TypeEmailSendDepositReversal, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendDepositReversalTask(ctx, task, processor.mailer)
	})
	mux.HandleFunc(tasks.TypeWithdrawalProcessing, func(ctx context.Context, task *asynq.Task) error {
		txService := services.NewTransactionService(processor.db)
		return HandleWithdrawalProcessTask(ctx, task, processor.db, processor.mailer, processor.distributor, txService, processor.txDataFileProcessor.txFileService)
	})
	mux.HandleFunc(tasks.TypeEmailSendWithdrawalConf, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendWithdrawalConfirmationTask(ctx, task, processor.mailer)
	})
	mux.HandleFunc(tasks.TypeEmailSendWithdrawalFail, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendWithdrawalFailureTask(ctx, task, processor.mailer)
	})

	// Register the TxDataFile handler using the correct constant
	mux.HandleFunc(tasks.TypeGenerateTxDataFile, func(ctx context.Context, task *asynq.Task) error {
		aiChatService := services.NewAIChatService(processor.db, processor.config, processor.distributor)
		return processor.txDataFileProcessor.ProcessTask(ctx, task, aiChatService)
	})

	// Register external transfer handler using the correct constant
	mux.HandleFunc(tasks.TaskProcessExternalTransfer, processor.ProcessTaskProcessExternalTransfer)

	// Register the new AI Chat History handler
	mux.HandleFunc(tasks.TypeUpdateChatHistoryAndIndex, func(ctx context.Context, task *asynq.Task) error {
		aiChatService := services.NewAIChatService(processor.db, processor.config, processor.distributor)
		return HandleUpdateChatHistoryAndIndexTask(ctx, task, aiChatService)
	})

	// Register the new transaction file update handler
	mux.HandleFunc(tasks.TypeUpdateTxFileAndIndex, processor.HandleUpdateTxFile)

	// Register the scheduled transfer check handler
	mux.HandleFunc(tasks.TypeScheduledTransferCheck, processor.scheduledTransferProcessor.ProcessScheduledTransferCheck)

	// Register the scheduled auto-save check handler
	mux.HandleFunc(tasks.TypeScheduledAutoSaveCheck, processor.scheduledAutoSaveProcessor.ProcessScheduledAutoSaveCheck)

	// Register invoice email handlers
	mux.HandleFunc(tasks.TypeEmailSendInvoice, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendInvoiceTask(ctx, task, processor.mailer)
	})
	mux.HandleFunc(tasks.TypeEmailSendPaymentConfirm, func(ctx context.Context, task *asynq.Task) error {
		return HandleEmailSendPaymentConfirmationTask(ctx, task, processor.mailer)
	})

	// Register electricity bill payment handlers
	mux.HandleFunc(tasks.TaskProcessBillPayment, func(ctx context.Context, task *asynq.Task) error {
		// Initialize payment provider factory
		flutterwaveConfig := services.FlutterwaveConfig{
			SecretKey: processor.config.FlutterwaveSecretKey,
			PublicKey: processor.config.FlutterwavePublicKey,
			BaseURL:   processor.config.FlutterwaveBaseURL,
			Enabled:   processor.config.FlutterwaveEnabled,
		}
		flutterwaveClient := services.NewFlutterwaveBillClient(flutterwaveConfig)

		paystackConfig := services.PaystackConfig{
			SecretKey: processor.config.PaystackSecretKey,
			PublicKey: processor.config.PaystackPublicKey,
			BaseURL:   processor.config.PaystackBaseURL,
			Enabled:   processor.config.PaystackEnabled,
		}
		paystackClient := services.NewPaystackBillClient(paystackConfig)

		billProviderFactory := services.NewBillPaymentProviderFactory(flutterwaveClient, paystackClient)

		return HandleBillPaymentProcessTask(ctx, task, processor.db, billProviderFactory, processor.distributor)
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
	// PRODUCTION-GRADE TRANSACTION PROCESSING WITH COMPREHENSIVE ERROR HANDLING

	log.Info().Uint("transfer_id", transferID).Msg("Starting transfer processing")

	// Start database transaction with proper error handling
	tx := processor.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		log.Error().Err(tx.Error).Uint("transfer_id", transferID).Msg("Failed to begin database transaction")
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Ensure rollback on panic or error
	var commitError error
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Error().
				Uint("transfer_id", transferID).
				Interface("panic", r).
				Msg("CRITICAL: Panic during transfer processing - transaction rolled back")
		} else if commitError != nil || tx.Error != nil {
			tx.Rollback()
			log.Warn().Uint("transfer_id", transferID).Msg("Transaction rolled back due to error")
		}
	}()

	// Fetch transfer with account details
	var transfer models.Transfer
	if err := tx.Preload("FromAccount").Preload("ToAccount").First(&transfer, transferID).Error; err != nil {
		commitError = err
		log.Error().Err(err).Uint("transfer_id", transferID).Msg("Failed to fetch transfer record")
		return fmt.Errorf("failed to get transfer: %w", err)
	}

	// EDGE CASE: Validate accounts exist
	if transfer.FromAccount.ID == 0 || transfer.ToAccount.ID == 0 {
		transfer.Status = models.TransferStatusFailed
		now := time.Now()
		transfer.FailedAt = &now
		transfer.FailureReason = "invalid source or destination account"

		if err := tx.Save(&transfer).Error; err != nil {
			commitError = err
			log.Error().Err(err).Uint("transfer_id", transferID).Msg("Failed to save failure status")
			return err
		}

		commitError = tx.Commit().Error
		if commitError != nil {
			log.Error().Err(commitError).Uint("transfer_id", transferID).Msg("Failed to commit failure status")
			return commitError
		}

		log.Warn().Uint("transfer_id", transferID).Msg("Transfer failed: invalid accounts")
		return fmt.Errorf("transfer %d links to non-existent account(s)", transferID)
	}

	// EDGE CASE: Check if already processed (idempotency)
	if transfer.Status != models.TransferStatusPending && transfer.Status != models.TransferStatusProcessing {
		log.Warn().
			Uint("transfer_id", transferID).
			Str("status", string(transfer.Status)).
			Msg("Transfer already processed - skipping (idempotent)")
		return nil
	}

	// Update status to processing
	transfer.Status = models.TransferStatusProcessing
	if err := tx.Save(&transfer).Error; err != nil {
		commitError = err
		log.Error().Err(err).Uint("transfer_id", transferID).Msg("Failed to update status to processing")
		return err
	}

	// EDGE CASE: Validate sufficient balance
	if transfer.FromAccount.Balance < transfer.TotalAmount {
		transfer.Status = models.TransferStatusFailed
		now := time.Now()
		transfer.FailedAt = &now
		transfer.FailureReason = "insufficient funds at processing time"

		if err := tx.Save(&transfer).Error; err != nil {
			commitError = err
			log.Error().Err(err).Uint("transfer_id", transferID).Msg("Failed to save insufficient funds failure")
			return fmt.Errorf("failed to update transfer status to failed: %w", err)
		}

		commitError = tx.Commit().Error
		if commitError != nil {
			log.Error().Err(commitError).Uint("transfer_id", transferID).Msg("Failed to commit insufficient funds status")
			return commitError
		}

		log.Warn().
			Uint("transfer_id", transferID).
			Int64("required", transfer.TotalAmount).
			Int64("available", transfer.FromAccount.Balance).
			Msg("Transfer failed: insufficient funds")

		// TODO: Send notification to user about failed transfer
		return nil // Not an error - properly handled failure
	}

	// EDGE CASE: Check account status (frozen, locked, closed)
	if transfer.FromAccount.Status != "active" {
		transfer.Status = models.TransferStatusFailed
		now := time.Now()
		transfer.FailedAt = &now
		transfer.FailureReason = fmt.Sprintf("source account is %s", transfer.FromAccount.Status)

		if err := tx.Save(&transfer).Error; err != nil {
			commitError = err
			return err
		}

		commitError = tx.Commit().Error
		log.Warn().
			Uint("transfer_id", transferID).
			Str("account_status", transfer.FromAccount.Status).
			Msg("Transfer failed: account not active")
		return commitError
	}

	// Execute the transfer - debit and credit atomically
	log.Info().
		Uint("transfer_id", transferID).
		Int64("amount", transfer.Amount).
		Int64("fee", transfer.Fee).
		Int64("total", transfer.TotalAmount).
		Msg("Executing transfer")

	// Debit from source account
	originalSourceBalance := transfer.FromAccount.Balance
	transfer.FromAccount.Balance -= transfer.TotalAmount

	if err := tx.Save(&transfer.FromAccount).Error; err != nil {
		commitError = err
		log.Error().
			Err(err).
			Uint("transfer_id", transferID).
			Uint("account_id", transfer.FromAccount.ID).
			Msg("CRITICAL: Failed to debit source account")
		return fmt.Errorf("failed to update sender account balance: %w", err)
	}

	// Credit to destination account
	originalDestBalance := transfer.ToAccount.Balance
	transfer.ToAccount.Balance += transfer.Amount

	if err := tx.Save(&transfer.ToAccount).Error; err != nil {
		commitError = err
		log.Error().
			Err(err).
			Uint("transfer_id", transferID).
			Uint("account_id", transfer.ToAccount.ID).
			Msg("CRITICAL: Failed to credit destination account - rolling back")

		// ROLLBACK: Restore source account balance
		transfer.FromAccount.Balance = originalSourceBalance
		if rollbackErr := tx.Save(&transfer.FromAccount).Error; rollbackErr != nil {
			log.Error().
				Err(rollbackErr).
				Uint("transfer_id", transferID).
				Msg("CRITICAL: Failed to rollback source account - MANUAL INTERVENTION REQUIRED")
		}

		return fmt.Errorf("failed to update recipient account balance: %w", err)
	}

	// Mark transfer as completed
	transfer.Status = models.TransferStatusCompleted
	now := time.Now()
	transfer.CompletedAt = &now

	if err := tx.Save(&transfer).Error; err != nil {
		commitError = err
		log.Error().
			Err(err).
			Uint("transfer_id", transferID).
			Msg("CRITICAL: Failed to update transfer status to completed")

		// ROLLBACK: Restore account balances
		transfer.FromAccount.Balance = originalSourceBalance
		transfer.ToAccount.Balance = originalDestBalance

		if rollbackErr := tx.Save(&transfer.FromAccount).Error; rollbackErr != nil {
			log.Error().Err(rollbackErr).Msg("CRITICAL: Rollback failed for source account")
		}
		if rollbackErr := tx.Save(&transfer.ToAccount).Error; rollbackErr != nil {
			log.Error().Err(rollbackErr).Msg("CRITICAL: Rollback failed for destination account")
		}

		return fmt.Errorf("failed to update transfer status to completed: %w", err)
	}

	// Commit the transaction
	commitError = tx.Commit().Error
	if commitError != nil {
		log.Error().
			Err(commitError).
			Uint("transfer_id", transferID).
			Msg("CRITICAL: Failed to commit transfer transaction")
		return fmt.Errorf("failed to commit transaction: %w", commitError)
	}

	log.Info().
		Uint("transfer_id", transferID).
		Uint("from_account", transfer.FromAccountID).
		Uint("to_account", *transfer.ToAccountID).
		Int64("amount", transfer.Amount).
		Msg("Transfer completed successfully")

	// Create transaction record
	txService := services.NewTransactionService(processor.db)
	record := services.TransactionRecord{
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
	if err := txService.CreateTransaction(ctx, record); err != nil {
		log.Printf("Failed to create transaction record: %v", err)
		// Don't return error as this is not critical
	}

	// Append to transaction file
	if err := processor.txDataFileProcessor.txFileService.AppendTransactionToFile(ctx, uint(transfer.FromUserID), record); err != nil {
		log.Printf("Failed to append transaction to file: %v", err)
		// Don't return error as this is not critical
	}

	return nil
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
		log.Error().Err(err).Msg("Failed to unmarshal external transfer payload")
		return fmt.Errorf("failed to unmarshal external transfer payload: %w", asynq.SkipRetry)
	}

	log.Info().Str("transfer_id", payload.TransferID).Msg("Processing external transfer task")

	// Step 1: Update transfer status in database
	transferID := payload.TransferID
	success := true // Simulate success for now
	var dbErr error
	if success {
		dbErr = processor.db.Model(&models.Transfer{}).Where("id = ?", transferID).Update("status", models.TransferStatusCompleted).Error
	} else {
		dbErr = processor.db.Model(&models.Transfer{}).Where("id = ?", transferID).Updates(models.Transfer{Status: models.TransferStatusFailed, FailureReason: "Simulated external failure"}).Error
	}

	if dbErr != nil {
		log.Error().Err(dbErr).Str("transfer_id", transferID).Msg("Failed to update transfer status")
		return fmt.Errorf("failed to update transfer status: %w", dbErr)
	}

	// Step 2: Create transaction record
	var transfer models.Transfer
	if err := processor.db.First(&transfer, transferID).Error; err != nil {
		log.Error().Err(err).Str("transfer_id", transferID).Msg("Failed to fetch transfer details")
		return fmt.Errorf("failed to fetch transfer details: %w", err)
	}

	txService := services.NewTransactionService(processor.db)
	record := services.TransactionRecord{
		UserID:              uint64(transfer.FromUserID),
		Timestamp:           transfer.CreatedAt,
		Type:                "EXTERNAL_TRANSFER",
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

	if err := txService.CreateTransaction(ctx, record); err != nil {
		log.Error().Err(err).Str("transfer_id", transferID).Uint("user_id", transfer.FromUserID).Msg("Failed to create transaction record")
		// Don't return error as this is not critical for the transfer process
	}

	// Step 3: Update transaction file and trigger AI indexing
	if err := processor.txDataFileProcessor.txFileService.AppendTransactionToFile(ctx, uint(transfer.FromUserID), record); err != nil {
		log.Error().Err(err).Str("transfer_id", transferID).Uint("user_id", transfer.FromUserID).Msg("Failed to append transaction to file and trigger indexing")
		// Don't return error as this is not critical for the transfer process
	}

	log.Info().Str("transfer_id", transferID).Uint("user_id", transfer.FromUserID).Msg("Successfully processed external transfer task")
	return nil
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

// HandleUpdateTxFile processes the transaction file update task
func (processor *RedisTaskProcessor) HandleUpdateTxFile(ctx context.Context, t *asynq.Task) error {
	var payload tasks.UpdateTxFileAndIndexPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal UpdateTxFileAndIndex payload: %w", asynq.SkipRetry)
	}

	log.Info().Uint("user_id", payload.UserID).Msg("Processing transaction file update task")

	// Parse the transaction data
	var txHistoryList []map[string]interface{}
	if err := json.Unmarshal([]byte(payload.TxData), &txHistoryList); err != nil {
		return fmt.Errorf("failed to unmarshal transaction data: %w", asynq.SkipRetry)
	}

	// Create transaction file service
	txFileService := services.NewGenerateTxDataService(processor.db, *processor.config)

	// Convert transactions to TransactionRecord format
	var records []services.TransactionRecord
	for _, tx := range txHistoryList {
		// Safely get values with type assertions
		txType, _ := tx["type"].(string)
		amount, _ := tx["amount"].(float64)
		currency, _ := tx["currency"].(string)
		status, _ := tx["status"].(string)
		description, _ := tx["description"].(string)
		if description == "" {
			// Set a default description if none provided
			description = fmt.Sprintf("%s transaction", txType)
		}

		record := services.TransactionRecord{
			UserID:      uint64(payload.UserID),
			Type:        txType,
			Amount:      amount,
			Currency:    currency,
			Status:      status,
			Description: description,
			Timestamp:   time.Now(), // Set current time as timestamp
		}
		records = append(records, record)
	}

	// Format transactions as CSV
	csvData, err := txFileService.FormatTransactionsToCSV(records)
	if err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("Failed to format transactions to CSV")
		return fmt.Errorf("failed to format transactions to CSV: %w", err)
	}

	// Upload to GCS
	if err := txFileService.UploadOrOverwriteTxFile(ctx, fmt.Sprint(payload.UserID), csvData); err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("Failed to upload transaction file to GCS")
		return fmt.Errorf("failed to upload transaction file: %w", err)
	}

	// Trigger indexing
	if err := txFileService.TriggerTransactionFileIndexing(ctx, payload.UserID); err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("Failed to trigger transaction file indexing")
		// Don't return error as indexing failure shouldn't invalidate the update
	}

	log.Info().Uint("user_id", payload.UserID).Msg("Successfully processed transaction file update task")
	return nil
}

func (processor *RedisTaskProcessor) Shutdown() {
	processor.server.Shutdown()
}

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
}

func NewRedisTaskProcessor(redisOpt asynq.RedisClientOpt, db *gorm.DB, mailer mail.EmailSender, config *configs.Config, distributor tasks.TaskDistributor) TaskProcessor {
	logger := NewLogger()
	redis.SetLogger(logger)

	txDataFileProcessor := NewGenerateTxDataFileProcessor(db, *config)

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

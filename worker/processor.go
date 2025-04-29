package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/mail"
	"lazervaultGo/models"
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

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
)

type TaskProcessor interface {
	Start() error
	ProcessTaskSendVerifyEmail(ctx context.Context, task *asynq.Task) error
	ProcessTaskProcessTransfer(ctx context.Context, task *asynq.Task) error
	ProcessTaskSendPasswordResetOTP(ctx context.Context, task *asynq.Task) error
}

type RedisTaskProcessor struct {
	server      *asynq.Server
	db          *gorm.DB
	mailer      mail.EmailSender
	config      *configs.Config
	distributor tasks.TaskDistributor
}

func NewRedisTaskProcessor(redisOpt asynq.RedisClientOpt, db *gorm.DB, mailer mail.EmailSender, config *configs.Config, distributor tasks.TaskDistributor) TaskProcessor {
	logger := NewLogger()
	redis.SetLogger(logger)

	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Queues: map[string]int{
				QueueCritical: 10,
				QueueDefault:  5,
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
		server:      server,
		db:          db,
		mailer:      mailer,
		config:      config,
		distributor: distributor,
	}
}

func (processor *RedisTaskProcessor) Start() error {
	mux := asynq.NewServeMux()

	// Register handlers using closures to pass dependencies
	mux.HandleFunc(tasks.TaskSendVerifyEmail, func(ctx context.Context, task *asynq.Task) error {
		// HandleEmailSendVerifyUserTask is defined in task_send_email.go (worker package)
		return HandleEmailSendVerifyUserTask(ctx, task, processor.mailer)
	})
	mux.HandleFunc(tasks.TaskProcessTransfer, processor.ProcessTaskProcessTransfer) // Keep existing method if needed
	// Point password reset task type to the correct processor method
	mux.HandleFunc(tasks.TaskSendPasswordResetOTP, processor.ProcessTaskSendPasswordResetOTP)
	// Register new handlers
	mux.HandleFunc(tasks.TypeDepositProcess, func(ctx context.Context, task *asynq.Task) error {
		// HandleDepositProcessTask is defined in task_process_deposit.go (worker package)
		return HandleDepositProcessTask(ctx, task, processor.db, processor.mailer, processor.distributor)
	})
	mux.HandleFunc(tasks.TypeEmailSendDepositReversal, func(ctx context.Context, task *asynq.Task) error {
		// HandleEmailSendDepositReversalTask is defined in task_send_email.go (worker package)
		return HandleEmailSendDepositReversalTask(ctx, task, processor.mailer)
	})
	// Added withdrawal handlers
	mux.HandleFunc(tasks.TypeWithdrawalProcess, func(ctx context.Context, task *asynq.Task) error {
		// HandleWithdrawalProcessTask is defined in task_process_withdrawal.go (worker package)
		return HandleWithdrawalProcessTask(ctx, task, processor.db, processor.mailer, processor.distributor)
	})
	mux.HandleFunc(tasks.TypeEmailSendWithdrawalConf, func(ctx context.Context, task *asynq.Task) error {
		// HandleEmailSendWithdrawalConfirmationTask is defined in task_send_email.go (worker package)
		return HandleEmailSendWithdrawalConfirmationTask(ctx, task, processor.mailer)
	})
	mux.HandleFunc(tasks.TypeEmailSendWithdrawalFail, func(ctx context.Context, task *asynq.Task) error {
		// HandleEmailSendWithdrawalFailureTask is defined in task_send_email.go (worker package)
		return HandleEmailSendWithdrawalFailureTask(ctx, task, processor.mailer)
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

	return tx.Commit().Error
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

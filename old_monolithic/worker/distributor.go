package worker

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"

	"lazervaultGo/tasks"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
)

type RedisTaskDistributor struct {
	client *asynq.Client
}

func NewRedisTaskDistributor(redisClientOpt asynq.RedisClientOpt) tasks.TaskDistributor {
	client := asynq.NewClient(redisClientOpt)
	return &RedisTaskDistributor{
		client: client,
	}
}

func (distributor *RedisTaskDistributor) DistributeTaskSendVerifyEmail(
	ctx context.Context,
	payload *tasks.PayloadSendVerifyEmail,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(tasks.TaskSendVerifyEmail, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
}

// DistributeTaskProcessTransfer updated to accept encoding.BinaryMarshaler
func (distributor *RedisTaskDistributor) DistributeTaskProcessTransfer(
	ctx context.Context,
	payload encoding.BinaryMarshaler, // Changed payload type
	opts ...asynq.Option,
) error {
	jsonPayload, err := payload.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal process transfer payload: %w", err)
	}
	// Use the generic DistributeTask method internally
	return distributor.DistributeTask(ctx, tasks.TaskProcessTransfer, jsonPayload, opts...)
}

// Add missing DistributeTaskProcessExternalTransfer method
func (distributor *RedisTaskDistributor) DistributeTaskProcessExternalTransfer(
	ctx context.Context,
	payload encoding.BinaryMarshaler,
	opts ...asynq.Option,
) error {
	jsonPayload, err := payload.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal process external transfer payload: %w", err)
	}
	// Use the generic DistributeTask method internally
	return distributor.DistributeTask(ctx, tasks.TaskProcessExternalTransfer, jsonPayload, opts...)
}

func (distributor *RedisTaskDistributor) DistributeTaskSendPasswordResetOTP(
	ctx context.Context,
	payload *tasks.PayloadSendPasswordResetOTP,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(tasks.TaskSendPasswordResetOTP, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
}

func (distributor *RedisTaskDistributor) DistributeTaskSendPasswordResetEmailOTP(
	ctx context.Context,
	payload *tasks.PayloadSendPasswordResetEmailOTP,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(tasks.TaskSendPasswordResetEmailOTP, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
}

// Generic DistributeTask implementation
func (distributor *RedisTaskDistributor) DistributeTask(
	ctx context.Context,
	taskType string,
	payload []byte, // Accepts raw bytes
	opts ...asynq.Option,
) error {
	task := asynq.NewTask(taskType, payload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task type %s: %w", taskType, err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
}

// DistributeTaskDepositProcess enqueues a task to process a deposit.
func (distributor *RedisTaskDistributor) DistributeTaskDepositProcess(
	ctx context.Context,
	payload *tasks.DepositProcessPayload,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal deposit process payload: %w", err)
	}

	task := asynq.NewTask(tasks.TypeDepositProcessing, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue deposit process task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
}

// DistributeTaskWithdrawalProcess enqueues a task to process a withdrawal.
func (distributor *RedisTaskDistributor) DistributeTaskWithdrawalProcess(
	ctx context.Context,
	payload *tasks.WithdrawalProcessPayload,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal withdrawal process payload: %w", err)
	}

	task := asynq.NewTask(tasks.TypeWithdrawalProcessing, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue withdrawal process task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
}

// DistributeTaskSendWithdrawalConfirmation enqueues a task to send a withdrawal confirmation email.
func (distributor *RedisTaskDistributor) DistributeTaskSendWithdrawalConfirmation(
	ctx context.Context,
	payload *tasks.EmailSendWithdrawalConfirmationPayload,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal withdrawal confirmation email payload: %w", err)
	}

	task := asynq.NewTask(tasks.TypeEmailSendWithdrawalConf, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue withdrawal confirmation email task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
}

// DistributeTaskGenerateTxDataFile enqueues a task to generate a user's transaction data file.
func (distributor *RedisTaskDistributor) DistributeTaskGenerateTxDataFile(
	ctx context.Context,
	payload *tasks.GenerateTxDataFilePayload,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal generate tx data file payload: %w", err)
	}

	// Use the generic DistributeTask method internally
	return distributor.DistributeTask(ctx, tasks.TypeGenerateTxDataFile, jsonPayload, opts...)
}

// DistributeProcessTransferTask distributes a transfer processing task
func (distributor *RedisTaskDistributor) DistributeProcessTransferTask(
	ctx context.Context,
	payload *tasks.ProcessTransferPayload,
	opts ...asynq.Option,
) error {
	jsonPayload, err := payload.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal process transfer payload: %w", err)
	}
	return distributor.DistributeTask(ctx, tasks.TaskProcessTransfer, jsonPayload, opts...)
}

// DistributeScheduledTransferCheckTask distributes a scheduled transfer check task
func (distributor *RedisTaskDistributor) DistributeScheduledTransferCheckTask(
	ctx context.Context,
	payload *tasks.ScheduledTransferCheckPayload,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal scheduled transfer check payload: %w", err)
	}
	return distributor.DistributeTask(ctx, tasks.TypeScheduledTransferCheck, jsonPayload, opts...)
}

// DistributeScheduledAutoSaveCheckTask distributes a scheduled auto-save check task
func (distributor *RedisTaskDistributor) DistributeScheduledAutoSaveCheckTask(
	ctx context.Context,
	payload *tasks.ScheduledAutoSaveCheckPayload,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal scheduled auto-save check payload: %w", err)
	}
	return distributor.DistributeTask(ctx, tasks.TypeScheduledAutoSaveCheck, jsonPayload, opts...)
}

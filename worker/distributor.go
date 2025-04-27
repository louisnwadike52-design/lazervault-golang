package worker

import (
	"context"
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

func (distributor *RedisTaskDistributor) DistributeTaskProcessTransfer(
	ctx context.Context,
	payload *tasks.PayloadProcessTransfer,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(tasks.TaskProcessTransfer, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}

	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return nil
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

	task := asynq.NewTask(tasks.TypeDepositProcess, jsonPayload, opts...)
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

	task := asynq.NewTask(tasks.TypeWithdrawalProcess, jsonPayload, opts...)
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

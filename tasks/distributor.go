package tasks

import (
	"context"
	"encoding"
	"fmt"

	"github.com/hibiken/asynq"
)

// TaskDistributor defines the interface for enqueuing tasks.
type TaskDistributor interface {
	// Generic method to distribute any task
	DistributeTask(
		ctx context.Context,
		taskType string,
		payload []byte,
		opts ...asynq.Option,
	) error

	// Specific methods can still exist if preferred for type safety
	DistributeTaskSendVerifyEmail(
		ctx context.Context,
		payload *PayloadSendVerifyEmail, // Payloads are defined in payloads.go
		opts ...asynq.Option,
	) error
	// DistributeTaskProcessTransfer - specific payload version (optional)
	// DistributeTaskProcessTransfer(
	// 	ctx context.Context,
	// 	payload *PayloadProcessTransfer,
	// 	opts ...asynq.Option,
	// ) error
	DistributeTaskSendPasswordResetOTP(
		ctx context.Context,
		payload *PayloadSendPasswordResetOTP,
		opts ...asynq.Option,
	) error

	// Deposit processing task
	DistributeTaskDepositProcess(
		ctx context.Context,
		payload *DepositProcessPayload, // Use specific payload type
		opts ...asynq.Option,
	) error

	// Withdrawal processing task
	DistributeTaskWithdrawalProcess(
		ctx context.Context,
		payload *WithdrawalProcessPayload, // Defined in payloads.go
		opts ...asynq.Option,
	) error

	// Withdrawal confirmation email task
	DistributeTaskSendWithdrawalConfirmation(
		ctx context.Context,
		payload *EmailSendWithdrawalConfirmationPayload, // Defined in payloads.go
		opts ...asynq.Option,
	) error

	// Use encoding.BinaryMarshaler for transfer tasks to accept different payload structs
	DistributeTaskProcessTransfer(ctx context.Context, payload encoding.BinaryMarshaler, opts ...asynq.Option) error
	DistributeTaskProcessExternalTransfer(ctx context.Context, payload encoding.BinaryMarshaler, opts ...asynq.Option) error
}

// RedisTaskDistributor implements TaskDistributor using Redis.
type RedisTaskDistributor struct {
	client *asynq.Client
}

func NewRedisTaskDistributor(redisOpt asynq.RedisClientOpt) TaskDistributor {
	client := asynq.NewClient(redisOpt)
	return &RedisTaskDistributor{client: client}
}

// Implement the generic DistributeTask method
func (distributor *RedisTaskDistributor) DistributeTask(ctx context.Context, taskType string, payload []byte, opts ...asynq.Option) error {
	task := asynq.NewTask(taskType, payload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task %s: %w", taskType, err)
	}
	fmt.Printf("Enqueued task %s: id=%s queue=%s\n", taskType, info.ID, info.Queue)
	return nil
}

// Implement DistributeTaskProcessTransfer using encoding.BinaryMarshaler
func (distributor *RedisTaskDistributor) DistributeTaskProcessTransfer(ctx context.Context, payload encoding.BinaryMarshaler, opts ...asynq.Option) error {
	jsonPayload, err := payload.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal payload for %s: %w", TaskProcessTransfer, err)
	}
	// Use the generic DistributeTask method internally
	return distributor.DistributeTask(ctx, TaskProcessTransfer, jsonPayload, opts...)
}

// Implement DistributeTaskProcessExternalTransfer using encoding.BinaryMarshaler
func (distributor *RedisTaskDistributor) DistributeTaskProcessExternalTransfer(ctx context.Context, payload encoding.BinaryMarshaler, opts ...asynq.Option) error {
	jsonPayload, err := payload.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal payload for %s: %w", TaskProcessExternalTransfer, err)
	}
	// Use the generic DistributeTask method internally
	return distributor.DistributeTask(ctx, TaskProcessExternalTransfer, jsonPayload, opts...)
}

// --- Implementations for specific task types (forwarding to generic method) ---

func (distributor *RedisTaskDistributor) DistributeTaskSendVerifyEmail(ctx context.Context, payload *PayloadSendVerifyEmail, opts ...asynq.Option) error {
	panic("DistributeTaskSendVerifyEmail not implemented for RedisTaskDistributor - Use DistributeTask")
	// Or marshal and call DistributeTask:
	// jsonPayload, err := json.Marshal(payload)
	// if err != nil {
	// 	return fmt.Errorf("failed to marshal payload for %s: %w", TaskSendVerifyEmail, err)
	// }
	// return distributor.DistributeTask(ctx, TaskSendVerifyEmail, jsonPayload, opts...)
}

func (distributor *RedisTaskDistributor) DistributeTaskSendPasswordResetOTP(ctx context.Context, payload *PayloadSendPasswordResetOTP, opts ...asynq.Option) error {
	panic("DistributeTaskSendPasswordResetOTP not implemented for RedisTaskDistributor - Use DistributeTask")
}

func (distributor *RedisTaskDistributor) DistributeTaskDepositProcess(ctx context.Context, payload *DepositProcessPayload, opts ...asynq.Option) error {
	panic("DistributeTaskDepositProcess not implemented for RedisTaskDistributor - Use DistributeTask")
}

func (distributor *RedisTaskDistributor) DistributeTaskWithdrawalProcess(ctx context.Context, payload *WithdrawalProcessPayload, opts ...asynq.Option) error {
	panic("DistributeTaskWithdrawalProcess not implemented for RedisTaskDistributor - Use DistributeTask")
}

func (distributor *RedisTaskDistributor) DistributeTaskSendWithdrawalConfirmation(ctx context.Context, payload *EmailSendWithdrawalConfirmationPayload, opts ...asynq.Option) error {
	panic("DistributeTaskSendWithdrawalConfirmation not implemented for RedisTaskDistributor - Use DistributeTask")
}

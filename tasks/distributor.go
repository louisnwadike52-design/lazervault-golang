package tasks

import (
	"context"

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
	DistributeTaskProcessTransfer(
		ctx context.Context,
		payload *PayloadProcessTransfer,
		opts ...asynq.Option,
	) error
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
}

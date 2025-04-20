package tasks

import (
	"context"

	"github.com/hibiken/asynq"
)

// TaskDistributor defines the interface for enqueuing tasks.
type TaskDistributor interface {
	DistributeTaskSendVerifyEmail(
		ctx context.Context,
		payload *PayloadSendVerifyEmail, // We might need to move PayloadSendVerifyEmail here too
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
}

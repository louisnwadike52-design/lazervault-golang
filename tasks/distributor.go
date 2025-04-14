package tasks

import (
	"context"

	"github.com/hibiken/asynq"
)

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
}

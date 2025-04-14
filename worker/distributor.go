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

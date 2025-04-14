package worker

import (
	"context"
	"fmt"
	"lazervaultGo/mail"

	"github.com/go-redis/redis/v8"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
)

type TaskProcessor interface {
	Start(redisOpt asynq.RedisClientOpt) error
	ProcessTaskSendVerifyEmail(ctx context.Context, task *asynq.Task) error
}

type RedisTaskProcessor struct {
	server *asynq.Server
	db     *gorm.DB
	mailer mail.EmailSender
}

func NewRedisTaskProcessor(redisOpt asynq.RedisClientOpt, db *gorm.DB, mailer mail.EmailSender) TaskProcessor {
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
		server: server,
		db:     db,
		mailer: mailer,
	}
}

func (processor *RedisTaskProcessor) Start(redisOpt asynq.RedisClientOpt) error {
	mux := asynq.NewServeMux()

	mux.HandleFunc(TaskSendVerifyEmail, processor.ProcessTaskSendVerifyEmail)

	if err := processor.server.Start(mux); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	// Print Redis server address
	fmt.Printf("Started redis server on %s\n", redisOpt.Addr)

	return nil
}

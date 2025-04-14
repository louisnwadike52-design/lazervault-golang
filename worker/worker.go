package worker

import (
	"context"
	"lazervaultGo/configs"
	"lazervaultGo/mail"
	"lazervaultGo/tasks"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type RedisWorker struct {
	distributor tasks.TaskDistributor
	processor   TaskProcessor
}

type RedisStorage struct {
	redisStorage *asynq.Client
	ctx          context.Context
}

func NewRedisStorage(redisOpt asynq.RedisClientOpt) *RedisStorage {
	return &RedisStorage{redisStorage: asynq.NewClient(redisOpt), ctx: context.Background()}
}

func NewRedisWorker(redisOpt asynq.RedisClientOpt, db *gorm.DB, mailer mail.EmailSender, config *configs.Config) *RedisWorker {
	distributor := NewRedisTaskDistributor(redisOpt)
	processor := NewRedisTaskProcessor(redisOpt, db, mailer, config)

	go func() {
		if err := processor.Start(); err != nil {
			log.Fatal().Err(err).Msg("failed to start task processor")
		}
	}()
	log.Info().Msg("task processor started")

	return &RedisWorker{
		distributor: distributor,
		processor:   processor,
	}
}

func (rw *RedisWorker) GetDistributor() tasks.TaskDistributor {
	return rw.distributor
}

func (rw *RedisWorker) GetProcessor() TaskProcessor {
	return rw.processor
}

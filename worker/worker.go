package worker

import (
	"context"
	"lazervaultGo/mail"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type RedisWorker struct {
	distributor  TaskDistributor
	processor    TaskProcessor
	redisStorage *RedisStorage
}

type RedisStorage struct {
	redisStorage *asynq.Client
	ctx          context.Context
}

func NewRedisStorage(redisOpt asynq.RedisClientOpt) *RedisStorage {
	return &RedisStorage{redisStorage: asynq.NewClient(redisOpt), ctx: context.Background()}
}

func NewRedisWorker(redisOpt asynq.RedisClientOpt, db *gorm.DB, mailer mail.EmailSender, redisStorage *RedisStorage) *RedisWorker {
	distributor := NewRedisTaskDistributor(redisOpt)
	processor := NewRedisTaskProcessor(redisOpt, db, mailer)
	processor.Start(redisOpt)

	return &RedisWorker{
		distributor:  distributor,
		processor:    processor,
		redisStorage: redisStorage,
	}
}

package main

import (
	"lazervaultGo/configs"
	"lazervaultGo/database"
	"lazervaultGo/grpcApi"
	"lazervaultGo/mail"
	"lazervaultGo/token"
	"lazervaultGo/worker"
	"log"

	"github.com/hibiken/asynq"
)

func main() {
	// Load configuration
	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal("cannot load config:", err)
	}

	// Initialize database
	db, err := database.ConnectDB(config)
	if err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	migrator := database.NewMigrator(db)

	// Run migrations
	if err := migrator.RunMigrations(); err != nil {
		log.Fatal("cannot run migrations:", err)
	}

	// Create token maker
	tokenMaker, err := token.NewPasetoMaker(config.TokenSymmetricKey)
	if err != nil {
		log.Fatal("cannot create token maker:", err)
	}

	mailer := mail.NewGmailSender(config.EmailSenderName, config.EmailSenderAddress, config.EmailSenderPassword)

	redisStorage := worker.NewRedisStorage(asynq.RedisClientOpt{
		Addr:     config.RedisServerAddr,
		Password: "",
		DB:       0,
	})

	redisWorker := worker.NewRedisWorker(asynq.RedisClientOpt{
		Addr:     config.RedisServerAddr,
		Password: "",
		DB:       0,
	}, db, mailer, redisStorage)

	// Create and start server
	server := grpcApi.NewServer(db, &config, tokenMaker, redisWorker)

	// Handle graceful shutdown
	go handleShutdown(server)

	// Start server
	if err := server.Start(); err != nil {
		log.Fatal("cannot start server:", err)
	}
}

func handleShutdown(server *grpcApi.Server) {
	// Setup signal handling for graceful shutdown
	// ... implementation ...
}

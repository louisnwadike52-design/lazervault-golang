package main

import (
	"lazervaultGo/configs"
	"lazervaultGo/database"
	"lazervaultGo/grpcApi"
	"lazervaultGo/mail"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"lazervaultGo/worker"

	"github.com/rs/zerolog/log"

	"github.com/hibiken/asynq"
)

func main() {
	// Load configuration
	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("cannot load config")
	}

	// Initialize database
	db, err := database.ConnectDB(config)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot connect to db")
	}

	migrator := database.NewMigrator(db)

	// // Drop all tables
	// if err := migrator.DropAllTables(); err != nil {
	// 	log.Fatal().Err(err).Msg("cannot drop tables") // Adjusted if uncommented
	// }

	// Run migrations
	if err := migrator.RunMigrations(); err != nil {
		log.Fatal().Err(err).Msg("cannot run migrations")
	}

	// Create token maker
	tokenMaker, err := token.NewPasetoMaker(config.TokenSymmetricKey)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot create token maker")
	}

	mailer := mail.NewGmailSender(config.EmailSenderName, config.EmailSenderAddress, config.EmailSenderPassword)

	// Setup Redis connection options
	redisOpt := asynq.RedisClientOpt{
		Addr:     config.RedisServerAddr,
		Password: "", // Add password if needed
		DB:       0,  // Use default DB
	}

	// 1. Create the Task Distributor
	taskDistributor := worker.NewRedisTaskDistributor(redisOpt)

	// 2. Initialize AIChatService (needs distributor)
	aiChatService := services.NewAIChatService(db, &config, taskDistributor)

	// 3. Create the full Redis Worker (processor starts automatically inside)
	//    Pass the initialized aiChatService to the worker constructor.
	redisWorker := worker.NewRedisWorker(redisOpt, db, mailer, &config, aiChatService)

	// 4. Create and initialize the gRPC/HTTP server
	//    Pass the redisWorker (which contains distributor & processor)
	server, err := grpcApi.NewServer(db, &config, tokenMaker, redisWorker)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot create server")
	}

	// Handle graceful shutdown
	go handleShutdown(server)

	// Start server
	if err := server.Start(); err != nil {
		log.Fatal().Err(err).Msg("cannot start server")
	}
}

func handleShutdown(server *grpcApi.Server) {
	// Setup signal handling for graceful shutdown
	// ... implementation ...
}

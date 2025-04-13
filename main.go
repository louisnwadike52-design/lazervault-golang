package main

import (
	"lazervaultGo/configs"
	"lazervaultGo/database"
	"lazervaultGo/grpcApi"
	"lazervaultGo/token"
	"log"
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

	// Create and start server
	server := grpcApi.NewServer(db, &config, tokenMaker)

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

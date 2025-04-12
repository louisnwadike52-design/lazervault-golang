package main

import (
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/database"
	"lazervaultGo/grpcApi"
	"lazervaultGo/restApi"
	"lazervaultGo/token"
	"log"
)

func main() {
	// Load configuration
	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal("Cannot load config:", err)
	}

	// Initialize database
	db, err := database.ConnectDB(config)
	if err != nil {
		log.Fatal("Cannot connect to db:", err)
	}

	// Auto migrate database
	if err := database.AutoMigrateDB(db); err != nil {
		log.Fatal("Cannot auto migrate db:", err)
	}

	// Initialize token maker
	tokenMaker, err := token.NewJWTMaker(config.TokenSymmetricKey)
	if err != nil {
		log.Fatal("cannot create token maker:", err)
	}

	errChan := make(chan error, 2)

	// gRPC server goroutine
	go func() {
		log.Printf("Starting gRPC server on port %s", config.GRPCServerPort)
		if err := grpcApi.RunGRPCServer(db, tokenMaker, &config); err != nil {
			errChan <- fmt.Errorf("gRPC server error: %v", err)
		}
	}()

	// REST server goroutine
	go func() {
		server := restApi.Server{
			DB: db,
		}
		if _, err := server.Serve(); err != nil {
			errChan <- fmt.Errorf("REST server error: %v", err)
		}
	}()

	// Wait for any errors
	// select {
	// case err := <-errChan:
	// 	log.Fatal(err)
	// }

	if err := <-errChan; err != nil {
		log.Fatal(err)
	}
}

package config

import (
	"fmt"
	"os"
)

// MicroserviceConfig holds configuration for microservice connections
type MicroserviceConfig struct {
	AuthServiceAddr     string
	AccountsServiceAddr string
}

// LoadMicroserviceConfig loads microservice addresses from environment
func LoadMicroserviceConfig() (*MicroserviceConfig, error) {
	authAddr := os.Getenv("AUTH_SERVICE_ADDR")
	if authAddr == "" {
		authAddr = "127.0.0.1:50051" // Default auth service address
	}

	accountsAddr := os.Getenv("ACCOUNTS_SERVICE_ADDR")
	if accountsAddr == "" {
		accountsAddr = "127.0.0.1:50052" // Default accounts service address
	}

	config := &MicroserviceConfig{
		AuthServiceAddr:     authAddr,
		AccountsServiceAddr: accountsAddr,
	}

	fmt.Printf("📡 Core Gateway Microservice Config:\n")
	fmt.Printf("   Auth Service: %s\n", config.AuthServiceAddr)
	fmt.Printf("   Accounts Service: %s\n", config.AccountsServiceAddr)

	return config, nil
}

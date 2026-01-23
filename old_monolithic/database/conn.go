package database

import (
	"lazervaultGo/configs"
	"log"
	"time"

	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ConnectDB(config configs.Config) (*gorm.DB, error) {
	log.Printf("===== DATABASE CONNECTION DEBUG =====")
	log.Printf("DB_USER: '%s' (len=%d)", config.DBUser, len(config.DBUser))
	log.Printf("DB_NAME: '%s' (len=%d)", config.DBName, len(config.DBName))
	log.Printf("DB_HOST: '%s' (len=%d)", config.DBHost, len(config.DBHost))
	log.Printf("DB_PORT: '%s' (len=%d)", config.DBPort, len(config.DBPort))
	log.Printf("DB_PASSWORD: '%s' (len=%d)", config.DBPassword, len(config.DBPassword))
	log.Printf("DB_SSLMODE: '%s' (len=%d)", config.DBSSLMode, len(config.DBSSLMode))

	// Use URI format for better compatibility
	var dsn string
	if config.DBPassword != "" {
		dsn = fmt.Sprintf(
			"postgresql://%s:%s@%s:%s/%s?sslmode=%s",
			config.DBUser,
			config.DBPassword,
			config.DBHost,
			config.DBPort,
			config.DBName,
			config.DBSSLMode,
		)
		log.Printf("Using password-based DSN")
	} else {
		dsn = fmt.Sprintf(
			"postgresql://%s@%s:%s/%s?sslmode=%s",
			config.DBUser,
			config.DBHost,
			config.DBPort,
			config.DBName,
			config.DBSSLMode,
		)
		log.Printf("Using no-password DSN")
	}

	log.Printf("Generated DSN: %s", dsn)
	log.Printf("=====================================")

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info), // Changed to Info to see SQL queries
	})

	if err != nil {
		log.Println("Error connecting to database: ", err)
		return nil, err
	}

	sqlDb, err := db.DB()
	if err != nil {
		log.Println("Error getting database: ", err)
		return nil, err
	}

	sqlDb.SetMaxIdleConns(10)
	sqlDb.SetMaxOpenConns(100)
	sqlDb.SetConnMaxLifetime(time.Hour)

	log.Println("Successfully connected to database", config.DBName, "at", config.DBHost, "on port", config.DBPort, "with user", config.DBUser)

	return db, nil

}

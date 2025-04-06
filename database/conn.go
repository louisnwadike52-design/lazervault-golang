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
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		config.DBHost,
		config.DBPort,
		config.DBUser,
		config.DBPassword,
		config.DBName,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
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

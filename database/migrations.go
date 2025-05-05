package database

import (
	"fmt"
	"lazervaultGo/models"

	"gorm.io/gorm"
)

type Migrator struct {
	db *gorm.DB
}

func NewMigrator(db *gorm.DB) *Migrator {
	return &Migrator{db: db}
}

func (m *Migrator) DropAllTables() error {
	return m.db.Migrator().DropTable(
		&models.User{},
		&models.Account{},
		&models.Session{},
		&models.Transfer{},
		&models.Deposit{},
		&models.Withdrawal{},
		&models.Recipient{},
		&models.ExchangeTransaction{},
		&models.UserTransactionFile{},
		&models.AIChatHistory{},
		&models.UserChatHistoryFile{},
	)
}

func (m *Migrator) RunMigrations() error {
	// Enable UUID generation extension if not already enabled
	err := m.db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`).Error
	if err != nil {
		return fmt.Errorf("failed to enable uuid-ossp extension: %w", err)
	}

	// Proceed with AutoMigrate
	return m.db.AutoMigrate(
		&models.User{},
		&models.Account{},
		&models.Session{},
		&models.Transfer{},
		&models.Deposit{},
		&models.Withdrawal{},
		&models.Recipient{},
		&models.ExchangeTransaction{},
		&models.UserTransactionFile{},
		&models.AIChatHistory{},
		&models.UserChatHistoryFile{},
	)
}

func (m *Migrator) CreateSessionsTable() error {
	return m.db.AutoMigrate(&models.Session{})
}

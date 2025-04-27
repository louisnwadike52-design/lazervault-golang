package database

import (
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
		&models.FailedTransfer{},
		&models.Deposit{},
		&models.FailedDeposit{},
		&models.Withdrawal{},
		&models.Recipient{},
	)
}

func (m *Migrator) RunMigrations() error {
	return m.db.AutoMigrate(
		&models.User{},
		&models.Account{},
		&models.Session{},
		&models.Transfer{},
		&models.FailedTransfer{},
		&models.Deposit{},
		&models.FailedDeposit{},
		&models.Withdrawal{},
		&models.Recipient{},
	)
}

func (m *Migrator) CreateSessionsTable() error {
	return m.db.AutoMigrate(&models.Session{})
}

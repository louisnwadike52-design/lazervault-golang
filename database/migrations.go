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
	// Drop tables in reverse order of dependencies
	err := m.db.Exec(`
		DROP TABLE IF EXISTS 
			insurance_claims,
			insurance_payments,
			insurances,
			payment_disputes,
			tagged_invoices,
			user_payment_methods,
			user_account_balances,
			invoice_payment_transactions,
			invoices,
			user_voice_files,
			user_chat_history_files,
			ai_chat_histories,
			user_transaction_files,
			exchange_transactions,
			recipients,
			withdrawals,
			deposits,
			transfers,
			sessions,
			accounts,
			users CASCADE;
	`).Error

	if err != nil {
		return fmt.Errorf("failed to drop tables: %w", err)
	}

	return nil
}

func (m *Migrator) RunMigrations() error {
	// Enable UUID extension
	m.db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`)

	// Drop all tables and recreate them
	// err := m.DropAllTables()
	// if err != nil {
	// 	return fmt.Errorf("failed to drop tables: %w", err)
	// }

	// Run migrations
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
		&models.UserVoiceFile{},
		&models.Invoice{},
		&models.InvoicePaymentTransaction{},
		&models.UserAccountBalance{},
		&models.UserPaymentMethod{},
		&models.TaggedInvoice{},
		&models.PaymentDispute{},
		&models.Insurance{},
		&models.InsurancePayment{},
		&models.InsuranceClaim{},
	)
}

func (m *Migrator) CreateSessionsTable() error {
	return m.db.AutoMigrate(&models.Session{})
}

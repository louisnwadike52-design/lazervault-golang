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
			invitations,
			contribution_receipts,
			payout_transactions,
			payout_schedules,
			contribution_payments,
			contributions,
			group_members,
			group_accounts,
			budget_alerts,
			budgets,
			expenses,
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
	err := m.DropAllTables()
	if err != nil {
		return fmt.Errorf("failed to drop tables: %w", err)
	}

	// Migrate User table first WITHOUT foreign key constraints
	// This prevents GORM from creating backwards foreign keys from users.user_id to other tables
	err = m.db.Migrator().AutoMigrate(&models.User{})
	if err != nil {
		return fmt.Errorf("failed to migrate User table: %w", err)
	}

	// Now migrate all other tables - they will correctly create foreign keys TO users.id
	return m.db.AutoMigrate(
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
		&models.SyncedContact{},
		&models.SyncPreferences{},
		&models.Expense{},
		&models.Budget{},
		&models.BudgetAlert{},
		// Financial Statistics Models
		&models.IncomeSource{},
		&models.Investment{},
		&models.FinancialGoal{},
		&models.SavingsGoal{},
		&models.RecurringBill{},
		// Auth & Verification Models
		&models.EmailVerification{},
		&models.PasswordResetOTP{},
		// Group Account Models
		&models.GroupAccount{},
		&models.GroupMember{},
		&models.Contribution{},
		&models.ContributionPayment{},
		&models.PayoutSchedule{},
		&models.PayoutTransaction{},
		&models.ContributionReceipt{},
		// Invitation Model
		&models.Invitation{},
	)
}

func (m *Migrator) CreateSessionsTable() error {
	return m.db.AutoMigrate(&models.Session{})
}

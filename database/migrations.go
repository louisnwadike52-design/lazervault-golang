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
			autosave_transactions,
			autosave_rules,
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
			ticket_replies,
			contact_messages,
			support_tickets,
			device_permissions,
			facial_data,
			id_documents,
			sessions,
			accounts,
			users CASCADE;
	`).Error

	if err != nil {
		return fmt.Errorf("failed to drop tables: %w", err)
	}

	return nil
}

func (m *Migrator) AddMultiCountrySupport() error {
	// Add preferred_countries and active_country to user_preferences
	err := m.db.Exec(`
		ALTER TABLE user_preferences
		ADD COLUMN IF NOT EXISTS preferred_countries text[] DEFAULT '{}',
		ADD COLUMN IF NOT EXISTS active_country varchar(10) DEFAULT '';
	`).Error
	if err != nil {
		return fmt.Errorf("failed to add multi-country fields to user_preferences: %w", err)
	}

	// Add missing fields to recipients table
	err = m.db.Exec(`
		ALTER TABLE recipients
		ADD COLUMN IF NOT EXISTS email varchar(255) DEFAULT '',
		ADD COLUMN IF NOT EXISTS phone_number varchar(50) DEFAULT '',
		ADD COLUMN IF NOT EXISTS currency varchar(10) DEFAULT '',
		ADD COLUMN IF NOT EXISTS swift_code varchar(20) DEFAULT '',
		ADD COLUMN IF NOT EXISTS iban varchar(50) DEFAULT '';
	`).Error
	if err != nil {
		return fmt.Errorf("failed to add fields to recipients: %w", err)
	}

	// Add country field to accounts table
	err = m.db.Exec(`
		ALTER TABLE accounts
		ADD COLUMN IF NOT EXISTS country varchar(10) NOT NULL DEFAULT 'US';
	`).Error
	if err != nil {
		return fmt.Errorf("failed to add country field to accounts: %w", err)
	}

	// Create index on country column
	err = m.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_accounts_country ON accounts(country);
	`).Error
	if err != nil {
		return fmt.Errorf("failed to create country index on accounts: %w", err)
	}

	// Update existing accounts to set country based on currency
	err = m.db.Exec(`
		UPDATE accounts SET country =
		CASE
			WHEN currency = 'USD' THEN 'US'
			WHEN currency = 'GBP' THEN 'GB'
			WHEN currency = 'NGN' THEN 'NG'
			WHEN currency = 'EUR' THEN 'EU'
			WHEN currency = 'CAD' THEN 'CA'
			WHEN currency = 'AUD' THEN 'AU'
			WHEN currency = 'INR' THEN 'IN'
			WHEN currency = 'CNY' THEN 'CN'
			WHEN currency = 'JPY' THEN 'JP'
			WHEN currency = 'KES' THEN 'KE'
			WHEN currency = 'ZAR' THEN 'ZA'
			ELSE 'US'
		END
		WHERE country = 'US';
	`).Error
	if err != nil {
		return fmt.Errorf("failed to update country values in accounts: %w", err)
	}

	// Create composite index for efficient filtering by user and country
	err = m.db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_accounts_owner_country ON accounts(owner_user_id, country);
	`).Error
	if err != nil {
		return fmt.Errorf("failed to create composite index on accounts: %w", err)
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

	// Migrate User table first WITHOUT foreign key constraints
	// This prevents GORM from creating backwards foreign keys from users.user_id to other tables
	err := m.db.Migrator().AutoMigrate(&models.User{})
	if err != nil {
		return fmt.Errorf("failed to migrate User table: %w", err)
	}

	// Now migrate all other tables - they will correctly create foreign keys TO users.id
	err = m.db.AutoMigrate(
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
		// User Preferences Model
		&models.UserPreferences{},
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
		// Tag Pay Models
		&models.TagPay{},
		&models.TagPayTransaction{},
		&models.MoneyRequest{},
		&models.UserTag{},
		// Support Models
		&models.SupportTicket{},
		&models.TicketReply{},
		&models.ContactMessage{},
		// Identity Verification Models
		&models.IDDocument{},
		&models.FacialData{},
		&models.DevicePermission{},
		// Auto-Save Models
		&models.AutoSaveRule{},
		&models.AutoSaveTransaction{},
	)
	if err != nil {
		return fmt.Errorf("failed to run auto migrations: %w", err)
	}

	// Run multi-country support migration
	err = m.AddMultiCountrySupport()
	if err != nil {
		return fmt.Errorf("failed to add multi-country support: %w", err)
	}

	return nil
}

func (m *Migrator) CreateSessionsTable() error {
	return m.db.AutoMigrate(&models.Session{})
}

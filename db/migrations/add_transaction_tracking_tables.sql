-- Migration: Add Income and Expenditure Transaction Tracking Tables
-- Date: December 28, 2025
-- Purpose: Automatically track all financial transactions across the platform for statistics

-- ========================================
-- INCOME TRANSACTIONS TABLE
-- ========================================

CREATE TABLE IF NOT EXISTS income_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id BIGINT NOT NULL,
    amount DECIMAL(20, 2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    source_type VARCHAR(50) NOT NULL,
    source_id VARCHAR(255),
    source_reference VARCHAR(500),
    category VARCHAR(50),
    description TEXT,
    sender_id BIGINT,
    sender_name VARCHAR(255),
    transaction_date TIMESTAMP NOT NULL DEFAULT NOW(),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Indexes for income_transactions
CREATE INDEX idx_income_txn_user ON income_transactions(user_id);
CREATE INDEX idx_income_txn_amount ON income_transactions(amount);
CREATE INDEX idx_income_txn_source_type ON income_transactions(source_type);
CREATE INDEX idx_income_txn_source_id ON income_transactions(source_id);
CREATE INDEX idx_income_txn_category ON income_transactions(category);
CREATE INDEX idx_income_txn_date ON income_transactions(transaction_date);
CREATE INDEX idx_income_txn_sender ON income_transactions(sender_id);
CREATE INDEX idx_income_txn_deleted ON income_transactions(deleted_at);

-- ========================================
-- EXPENDITURE TRANSACTIONS TABLE
-- ========================================

CREATE TABLE IF NOT EXISTS expenditure_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id BIGINT NOT NULL,
    amount DECIMAL(20, 2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    expense_type VARCHAR(50) NOT NULL,
    expense_id VARCHAR(255),
    expense_reference VARCHAR(500),
    category VARCHAR(50),
    recipient_id BIGINT,
    recipient_name VARCHAR(255),
    merchant VARCHAR(255),
    description TEXT,
    transaction_date TIMESTAMP NOT NULL DEFAULT NOW(),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Indexes for expenditure_transactions
CREATE INDEX idx_expenditure_txn_user ON expenditure_transactions(user_id);
CREATE INDEX idx_expenditure_txn_amount ON expenditure_transactions(amount);
CREATE INDEX idx_expenditure_txn_type ON expenditure_transactions(expense_type);
CREATE INDEX idx_expenditure_txn_expense_id ON expenditure_transactions(expense_id);
CREATE INDEX idx_expenditure_txn_category ON expenditure_transactions(category);
CREATE INDEX idx_expenditure_txn_date ON expenditure_transactions(transaction_date);
CREATE INDEX idx_expenditure_txn_recipient ON expenditure_transactions(recipient_id);
CREATE INDEX idx_expenditure_txn_merchant ON expenditure_transactions(merchant);
CREATE INDEX idx_expenditure_txn_deleted ON expenditure_transactions(deleted_at);

-- ========================================
-- COMMENTS FOR DOCUMENTATION
-- ========================================

COMMENT ON TABLE income_transactions IS 'Automatically tracks all income operations across the platform (deposits, transfers received, invoice payments received, etc.)';
COMMENT ON TABLE expenditure_transactions IS 'Automatically tracks all expenditure operations across the platform (withdrawals, transfers sent, invoice payments made, bill payments, etc.)';

COMMENT ON COLUMN income_transactions.source_type IS 'Type of income source: deposit, transfer_received, invoice_payment_received, tagged_invoice_payment_received, etc.';
COMMENT ON COLUMN income_transactions.metadata IS 'Flexible JSON field for service-specific data';

COMMENT ON COLUMN expenditure_transactions.expense_type IS 'Type of expense: withdrawal, transfer_sent, invoice_payment_made, bill_payment, exchange, etc.';
COMMENT ON COLUMN expenditure_transactions.metadata IS 'Flexible JSON field for service-specific data';

-- ========================================
-- ROLLBACK (if needed)
-- ========================================

-- To rollback this migration, run:
-- DROP TABLE IF EXISTS income_transactions CASCADE;
-- DROP TABLE IF EXISTS expenditure_transactions CASCADE;

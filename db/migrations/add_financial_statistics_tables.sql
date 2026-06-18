-- Migration: Add Financial Statistics Tables
-- Created: 2025-12-10
-- Description: Adds tables for income sources, investments, financial goals, savings goals, and recurring bills

-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ================================================
-- INCOME SOURCES TABLE
-- ================================================
CREATE TABLE IF NOT EXISTS income_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    amount DECIMAL(15,2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    category VARCHAR(50) NOT NULL,
    is_recurring BOOLEAN DEFAULT FALSE,
    recurrence_pattern VARCHAR(20),
    last_received TIMESTAMP,
    next_expected TIMESTAMP,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Indexes for income_sources
CREATE INDEX IF NOT EXISTS idx_income_user ON income_sources(user_id);
CREATE INDEX IF NOT EXISTS idx_income_category ON income_sources(category);
CREATE INDEX IF NOT EXISTS idx_income_active ON income_sources(is_active);
CREATE INDEX IF NOT EXISTS idx_income_deleted_at ON income_sources(deleted_at);

-- ================================================
-- INVESTMENTS TABLE
-- ================================================
CREATE TABLE IF NOT EXISTS investments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    investment_type VARCHAR(50) NOT NULL,
    current_value DECIMAL(15,2) NOT NULL,
    initial_investment DECIMAL(15,2) NOT NULL,
    gain_loss DECIMAL(15,2) DEFAULT 0,
    gain_loss_percentage DECIMAL(10,2) DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    purchase_date TIMESTAMP NOT NULL,
    last_updated TIMESTAMP NOT NULL,
    ticker_symbol VARCHAR(20),
    quantity INTEGER DEFAULT 1,
    current_price DECIMAL(15,2) DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Indexes for investments
CREATE INDEX IF NOT EXISTS idx_investment_user ON investments(user_id);
CREATE INDEX IF NOT EXISTS idx_investment_type ON investments(investment_type);
CREATE INDEX IF NOT EXISTS idx_investment_last_updated ON investments(last_updated);
CREATE INDEX IF NOT EXISTS idx_investment_ticker ON investments(ticker_symbol);
CREATE INDEX IF NOT EXISTS idx_investment_deleted_at ON investments(deleted_at);

-- ================================================
-- FINANCIAL GOALS TABLE
-- ================================================
CREATE TABLE IF NOT EXISTS financial_goals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    goal_type VARCHAR(50) NOT NULL,
    target_amount DECIMAL(15,2) NOT NULL,
    current_amount DECIMAL(15,2) DEFAULT 0,
    monthly_contribution DECIMAL(15,2) DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    target_date TIMESTAMP NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'in_progress',
    percentage_complete DECIMAL(5,2) DEFAULT 0,
    months_remaining INTEGER DEFAULT 0,
    icon VARCHAR(50),
    color VARCHAR(20),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Indexes for financial_goals
CREATE INDEX IF NOT EXISTS idx_goal_user ON financial_goals(user_id);
CREATE INDEX IF NOT EXISTS idx_goal_type ON financial_goals(goal_type);
CREATE INDEX IF NOT EXISTS idx_goal_target_date ON financial_goals(target_date);
CREATE INDEX IF NOT EXISTS idx_goal_status ON financial_goals(status);
CREATE INDEX IF NOT EXISTS idx_goal_deleted_at ON financial_goals(deleted_at);

-- ================================================
-- SAVINGS GOALS TABLE
-- ================================================
CREATE TABLE IF NOT EXISTS savings_goals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    target_amount DECIMAL(15,2) NOT NULL,
    current_amount DECIMAL(15,2) DEFAULT 0,
    percentage_complete DECIMAL(5,2) DEFAULT 0,
    target_date TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Indexes for savings_goals
CREATE UNIQUE INDEX IF NOT EXISTS idx_savings_goal_user ON savings_goals(user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_savings_goal_deleted_at ON savings_goals(deleted_at);

-- ================================================
-- RECURRING BILLS TABLE
-- ================================================
CREATE TABLE IF NOT EXISTS recurring_bills (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    amount DECIMAL(15,2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    category VARCHAR(50) NOT NULL,
    recurrence_pattern VARCHAR(20) NOT NULL,
    next_due_date TIMESTAMP NOT NULL,
    last_paid_date TIMESTAMP,
    status VARCHAR(30) NOT NULL DEFAULT 'upcoming',
    days_until_due INTEGER DEFAULT 0,
    merchant VARCHAR(200),
    icon VARCHAR(50),
    auto_pay_enabled BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- Indexes for recurring_bills
CREATE INDEX IF NOT EXISTS idx_bill_user ON recurring_bills(user_id);
CREATE INDEX IF NOT EXISTS idx_bill_due_date ON recurring_bills(next_due_date);
CREATE INDEX IF NOT EXISTS idx_bill_status ON recurring_bills(status);
CREATE INDEX IF NOT EXISTS idx_bill_deleted_at ON recurring_bills(deleted_at);

-- ================================================
-- TRIGGERS FOR UPDATED_AT
-- ================================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Triggers for income_sources
DROP TRIGGER IF EXISTS update_income_sources_updated_at ON income_sources;
CREATE TRIGGER update_income_sources_updated_at
    BEFORE UPDATE ON income_sources
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Triggers for investments
DROP TRIGGER IF EXISTS update_investments_updated_at ON investments;
CREATE TRIGGER update_investments_updated_at
    BEFORE UPDATE ON investments
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Triggers for financial_goals
DROP TRIGGER IF EXISTS update_financial_goals_updated_at ON financial_goals;
CREATE TRIGGER update_financial_goals_updated_at
    BEFORE UPDATE ON financial_goals
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Triggers for savings_goals
DROP TRIGGER IF EXISTS update_savings_goals_updated_at ON savings_goals;
CREATE TRIGGER update_savings_goals_updated_at
    BEFORE UPDATE ON savings_goals
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Triggers for recurring_bills
DROP TRIGGER IF EXISTS update_recurring_bills_updated_at ON recurring_bills;
CREATE TRIGGER update_recurring_bills_updated_at
    BEFORE UPDATE ON recurring_bills
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ================================================
-- COMMENTS
-- ================================================

COMMENT ON TABLE income_sources IS 'Stores user income sources and their details';
COMMENT ON TABLE investments IS 'Stores user investment portfolio items';
COMMENT ON TABLE financial_goals IS 'Stores user financial goals with progress tracking';
COMMENT ON TABLE savings_goals IS 'Stores the primary savings goal for each user (one per user)';
COMMENT ON TABLE recurring_bills IS 'Stores recurring bills and upcoming payments';

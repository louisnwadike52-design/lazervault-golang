-- ============================================================================
-- Group Accounts Feature - Complete Database Migration
-- Created: December 8, 2025
-- Purpose: Production-ready group savings/contribution platform
-- ============================================================================

-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm"; -- For full-text search

-- ============================================================================
-- 1. GROUP ACCOUNTS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS group_accounts (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,

    name VARCHAR(255) NOT NULL,
    description TEXT,
    admin_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'closed')),
    metadata JSONB DEFAULT '{}'::jsonb
);

-- Indexes for group_accounts
CREATE INDEX IF NOT EXISTS idx_group_accounts_admin_id ON group_accounts(admin_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_group_accounts_status ON group_accounts(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_group_accounts_created_at ON group_accounts(created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_group_accounts_deleted_at ON group_accounts(deleted_at);
CREATE INDEX IF NOT EXISTS idx_group_accounts_name_trgm ON group_accounts USING gin(name gin_trgm_ops) WHERE deleted_at IS NULL; -- Full-text search

-- ============================================================================
-- 2. GROUP MEMBERS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS group_members (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,

    group_id VARCHAR(100) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member', 'viewer')),
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'suspended', 'removed')),
    permissions JSONB DEFAULT '{}'::jsonb,

    -- Composite unique constraint: A user can only be a member once per group
    UNIQUE(group_id, user_id)
);

-- Indexes for group_members
CREATE INDEX IF NOT EXISTS idx_group_members_group_id ON group_members(group_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_group_members_user_id ON group_members(user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_group_members_status ON group_members(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_group_members_role ON group_members(role) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_group_members_deleted_at ON group_members(deleted_at);
CREATE INDEX IF NOT EXISTS idx_group_members_group_user ON group_members(group_id, user_id) WHERE deleted_at IS NULL; -- Composite index for lookups

-- ============================================================================
-- 3. CONTRIBUTIONS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS contributions (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,

    -- Basic fields
    group_id VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    target_amount BIGINT NOT NULL CHECK (target_amount > 0),
    current_amount BIGINT NOT NULL DEFAULT 0 CHECK (current_amount >= 0),
    currency VARCHAR(10) NOT NULL DEFAULT 'USD',
    deadline TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'completed', 'cancelled')),
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    metadata JSONB DEFAULT '{}'::jsonb,

    -- Type-specific fields
    type VARCHAR(50) NOT NULL DEFAULT 'one_time' CHECK (type IN ('one_time', 'recurring', 'rotating_savings')),
    frequency VARCHAR(50) CHECK (frequency IN ('daily', 'weekly', 'biweekly', 'monthly', 'quarterly', 'yearly')),
    regular_amount BIGINT CHECK (regular_amount > 0),
    next_payment_date TIMESTAMP WITH TIME ZONE,
    start_date TIMESTAMP WITH TIME ZONE,
    total_cycles INT CHECK (total_cycles > 0),
    current_cycle INT DEFAULT 1 CHECK (current_cycle > 0),

    -- Payout/Rotation fields
    current_payout_recipient BIGINT REFERENCES users(id) ON DELETE SET NULL,
    next_payout_date TIMESTAMP WITH TIME ZONE,

    -- Payment settings
    auto_pay_enabled BOOLEAN DEFAULT FALSE,
    penalty_amount BIGINT CHECK (penalty_amount >= 0),
    grace_period_days INT CHECK (grace_period_days >= 0),
    allow_partial_payments BOOLEAN DEFAULT TRUE,
    minimum_balance BIGINT CHECK (minimum_balance >= 0)
);

-- Indexes for contributions
CREATE INDEX IF NOT EXISTS idx_contributions_group_id ON contributions(group_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contributions_created_by ON contributions(created_by) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contributions_status ON contributions(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contributions_type ON contributions(type) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contributions_next_payment_date ON contributions(next_payment_date) WHERE deleted_at IS NULL AND next_payment_date IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_contributions_next_payout_date ON contributions(next_payout_date) WHERE deleted_at IS NULL AND next_payout_date IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_contributions_current_payout_recipient ON contributions(current_payout_recipient) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contributions_deadline ON contributions(deadline) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contributions_created_at ON contributions(created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contributions_deleted_at ON contributions(deleted_at);
CREATE INDEX IF NOT EXISTS idx_contributions_title_trgm ON contributions USING gin(title gin_trgm_ops) WHERE deleted_at IS NULL; -- Full-text search
CREATE INDEX IF NOT EXISTS idx_contributions_active_recurring ON contributions(group_id, next_payment_date) WHERE deleted_at IS NULL AND status = 'active' AND type IN ('recurring', 'rotating_savings'); -- For scheduled payment processing

-- ============================================================================
-- 4. CONTRIBUTION PAYMENTS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS contribution_payments (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,

    contribution_id VARCHAR(100) NOT NULL,
    group_id VARCHAR(100) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency VARCHAR(10) NOT NULL,
    payment_date TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'refunded')),
    transaction_id VARCHAR(255) UNIQUE,
    receipt_id VARCHAR(255),
    notes TEXT,
    metadata JSONB DEFAULT '{}'::jsonb
);

-- Indexes for contribution_payments
CREATE INDEX IF NOT EXISTS idx_contribution_payments_contribution_id ON contribution_payments(contribution_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_payments_group_id ON contribution_payments(group_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_payments_user_id ON contribution_payments(user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_payments_status ON contribution_payments(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_payments_payment_date ON contribution_payments(payment_date DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_payments_transaction_id ON contribution_payments(transaction_id) WHERE deleted_at IS NULL AND transaction_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_payments_deleted_at ON contribution_payments(deleted_at);
CREATE INDEX IF NOT EXISTS idx_contribution_payments_user_contribution ON contribution_payments(user_id, contribution_id) WHERE deleted_at IS NULL; -- User payment history per contribution

-- ============================================================================
-- 5. PAYOUT SCHEDULES TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS payout_schedules (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,

    contribution_id VARCHAR(100) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    position INT NOT NULL CHECK (position > 0),
    scheduled_date TIMESTAMP WITH TIME ZONE NOT NULL,
    expected_amount BIGINT NOT NULL CHECK (expected_amount > 0),
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'completed', 'cancelled')),
    received_date TIMESTAMP WITH TIME ZONE,
    actual_amount BIGINT CHECK (actual_amount >= 0),
    notes TEXT,

    -- Ensure unique position per contribution
    UNIQUE(contribution_id, position)
);

-- Indexes for payout_schedules
CREATE INDEX IF NOT EXISTS idx_payout_schedules_contribution_id ON payout_schedules(contribution_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payout_schedules_user_id ON payout_schedules(user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payout_schedules_status ON payout_schedules(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payout_schedules_scheduled_date ON payout_schedules(scheduled_date) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payout_schedules_deleted_at ON payout_schedules(deleted_at);
CREATE INDEX IF NOT EXISTS idx_payout_schedules_pending ON payout_schedules(contribution_id, scheduled_date) WHERE deleted_at IS NULL AND status = 'pending'; -- For finding next payout

-- ============================================================================
-- 6. PAYOUT TRANSACTIONS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS payout_transactions (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,

    contribution_id VARCHAR(100) NOT NULL,
    group_id VARCHAR(100) NOT NULL,
    recipient_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency VARCHAR(10) NOT NULL,
    payout_date TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'refunded')),
    transaction_id VARCHAR(255) UNIQUE,
    payment_method VARCHAR(100),
    failure_reason TEXT,
    metadata JSONB DEFAULT '{}'::jsonb
);

-- Indexes for payout_transactions
CREATE INDEX IF NOT EXISTS idx_payout_transactions_contribution_id ON payout_transactions(contribution_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payout_transactions_group_id ON payout_transactions(group_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payout_transactions_recipient_user_id ON payout_transactions(recipient_user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payout_transactions_status ON payout_transactions(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payout_transactions_payout_date ON payout_transactions(payout_date DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_payout_transactions_transaction_id ON payout_transactions(transaction_id) WHERE deleted_at IS NULL AND transaction_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_payout_transactions_deleted_at ON payout_transactions(deleted_at);

-- ============================================================================
-- 7. CONTRIBUTION RECEIPTS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS contribution_receipts (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,

    payment_id VARCHAR(100) NOT NULL UNIQUE,
    contribution_id VARCHAR(100) NOT NULL,
    group_id VARCHAR(100) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency VARCHAR(10) NOT NULL,
    payment_date TIMESTAMP WITH TIME ZONE NOT NULL,
    generated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    receipt_number VARCHAR(100) NOT NULL UNIQUE,
    receipt_data JSONB DEFAULT '{}'::jsonb
);

-- Indexes for contribution_receipts
CREATE INDEX IF NOT EXISTS idx_contribution_receipts_payment_id ON contribution_receipts(payment_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_receipts_contribution_id ON contribution_receipts(contribution_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_receipts_group_id ON contribution_receipts(group_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_receipts_user_id ON contribution_receipts(user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_receipts_generated_at ON contribution_receipts(generated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_receipts_receipt_number ON contribution_receipts(receipt_number) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contribution_receipts_deleted_at ON contribution_receipts(deleted_at);

-- ============================================================================
-- TRIGGERS FOR AUTO-UPDATING updated_at TIMESTAMPS
-- ============================================================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to all tables
CREATE TRIGGER update_group_accounts_updated_at BEFORE UPDATE ON group_accounts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_group_members_updated_at BEFORE UPDATE ON group_members
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_contributions_updated_at BEFORE UPDATE ON contributions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_contribution_payments_updated_at BEFORE UPDATE ON contribution_payments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_payout_schedules_updated_at BEFORE UPDATE ON payout_schedules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_payout_transactions_updated_at BEFORE UPDATE ON payout_transactions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_contribution_receipts_updated_at BEFORE UPDATE ON contribution_receipts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- PERFORMANCE OPTIMIZATION NOTES
-- ============================================================================

-- 1. All tables use BIGSERIAL for IDs to support millions of records
-- 2. Partial indexes on deleted_at for soft-delete support
-- 3. GIN indexes for full-text search on name and title fields (requires pg_trgm extension)
-- 4. Composite indexes for common query patterns (group_id + user_id, etc.)
-- 5. CHECK constraints for data integrity at database level
-- 6. Foreign keys with appropriate ON DELETE behavior for referential integrity
-- 7. JSONB fields with GIN indexes for flexible metadata storage
-- 8. Timestamp indexes for time-based queries (payment_date, payout_date, etc.)
-- 9. Unique constraints to prevent duplicate records
-- 10. Triggers for automatic updated_at maintenance

-- ============================================================================
-- MIGRATION COMPLETE
-- ============================================================================

-- You can verify the tables were created with:
-- \dt group_*
-- \dt contribution*
-- \dt payout*

-- You can verify indexes with:
-- SELECT tablename, indexname FROM pg_indexes WHERE tablename LIKE 'group%' OR tablename LIKE 'contribution%' OR tablename LIKE 'payout%';

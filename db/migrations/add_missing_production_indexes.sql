-- Additional Performance Indexes for LazerVault Production Database
-- Created: 2025-12-08
-- Purpose: Add missing high-value indexes for production scale

-- ============================================================================
-- USERS TABLE - Additional Indexes
-- ============================================================================

-- Composite index for verified users with recent activity
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_verified_created
ON users(verified, created_at DESC);

-- Index on role for admin queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_role
ON users(role) WHERE role IS NOT NULL;

-- ============================================================================
-- SESSIONS TABLE - Additional Indexes
-- ============================================================================

-- Composite index for active sessions (not blocked)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_active
ON sessions(user_id, expires_at) WHERE is_blocked = false;

-- Index for session cleanup (blocked sessions)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_cleanup
ON sessions(created_at) WHERE is_blocked = true;

-- ============================================================================
-- TRANSFERS TABLE - Additional Indexes
-- ============================================================================

-- Index on from_user_id for user's sent transfers
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_from_user_id
ON transfers(from_user_id, created_at DESC);

-- Index on to_user_id for user's received transfers
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_to_user_id
ON transfers(to_user_id, created_at DESC);

-- Index on reference for lookup by reference
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_reference
ON transfers(reference) WHERE reference IS NOT NULL;

-- Composite index for user transfer history (both sent and received)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_user_history
ON transfers(from_user_id, to_user_id, created_at DESC);

-- ============================================================================
-- ACCOUNTS TABLE - Additional Indexes
-- ============================================================================

-- Composite index on owner, active status, and creation date
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_owner_active
ON accounts(owner_user_id, is_active, created_at DESC);

-- Index on status for account status queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_status
ON accounts(status);

-- Index on currency for multi-currency queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_currency
ON accounts(currency);

-- Composite index for active accounts only
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_active_only
ON accounts(owner_user_id, created_at DESC) WHERE is_active = true;

-- ============================================================================
-- RECIPIENTS TABLE - Additional Indexes
-- ============================================================================

-- Composite index for favorites and recents
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recipients_favorites
ON recipients(owner_user_id, is_favorite DESC, created_at DESC);

-- Full-text search on recipient name
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recipients_name_search
ON recipients USING GIN (to_tsvector('english', name));

-- ============================================================================
-- EXCHANGE_TRANSACTIONS TABLE - Additional Indexes
-- ============================================================================

-- Composite index for user exchange history
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_exchange_user_status
ON exchange_transactions(user_id, status, created_at DESC);

-- Index on currency pairs for rate lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_exchange_currencies
ON exchange_transactions(from_currency, to_currency);

-- ============================================================================
-- AI_CHAT_HISTORIES TABLE - Additional Indexes
-- ============================================================================

-- Composite index for user chat history with pagination
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ai_chats_user_created
ON ai_chat_histories(user_id, created_at DESC);

-- ============================================================================
-- DEPOSITS TABLE - Additional Indexes
-- ============================================================================

-- Composite index for user deposit history
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_deposits_user_status_created
ON deposits(user_id, status, created_at DESC);

-- Index on currency for multi-currency deposits
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_deposits_currency
ON deposits(currency);

-- ============================================================================
-- WITHDRAWALS TABLE - Additional Indexes
-- ============================================================================

-- Composite index for user withdrawal history
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_withdrawals_user_status_created
ON withdrawals(user_id, status, created_at DESC);

-- Index on currency for multi-currency withdrawals
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_withdrawals_currency
ON withdrawals(currency);

-- ============================================================================
-- PERFORMANCE VERIFICATION QUERIES
-- ============================================================================

-- Check index usage statistics:
-- SELECT schemaname, tablename, indexname, idx_scan, idx_tup_read, idx_tup_fetch
-- FROM pg_stat_user_indexes
-- ORDER BY idx_scan DESC;

-- Check index sizes:
-- SELECT schemaname, tablename, indexname,
--        pg_size_pretty(pg_relation_size(indexrelid)) as index_size
-- FROM pg_stat_user_indexes
-- ORDER BY pg_relation_size(indexrelid) DESC;

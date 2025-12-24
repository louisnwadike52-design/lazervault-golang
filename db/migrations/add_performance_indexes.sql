-- Performance Indexes for LazerVault Production Database
-- Created: 2025-12-08
-- Purpose: Add indexes for high-traffic tables to support millions of users

-- ============================================================================
-- USERS TABLE INDEXES
-- ============================================================================

-- Index on email for login lookups (most common query)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email ON users(email);

-- Index on phone for SMS/phone-based operations
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_phone ON users(phone_number);

-- Composite index for active users lookup
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_active_created ON users(is_active, created_at DESC);

-- Index on verification status
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_verification ON users(email_verified, phone_verified);

-- ============================================================================
-- SESSIONS TABLE INDEXES
-- ============================================================================

-- Index on user_id for session lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);

-- Index on access_token for authentication
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_access_token ON sessions(access_token);

-- Composite index for active sessions
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_active ON sessions(user_id, expires_at) WHERE is_active = true;

-- Index on expires_at for cleanup jobs
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- ============================================================================
-- TRANSFERS TABLE INDEXES
-- ============================================================================

-- Index on sender_id for user's sent transfers
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_sender_id ON transfers(sender_id, created_at DESC);

-- Index on recipient_id for user's received transfers
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_recipient_id ON transfers(recipient_id, created_at DESC);

-- Index on status for transfer processing
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_status ON transfers(status, created_at DESC);

-- Composite index for pending transfers check
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_pending ON transfers(status, scheduled_at) WHERE status = 'pending';

-- Index on transaction_ref for lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_transaction_ref ON transfers(transaction_ref);

-- ============================================================================
-- BATCH TRANSFERS TABLE INDEXES
-- ============================================================================

-- Index on user_id for batch transfer history
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_transfers_user_id ON batch_transfers(user_id, created_at DESC);

-- Index on status
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_transfers_status ON batch_transfers(status);

-- Index on batch_id for grouping
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_transfers_batch_id ON batch_transfers(batch_id);

-- ============================================================================
-- INVOICES TABLE INDEXES
-- ============================================================================

-- Index on user_id (invoice creator)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_invoices_user_id ON invoices(user_id, created_at DESC);

-- Index on recipient_id (invoice recipient)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_invoices_recipient_id ON invoices(recipient_id, created_at DESC);

-- Index on status
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_invoices_status ON invoices(is_paid, created_at DESC);

-- Composite index for unpaid invoices
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_invoices_unpaid ON invoices(recipient_id, is_paid, due_date) WHERE is_paid = false;

-- Index on due_date for overdue checks
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_invoices_due_date ON invoices(due_date);

-- ============================================================================
-- DEPOSITS TABLE INDEXES
-- ============================================================================

-- Index on user_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_deposits_user_id ON deposits(user_id, created_at DESC);

-- Index on status
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_deposits_status ON deposits(status);

-- Index on reference
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_deposits_reference ON deposits(reference);

-- ============================================================================
-- WITHDRAWALS TABLE INDEXES
-- ============================================================================

-- Index on user_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_withdrawals_user_id ON withdrawals(user_id, created_at DESC);

-- Index on status for processing
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_withdrawals_status ON withdrawals(status);

-- Index on reference
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_withdrawals_reference ON withdrawals(reference);

-- ============================================================================
-- RECIPIENTS TABLE INDEXES
-- ============================================================================

-- Index on user_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recipients_user_id ON recipients(user_id);

-- Composite index for active recipients
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recipients_active ON recipients(user_id, is_favorite DESC, created_at DESC);

-- ============================================================================
-- TRANSACTIONS TABLE (Generic) INDEXES
-- ============================================================================

-- Index on user_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_user_id ON transactions(user_id, created_at DESC);

-- Index on transaction_type
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_type ON transactions(transaction_type, created_at DESC);

-- Index on status
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_status ON transactions(status);

-- Index on reference for lookups
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_reference ON transactions(reference);

-- Composite index for user transaction history
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_user_history ON transactions(user_id, transaction_type, created_at DESC);

-- ============================================================================
-- ACCOUNT_CARDS TABLE INDEXES
-- ============================================================================

-- Index on user_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_cards_user_id ON account_cards(user_id);

-- Index on card_number (partial, last 4 digits for display)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_cards_number ON account_cards(card_number);

-- Index on status
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_cards_status ON account_cards(status);

-- ============================================================================
-- NOTIFICATIONS TABLE INDEXES
-- ============================================================================

-- Index on user_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_user_id ON notifications(user_id, created_at DESC);

-- Index on read status
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_read ON notifications(user_id, is_read, created_at DESC);

-- Index on type
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_type ON notifications(notification_type);

-- ============================================================================
-- AUDIT_LOGS TABLE INDEXES
-- ============================================================================

-- Index on user_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id, created_at DESC);

-- Index on action
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_logs_action ON audit_logs(action, created_at DESC);

-- Index on ip_address for security monitoring
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_logs_ip ON audit_logs(ip_address, created_at DESC);

-- ============================================================================
-- AI_CHAT_SESSIONS TABLE INDEXES
-- ============================================================================

-- Index on user_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ai_chats_user_id ON ai_chat_sessions(user_id, created_at DESC);

-- Index on session_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ai_chats_session_id ON ai_chat_sessions(session_id);

-- ============================================================================
-- CONTACT_SYNC TABLE INDEXES
-- ============================================================================

-- Index on user_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_contacts_user_id ON synced_contacts(user_id);

-- Index on phone_numbers (using GIN for array)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_contacts_phones ON synced_contacts USING GIN (phone_numbers);

-- ============================================================================
-- STATISTICS & ANALYTICS INDEXES
-- ============================================================================

-- Composite indexes for common analytics queries
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_analytics ON transfers(created_at, status, amount);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_analytics ON transactions(created_at, transaction_type, amount);

-- ============================================================================
-- CLEANUP OLD RECORDS INDEXES
-- ============================================================================

-- Indexes to support efficient cleanup of old data
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_cleanup ON sessions(created_at) WHERE is_active = false;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_cleanup ON notifications(created_at) WHERE is_read = true;

-- ============================================================================
-- PARTIAL INDEXES FOR SPECIFIC USE CASES
-- ============================================================================

-- Active users only
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_active_only ON users(id, created_at DESC) WHERE is_active = true;

-- Pending transfers for scheduled jobs
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transfers_scheduled ON transfers(scheduled_at) WHERE status = 'scheduled';

-- Unread notifications
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_unread ON notifications(user_id, created_at DESC) WHERE is_read = false;

-- ============================================================================
-- FULL TEXT SEARCH INDEXES (PostgreSQL)
-- ============================================================================

-- Full-text search on invoices
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_invoices_fulltext ON invoices USING GIN (to_tsvector('english', title || ' ' || description));

-- Full-text search on recipients
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_recipients_fulltext ON recipients USING GIN (to_tsvector('english', name || ' ' || email));

-- ============================================================================
-- VERIFY INDEXES
-- ============================================================================

-- Query to check index usage statistics:
-- SELECT schemaname, tablename, indexname, idx_scan, idx_tup_read, idx_tup_fetch
-- FROM pg_stat_user_indexes
-- ORDER BY idx_scan DESC;

-- Query to find missing indexes:
-- SELECT schemaname, tablename, attname, n_distinct, correlation
-- FROM pg_stats
-- WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
-- ORDER BY abs(correlation) DESC;

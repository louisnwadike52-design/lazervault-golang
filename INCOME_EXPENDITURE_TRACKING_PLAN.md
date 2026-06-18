# Income & Expenditure Tracking System - Implementation Plan

**Date:** December 28, 2025
**Objective:** Create comprehensive automatic tracking of ALL financial transactions across ALL services for the Statistics screen.

---

## Problem Statement

The current statistics screen relies on manually entered expenses and income sources. It doesn't automatically track transactions from various services like transfers, invoice payments, deposits, withdrawals, exchanges, etc.

**User Requirement:**
> For every service that involves money (income or expenditure), create dedicated tracking tables that are updated automatically in the background (non-blocking) when financial transactions occur.

---

## Solution Architecture

### 1. New Database Models

Create two new models for automatic transaction tracking:

#### **IncomeTransaction Model**
Automatically tracks ALL income operations across the platform:
- Deposits
- Invoice payments received
- Tagged invoice payments received
- Transfers received
- Exchange proceeds (when applicable)
- Any other income-generating operations

#### **ExpenditureTransaction Model**
Automatically tracks ALL expenditure operations across the platform:
- Withdrawals
- Transfers sent
- Invoice payments made
- Bill payments
- Exchange costs
- Tagged invoice payments made
- Any other money-spending operations

### 2. Service Mapping

| Service | Operation | Transaction Type | Model Field |
|---------|-----------|------------------|-------------|
| **transfer_service.go** | InitiateTransfer (sender) | EXPENDITURE | transfer_id, amount, recipient |
| **transfer_service.go** | InitiateTransfer (receiver) | INCOME | transfer_id, amount, sender |
| **deposit_service.go** | InitiateDeposit | INCOME | deposit_id, amount, source |
| **withdrawal_service.go** | InitiateWithdrawal | EXPENDITURE | withdrawal_id, amount, destination |
| **invoice_payment_service.go** | ProcessPayment (payer) | EXPENDITURE | invoice_id, amount, recipient |
| **invoice_payment_service.go** | ProcessPayment (payee) | INCOME | invoice_id, amount, payer |
| **tagged_invoice_service.go** | ProcessPayment (payer) | EXPENDITURE | invoice_id, amount, recipient |
| **tagged_invoice_service.go** | ProcessPayment (payee) | INCOME | invoice_id, amount, payer |
| **exchange_service.go** | InitiateExchange | EXPENDITURE | exchange_id, from_amount, from_currency |
| **bill_payment_provider.go** | PayBill | EXPENDITURE | bill_id, amount, merchant |

### 3. Background Task Implementation Pattern

For each service operation, add a **non-blocking background task** that:
1. Does NOT block the main transaction
2. Logs the transaction to income_transactions or expenditure_transactions table
3. Runs asynchronously using goroutines or task queues
4. Handles failures gracefully (retries, logging)

**Pattern Example:**
```go
// Main transaction (existing code)
err := s.db.Transaction(func(tx *gorm.DB) error {
    // ... existing transaction logic ...
    return nil
})

// Background tracking (NEW - non-blocking)
go func() {
    _ = s.trackExpenditure(ctx, userID, amount, "transfer", transferID)
}()
```

### 4. Statistics API Integration

Update StatisticsService to query both:
- **User-entered data:** expenses, budgets, income_sources (existing)
- **Auto-tracked data:** income_transactions, expenditure_transactions (new)

Provide aggregated views:
- Total income (user-entered + auto-tracked)
- Total expenditure (user-entered + auto-tracked)
- Breakdown by source/category
- Trends over time

---

## Implementation Steps

### Phase 1: Database Models & Migration
1. Create `IncomeTransaction` model in `models/statistics.go`
2. Create `ExpenditureTransaction` model in `models/statistics.go`
3. Create database migration
4. Add indexes for performance

### Phase 2: Proto Definitions
1. Update `proto/statistics.proto` with new messages
2. Add RPC methods for querying tracked transactions
3. Generate proto files

### Phase 3: Background Tracking Service
1. Create `TransactionTracker` service for centralized tracking
2. Implement `TrackIncome()` method
3. Implement `TrackExpenditure()` method
4. Add retry logic and error handling

### Phase 4: Service Integration
1. Update each service (8 services total) to call tracker
2. Add background goroutines for non-blocking execution
3. Test each service integration

### Phase 5: Statistics Service Updates
1. Add methods to query income_transactions
2. Add methods to query expenditure_transactions
3. Update analytics to include auto-tracked data
4. Update category breakdown logic

### Phase 6: Frontend Integration
1. Update `statistics_repository.dart` with new endpoints
2. Update `statistics_cubit.dart` to fetch tracked transactions
3. Update `statistics_screen.dart` to display comprehensive data
4. Remove any placeholders

### Phase 7: Testing
1. Unit tests for tracking service
2. Integration tests for each service
3. End-to-end test for statistics screen
4. Performance testing

---

## Database Schema

### income_transactions

```sql
CREATE TABLE income_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id BIGINT NOT NULL REFERENCES users(id),
    amount DECIMAL(20, 2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    source_type VARCHAR(50) NOT NULL, -- 'deposit', 'transfer_received', 'invoice_payment', etc.
    source_id VARCHAR(255), -- ID of the source transaction
    source_reference VARCHAR(500), -- Additional reference (e.g., invoice number)
    category VARCHAR(50), -- INCOME_CATEGORY enum
    description TEXT,
    transaction_date TIMESTAMP NOT NULL DEFAULT NOW(),
    metadata JSONB, -- Additional flexible data
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,

    INDEX idx_income_user (user_id),
    INDEX idx_income_date (transaction_date),
    INDEX idx_income_source_type (source_type),
    INDEX idx_income_category (category)
);
```

### expenditure_transactions

```sql
CREATE TABLE expenditure_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id BIGINT NOT NULL REFERENCES users(id),
    amount DECIMAL(20, 2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    expense_type VARCHAR(50) NOT NULL, -- 'withdrawal', 'transfer_sent', 'invoice_payment', 'exchange', 'bill_payment', etc.
    expense_id VARCHAR(255), -- ID of the expense transaction
    expense_reference VARCHAR(500), -- Additional reference
    category VARCHAR(50), -- EXPENSE_CATEGORY enum
    recipient_id BIGINT, -- User ID of recipient (if applicable)
    recipient_name VARCHAR(255),
    merchant VARCHAR(255),
    description TEXT,
    transaction_date TIMESTAMP NOT NULL DEFAULT NOW(),
    metadata JSONB, -- Additional flexible data
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,

    INDEX idx_expenditure_user (user_id),
    INDEX idx_expenditure_date (transaction_date),
    INDEX idx_expenditure_type (expense_type),
    INDEX idx_expenditure_category (category),
    INDEX idx_expenditure_recipient (recipient_id)
);
```

---

## Benefits

1. **Comprehensive Tracking:** Every financial operation is automatically tracked
2. **Real-time Insights:** Statistics screen shows accurate, up-to-date data
3. **No Manual Entry:** Users don't need to manually log transactions
4. **Non-blocking:** Main operations aren't slowed down by tracking
5. **Flexible:** Metadata field allows for service-specific data
6. **Auditable:** Complete transaction history for compliance
7. **Analytics-Ready:** Easy to query for reports and insights

---

## Next Steps

1. Start with Phase 1: Create models and migration
2. Implement tracking service (Phase 3)
3. Integrate one service as proof-of-concept
4. Roll out to all services
5. Update statistics screen

---

**Status:** Ready for implementation
**Est. Completion:** 2-3 hours for complete implementation

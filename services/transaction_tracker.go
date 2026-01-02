package services

import (
	"context"
	"encoding/json"
	"fmt"
	"lazervaultGo/models"
	"log"
	"time"

	"gorm.io/gorm"
)

// TransactionTracker handles automatic tracking of all income and expenditure transactions
// across the platform. This runs in the background and doesn't block main operations.
type TransactionTracker struct {
	db *gorm.DB
}

// NewTransactionTracker creates a new transaction tracker instance
func NewTransactionTracker(db *gorm.DB) *TransactionTracker {
	return &TransactionTracker{db: db}
}

// IncomeTrackingParams contains parameters for tracking an income transaction
type IncomeTrackingParams struct {
	UserID           uint
	Amount           float64
	Currency         string
	SourceType       string // "deposit", "transfer_received", "invoice_payment_received", etc.
	SourceID         string
	SourceReference  string
	Category         string // Maps to IncomeCategory enum
	Description      string
	SenderID         *uint
	SenderName       string
	TransactionDate  *time.Time
	Metadata         map[string]interface{} // Optional flexible data
}

// ExpenditureTrackingParams contains parameters for tracking an expenditure transaction
type ExpenditureTrackingParams struct {
	UserID           uint
	Amount           float64
	Currency         string
	ExpenseType      string // "withdrawal", "transfer_sent", "invoice_payment_made", "bill_payment", etc.
	ExpenseID        string
	ExpenseReference string
	Category         string // Maps to ExpenseCategory enum
	Description      string
	RecipientID      *uint
	RecipientName    string
	Merchant         string
	TransactionDate  *time.Time
	Metadata         map[string]interface{} // Optional flexible data
}

// TrackIncome records an income transaction in the background (non-blocking)
// This method should be called with `go tracker.TrackIncome(...)` to run asynchronously
func (t *TransactionTracker) TrackIncome(ctx context.Context, params IncomeTrackingParams) error {
	// Set default transaction date if not provided
	transactionDate := time.Now()
	if params.TransactionDate != nil {
		transactionDate = *params.TransactionDate
	}

	// Set default currency
	if params.Currency == "" {
		params.Currency = "USD"
	}

	// Set default category if not provided
	if params.Category == "" {
		params.Category = "INCOME_CATEGORY_OTHER"
	}

	// Convert metadata to JSON string
	metadataJSON := ""
	if params.Metadata != nil {
		jsonBytes, err := json.Marshal(params.Metadata)
		if err != nil {
			log.Printf("[TransactionTracker] Failed to marshal metadata for income tracking: %v", err)
		} else {
			metadataJSON = string(jsonBytes)
		}
	}

	// Create income transaction record
	incomeTransaction := &models.IncomeTransaction{
		UserID:          params.UserID,
		Amount:          params.Amount,
		Currency:        params.Currency,
		SourceType:      params.SourceType,
		SourceID:        params.SourceID,
		SourceReference: params.SourceReference,
		Category:        params.Category,
		Description:     params.Description,
		SenderID:        params.SenderID,
		SenderName:      params.SenderName,
		TransactionDate: transactionDate,
		Metadata:        metadataJSON,
	}

	// Save to database (with retry logic)
	maxRetries := 3
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = t.db.Create(incomeTransaction).Error
		if err == nil {
			log.Printf("[TransactionTracker] ✅ Successfully tracked income: user_id=%d, amount=%.2f %s, source=%s",
				params.UserID, params.Amount, params.Currency, params.SourceType)
			return nil
		}
		log.Printf("[TransactionTracker] ⚠️  Attempt %d/%d failed to track income: %v", attempt+1, maxRetries, err)
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond) // Exponential backoff
	}

	// Log final error but don't fail the main operation
	log.Printf("[TransactionTracker] ❌ Failed to track income after %d attempts: user_id=%d, error=%v",
		maxRetries, params.UserID, err)
	return fmt.Errorf("failed to track income: %w", err)
}

// TrackExpenditure records an expenditure transaction in the background (non-blocking)
// This method should be called with `go tracker.TrackExpenditure(...)` to run asynchronously
func (t *TransactionTracker) TrackExpenditure(ctx context.Context, params ExpenditureTrackingParams) error {
	// Set default transaction date if not provided
	transactionDate := time.Now()
	if params.TransactionDate != nil {
		transactionDate = *params.TransactionDate
	}

	// Set default currency
	if params.Currency == "" {
		params.Currency = "USD"
	}

	// Set default category if not provided
	if params.Category == "" {
		params.Category = "EXPENSE_CATEGORY_OTHER"
	}

	// Convert metadata to JSON string
	metadataJSON := ""
	if params.Metadata != nil {
		jsonBytes, err := json.Marshal(params.Metadata)
		if err != nil {
			log.Printf("[TransactionTracker] Failed to marshal metadata for expenditure tracking: %v", err)
		} else {
			metadataJSON = string(jsonBytes)
		}
	}

	// Create expenditure transaction record
	expenditureTransaction := &models.ExpenditureTransaction{
		UserID:           params.UserID,
		Amount:           params.Amount,
		Currency:         params.Currency,
		ExpenseType:      params.ExpenseType,
		ExpenseID:        params.ExpenseID,
		ExpenseReference: params.ExpenseReference,
		Category:         params.Category,
		Description:      params.Description,
		RecipientID:      params.RecipientID,
		RecipientName:    params.RecipientName,
		Merchant:         params.Merchant,
		TransactionDate:  transactionDate,
		Metadata:         metadataJSON,
	}

	// Save to database (with retry logic)
	maxRetries := 3
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = t.db.Create(expenditureTransaction).Error
		if err == nil {
			log.Printf("[TransactionTracker] ✅ Successfully tracked expenditure: user_id=%d, amount=%.2f %s, type=%s",
				params.UserID, params.Amount, params.Currency, params.ExpenseType)
			return nil
		}
		log.Printf("[TransactionTracker] ⚠️  Attempt %d/%d failed to track expenditure: %v", attempt+1, maxRetries, err)
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond) // Exponential backoff
	}

	// Log final error but don't fail the main operation
	log.Printf("[TransactionTracker] ❌ Failed to track expenditure after %d attempts: user_id=%d, error=%v",
		maxRetries, params.UserID, err)
	return fmt.Errorf("failed to track expenditure: %w", err)
}

// TrackIncomeAsync tracks income in a non-blocking goroutine
// This is the recommended way to track transactions without blocking main operations
func (t *TransactionTracker) TrackIncomeAsync(ctx context.Context, params IncomeTrackingParams) {
	go func() {
		// Create a new context with timeout for background work
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := t.TrackIncome(bgCtx, params); err != nil {
			// Error already logged in TrackIncome
		}
	}()
}

// TrackExpenditureAsync tracks expenditure in a non-blocking goroutine
// This is the recommended way to track transactions without blocking main operations
func (t *TransactionTracker) TrackExpenditureAsync(ctx context.Context, params ExpenditureTrackingParams) {
	go func() {
		// Create a new context with timeout for background work
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := t.TrackExpenditure(bgCtx, params); err != nil {
			// Error already logged in TrackExpenditure
		}
	}()
}

// GetIncomeTransactions retrieves income transactions for a user
func (t *TransactionTracker) GetIncomeTransactions(ctx context.Context, userID uint, startDate, endDate time.Time, limit int) ([]models.IncomeTransaction, error) {
	var transactions []models.IncomeTransaction

	query := t.db.Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
		userID, startDate, endDate).
		Order("transaction_date DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&transactions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch income transactions: %w", err)
	}

	return transactions, nil
}

// GetExpenditureTransactions retrieves expenditure transactions for a user
func (t *TransactionTracker) GetExpenditureTransactions(ctx context.Context, userID uint, startDate, endDate time.Time, limit int) ([]models.ExpenditureTransaction, error) {
	var transactions []models.ExpenditureTransaction

	query := t.db.Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
		userID, startDate, endDate).
		Order("transaction_date DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&transactions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch expenditure transactions: %w", err)
	}

	return transactions, nil
}

// GetIncomeBreakdownBySource gets total income grouped by source type
func (t *TransactionTracker) GetIncomeBreakdownBySource(ctx context.Context, userID uint, startDate, endDate time.Time) (map[string]float64, error) {
	type Result struct {
		SourceType string
		Total      float64
	}

	var results []Result
	err := t.db.Model(&models.IncomeTransaction{}).
		Select("source_type, SUM(amount) as total").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
			userID, startDate, endDate).
		Group("source_type").
		Scan(&results).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get income breakdown: %w", err)
	}

	breakdown := make(map[string]float64)
	for _, r := range results {
		breakdown[r.SourceType] = r.Total
	}

	return breakdown, nil
}

// GetExpenditureBreakdownByType gets total expenditure grouped by expense type
func (t *TransactionTracker) GetExpenditureBreakdownByType(ctx context.Context, userID uint, startDate, endDate time.Time) (map[string]float64, error) {
	type Result struct {
		ExpenseType string
		Total       float64
	}

	var results []Result
	err := t.db.Model(&models.ExpenditureTransaction{}).
		Select("expense_type, SUM(amount) as total").
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
			userID, startDate, endDate).
		Group("expense_type").
		Scan(&results).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get expenditure breakdown: %w", err)
	}

	breakdown := make(map[string]float64)
	for _, r := range results {
		breakdown[r.ExpenseType] = r.Total
	}

	return breakdown, nil
}

// GetTotalIncome calculates total income for a period
func (t *TransactionTracker) GetTotalIncome(ctx context.Context, userID uint, startDate, endDate time.Time) (float64, error) {
	var total float64
	err := t.db.Model(&models.IncomeTransaction{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
			userID, startDate, endDate).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error

	if err != nil {
		return 0, fmt.Errorf("failed to calculate total income: %w", err)
	}

	return total, nil
}

// GetTotalExpenditure calculates total expenditure for a period
func (t *TransactionTracker) GetTotalExpenditure(ctx context.Context, userID uint, startDate, endDate time.Time) (float64, error) {
	var total float64
	err := t.db.Model(&models.ExpenditureTransaction{}).
		Where("user_id = ? AND transaction_date >= ? AND transaction_date <= ?",
			userID, startDate, endDate).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error

	if err != nil {
		return 0, fmt.Errorf("failed to calculate total expenditure: %w", err)
	}

	return total, nil
}

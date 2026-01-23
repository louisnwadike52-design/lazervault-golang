package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/tasks"
	"math/rand"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Using common errors from errors.go

// Mock rates - Replace with a real rate provider service
var mockRates = map[string]map[string]float64{
	"GBP": {"USD": 1.25, "EUR": 1.17, "JPY": 190.50},
	"USD": {"GBP": 0.80, "EUR": 0.93, "JPY": 152.00},
	"EUR": {"GBP": 0.85, "USD": 1.07, "JPY": 164.00},
}

const DefaultExchangePageSize = 20

// IExchangeService defines the interface for exchange operations
type IExchangeService interface {
	GetExchangeRate(ctx context.Context, fromCurrency, toCurrency string) (float64, error)
	InitiateInternationalTransfer(ctx context.Context, req *InitiateTransferServiceRequest) (*models.ExchangeTransaction, error)
	GetRecentExchanges(ctx context.Context, req *GetRecentExchangesServiceRequest) ([]models.ExchangeTransaction, string, error)
}

// ExchangeService implements the IExchangeService
type ExchangeService struct {
	db          *gorm.DB
	distributor tasks.TaskDistributor // Added task distributor
	txTracker   *TransactionTracker   // Tracks income/expenditure for statistics
	// Add dependencies like AccountService or a RateProvider later
}

// NewExchangeService creates a new ExchangeService
// Updated to accept distributor
func NewExchangeService(db *gorm.DB, distributor tasks.TaskDistributor) IExchangeService {
	return &ExchangeService{
		db:          db,
		distributor: distributor,
		txTracker:   NewTransactionTracker(db), // Initialize transaction tracker
	}
}

// GetExchangeRate fetches the current exchange rate (mocked)
func (s *ExchangeService) GetExchangeRate(ctx context.Context, fromCurrency, toCurrency string) (float64, error) {
	if fromCurrency == toCurrency {
		return 1.0, nil
	}

	// Basic validation
	if fromCurrency == "" || toCurrency == "" {
		return 0, ErrInvalidCurrency
	}

	// Mock fetching rate
	if rateMap, ok := mockRates[fromCurrency]; ok {
		if rate, ok := rateMap[toCurrency]; ok {
			// Simulate slight variations
			rate += (rand.Float64() - 0.5) * 0.01 // +/- 0.5%
			return rate, nil
		}
	}
	return 0, ErrRateNotFound
}

// InitiateTransferServiceRequest contains parameters for initiating a transfer
type InitiateTransferServiceRequest struct {
	UserID          uint
	FromCurrency    string
	ToCurrency      string
	AmountFrom      float64
	ReceiverDetails models.ReceiverDetails // Use the model struct directly
}

// InitiateInternationalTransfer handles validating, calculating, and recording a new transfer
func (s *ExchangeService) InitiateInternationalTransfer(ctx context.Context, req *InitiateTransferServiceRequest) (*models.ExchangeTransaction, error) {
	// Validate input
	if req.UserID == 0 {
		return nil, ErrInvalidUserID
	}
	if req.FromCurrency == "" || req.ToCurrency == "" {
		return nil, ErrInvalidCurrency
	}
	if req.AmountFrom <= 0 {
		return nil, ErrInvalidAmount
	}
	if req.ReceiverDetails.FullName == "" || req.ReceiverDetails.AccountNumber == "" || req.ReceiverDetails.BankName == "" || req.ReceiverDetails.SwiftBicCode == "" {
		return nil, ErrInvalidReceiver
	}

	// --- Transactional Block Start (Recommended) ---
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	// Defer rollback in case of errors
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r) // Re-panic after rollback
		} else if tx.Error != nil {
			tx.Rollback() // Rollback if any error occurred
		}
	}()

	// 1. Fetch the current exchange rate
	rate, err := s.GetExchangeRate(ctx, req.FromCurrency, req.ToCurrency)
	if err != nil {
		tx.Rollback() // Ensure rollback on rate fetch error
		return nil, err
	}

	// 2. Calculate received amount (simple calculation, no fees yet)
	amountTo := req.AmountFrom * rate
	fees := 0.0 // Placeholder for fee calculation

	// 3. TODO: Check user balance & Debit funds (Requires AccountService interaction)
	// Example placeholder (replace with actual service call):
	// err = accountService.DebitAccount(ctx, tx, req.UserID, req.FromCurrency, req.AmountFrom + fees)
	// if err != nil {
	// 	 return nil, fmt.Errorf("failed to debit account: %w", err) // Error already wrapped by service
	// }
	// For now, we skip this critical step.

	// 4. Create the transaction record
	receiverDetailsJSON, err := json.Marshal(req.ReceiverDetails)
	if err != nil {
		// tx.Rollback() handled by defer
		return nil, fmt.Errorf("failed to marshal receiver details: %w", err)
	}

	transaction := &models.ExchangeTransaction{
		UserID:          req.UserID,
		FromCurrency:    req.FromCurrency,
		ToCurrency:      req.ToCurrency,
		AmountFrom:      req.AmountFrom,
		AmountTo:        amountTo,
		ExchangeRate:    rate,
		Fees:            fees,
		ReceiverDetails: datatypes.JSON(receiverDetailsJSON), // Use marshaled JSON
		Status:          pb.ExchangeStatus_PENDING.String(),  // Start as PENDING (will error until proto generated)
		CreatedAt:       time.Now().UTC(),
	}

	if err := tx.Create(transaction).Error; err != nil {
		// tx.Rollback() is handled by defer
		return nil, fmt.Errorf("failed to save exchange transaction: %w", err)
	}

	// 5. TODO: Enqueue a background task to process the actual transfer
	// Example placeholder (replace with actual task enqueuing):
	// taskPayload := worker.PayloadSendInternationalTransfer{TransactionID: transaction.ID}
	// _, err = taskDistributor.DistributeTaskSendInternationalTransfer(ctx, &taskPayload)
	// if err != nil {
	// 	 // Rollback or handle compensation logic if task enqueue fails critically
	// 	 return nil, fmt.Errorf("failed to enqueue transfer task: %w", err)
	// }

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// --- Track exchange transaction for statistics (Non-blocking) ---
	s.txTracker.TrackExpenditureAsync(ctx, ExpenditureTrackingParams{
		UserID:          transaction.UserID,
		Amount:          transaction.AmountFrom + transaction.Fees,
		Currency:        transaction.FromCurrency,
		ExpenseType:     "currency_exchange",
		ExpenseID:       transaction.ID,
		Category:        "EXPENSE_CATEGORY_OTHER",
		RecipientName:   req.ReceiverDetails.FullName,
		Merchant:        req.ReceiverDetails.BankName,
		Description:     fmt.Sprintf("Currency exchange %s to %s for %s", req.FromCurrency, req.ToCurrency, req.ReceiverDetails.FullName),
		TransactionDate: &transaction.CreatedAt,
		Metadata: map[string]interface{}{
			"exchange_id":    transaction.ID,
			"from_currency":  transaction.FromCurrency,
			"to_currency":    transaction.ToCurrency,
			"amount_from":    transaction.AmountFrom,
			"amount_to":      transaction.AmountTo,
			"exchange_rate":  transaction.ExchangeRate,
			"fees":           transaction.Fees,
			"receiver_name":  req.ReceiverDetails.FullName,
			"bank_name":      req.ReceiverDetails.BankName,
			"account_number": req.ReceiverDetails.AccountNumber,
			"swift_code":     req.ReceiverDetails.SwiftBicCode,
		},
	})

	// --- Enqueue Tx File Update Task (AFTER successful commit) ---
	txFilePayloadBytes, err := tasks.NewGenerateTxDataFileTask(transaction.UserID)
	if err != nil {
		fmt.Printf("CRITICAL ERROR: Failed creating tx file generation payload for user %d after exchange %s: %v\n", transaction.UserID, transaction.ID, err)
	} else {
		opts := []asynq.Option{
			asynq.MaxRetry(3),
			asynq.Timeout(10 * time.Minute),
			asynq.Queue(tasks.QueueLow),
		}
		if err := s.distributor.DistributeTask(ctx, tasks.TypeGenerateTxDataFile, txFilePayloadBytes, opts...); err != nil {
			fmt.Printf("CRITICAL ERROR: Failed enqueuing tx file generation task for user %d after exchange %s: %v\n", transaction.UserID, transaction.ID, err)
		}
	}

	return transaction, nil
}

// GetRecentExchangesServiceRequest contains parameters for fetching history
type GetRecentExchangesServiceRequest struct {
	UserID    uint
	PageSize  int
	PageToken string // Use transaction ID as page token
}

// GetRecentExchanges retrieves a paginated list of recent exchanges
func (s *ExchangeService) GetRecentExchanges(ctx context.Context, req *GetRecentExchangesServiceRequest) ([]models.ExchangeTransaction, string, error) {
	if req.UserID == 0 {
		return nil, "", ErrInvalidUserID
	}

	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = DefaultExchangePageSize
	}

	var transactions []models.ExchangeTransaction
	query := s.db.WithContext(ctx).Model(&models.ExchangeTransaction{}).
		Where("user_id = ?", req.UserID)

	// Pagination based on transaction ID
	if req.PageToken != "" {
		var lastTx models.ExchangeTransaction
		err := s.db.WithContext(ctx).Select("created_at").First(&lastTx, "id = ? AND user_id = ?", req.PageToken, req.UserID).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return []models.ExchangeTransaction{}, "", fmt.Errorf("invalid page token: %w", err)
			}
			return nil, "", fmt.Errorf("failed to query page token transaction: %w", err)
		}
		query = query.Where("(created_at, id) < (?, ?)", lastTx.CreatedAt, req.PageToken)
	}

	// Order by creation time descending, ID secondary
	err := query.Order("created_at desc, id desc").Limit(pageSize + 1).Find(&transactions).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", fmt.Errorf("failed to retrieve transactions: %w", err)
	}

	// Determine next page token
	nextPageToken := ""
	if len(transactions) > pageSize {
		nextPageToken = transactions[pageSize-1].ID
		transactions = transactions[:pageSize] // Trim the extra transaction
	}

	return transactions, nextPageToken, nil
}

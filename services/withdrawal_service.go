package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/tasks"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// --- Withdraw Service Errors ---
var (
	ErrWithdrawalSourceAccountNotFound = errors.New("source account not found or does not belong to user")
	ErrWithdrawalInvalidAmount         = errors.New("withdrawal amount must be positive")
	ErrWithdrawalCurrencyMismatch      = errors.New("withdrawal currency does not match source account currency")
	ErrWithdrawalInsufficientFunds     = errors.New("insufficient funds in the source account")
	ErrWithdrawalProcessingFailed      = errors.New("failed to process withdrawal")
	ErrWithdrawalInitiationFailed      = errors.New("withdrawal service: failed to initiate withdrawal record")
	ErrWithdrawalEnqueueTaskFailed     = errors.New("withdrawal service: failed to enqueue processing task")
	ErrWithdrawalCurrencyMissing       = errors.New("withdrawal service: currency is required")
	ErrWithdrawalNotFound              = errors.New("withdrawal service: withdrawal not found")
	ErrWithdrawalAccessDenied          = errors.New("withdrawal service: access denied to withdrawal details")
)

// --- Withdraw Service Interface ---

// IWithdrawalService defines the interface for withdrawal operations.
type IWithdrawalService interface {
	InitiateWithdrawal(ctx context.Context, userID uint, req InitiateWithdrawalRequest) (*pb.InitiateWithdrawalResponse, error)
	GetWithdrawalDetails(ctx context.Context, withdrawalID string, userID uint) (*models.Withdrawal, error)
}

// --- Withdraw Service Struct ---

// WithdrawalService handles business logic related to withdrawals.
type WithdrawalService struct {
	db          *gorm.DB
	distributor tasks.TaskDistributor
}

// --- Withdraw Service Constructor ---

// NewWithdrawalService creates a new WithdrawalService.
func NewWithdrawalService(db *gorm.DB, distributor tasks.TaskDistributor) IWithdrawalService {
	return &WithdrawalService{db: db, distributor: distributor}
}

// --- Withdraw Service Types ---

// InitiateWithdrawalRequest mirrors the proto request but uses Go types.
type InitiateWithdrawalRequest struct {
	UserID              uint   `json:"user_id"`
	SourceAccountID     uint   `json:"source_account_id"`
	Amount              int64  `json:"amount"` // Use int64
	Currency            string `json:"currency"`
	TargetBankName      string `json:"target_bank_name"`
	TargetAccountNumber string `json:"target_account_number"`
	TargetSortCode      string `json:"target_sort_code,omitempty"`
}

// --- Withdraw Service Methods ---

// InitiateWithdrawal validates, deducts funds, creates a withdrawal record, and enqueues a task for external payout processing.
func (s *WithdrawalService) InitiateWithdrawal(ctx context.Context, userID uint, req InitiateWithdrawalRequest) (*pb.InitiateWithdrawalResponse, error) {
	// 1. Validate Input
	if req.Amount <= 0 {
		return nil, ErrWithdrawalInvalidAmount
	}
	// TODO: More validation?

	var withdrawal models.Withdrawal // Define withdrawal variable early

	// 2. Use Transaction for atomicity
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 2a. Verify Source Account, Currency, and Sufficient Funds
		var sourceAccount models.Account
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ? AND owner_user_id = ?", req.SourceAccountID, userID).First(&sourceAccount).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrWithdrawalSourceAccountNotFound
			}
			return fmt.Errorf("db error finding source account: %w", err)
		}

		if sourceAccount.Currency != req.Currency {
			return ErrWithdrawalCurrencyMismatch
		}

		// Check balance using int64
		totalDeduction := req.Amount // Assuming no fee for now
		if sourceAccount.Balance < totalDeduction {
			return ErrWithdrawalInsufficientFunds
		}

		// 2b. Deduct Funds from Source Account
		sourceAccount.Balance -= totalDeduction
		if err := tx.Save(&sourceAccount).Error; err != nil {
			return fmt.Errorf("failed to update source account balance: %w", err)
		}

		// 2c. Create Withdrawal Record (Status: PENDING)
		withdrawal = models.Withdrawal{
			ID:                  uuid.NewString(),
			UserID:              userID,
			SourceAccountID:     req.SourceAccountID,
			Amount:              req.Amount, // Use int64
			Currency:            req.Currency,
			TargetBankName:      req.TargetBankName,
			TargetAccountNumber: req.TargetAccountNumber,
			TargetSortCode:      req.TargetSortCode,
			Status:              models.WithdrawalStatusPending, // Status indicates payout pending
		}
		if err := tx.Create(&withdrawal).Error; err != nil {
			return fmt.Errorf("%w: %v", ErrWithdrawalInitiationFailed, err)
		}

		// 2d. Enqueue Processing Task within the same transaction
		withdrawalPayload := &tasks.WithdrawalProcessPayload{WithdrawalID: withdrawal.ID}
		opts := []asynq.Option{
			asynq.MaxRetry(3),
		}
		if err := s.distributor.DistributeTaskWithdrawalProcess(ctx, withdrawalPayload, opts...); err != nil {
			fmt.Printf("CRITICAL: Error distributing withdrawal task for ID %s: %v\n", withdrawal.ID, err)
			return fmt.Errorf("%w: %v", ErrWithdrawalEnqueueTaskFailed, err)
		}

		return nil // Commit transaction
	})

	if txErr != nil {
		// Translate specific validation errors if needed, otherwise return txErr
		if errors.Is(txErr, ErrWithdrawalSourceAccountNotFound) ||
			errors.Is(txErr, ErrWithdrawalCurrencyMismatch) ||
			errors.Is(txErr, ErrWithdrawalInsufficientFunds) {
			return nil, txErr // Return specific validation error
		}
		// Return generic failure for other transaction errors
		return nil, fmt.Errorf("withdrawal initiation transaction failed: %w", txErr)
	}

	// 3. Return Acknowledgement Response
	resp := &pb.InitiateWithdrawalResponse{
		WithdrawalId: withdrawal.ID, // Withdrawal ID is available here
		Status:       pb.WithdrawalStatus_WITHDRAWAL_STATUS_PENDING,
		Message:      "Withdrawal initiated successfully. Processing payout.",
	}

	return resp, nil
}

// GetWithdrawalDetails retrieves a specific withdrawal record by ID, checking ownership.
func (s *WithdrawalService) GetWithdrawalDetails(ctx context.Context, withdrawalID string, userID uint) (*models.Withdrawal, error) {
	var withdrawal models.Withdrawal

	err := s.db.WithContext(ctx).Where("id = ?", withdrawalID).First(&withdrawal).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWithdrawalNotFound
		}
		return nil, fmt.Errorf("db error finding withdrawal: %w", err)
	}

	// Check ownership
	if withdrawal.UserID != userID {
		return nil, ErrWithdrawalAccessDenied
	}

	return &withdrawal, nil
}

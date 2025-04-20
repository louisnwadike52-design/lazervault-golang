package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// --- Withdraw Service Errors ---
var (
	ErrWithdrawalSourceAccountNotFound = errors.New("source account not found or does not belong to user")
	ErrWithdrawalInvalidAmount         = errors.New("withdrawal amount must be positive")
	ErrWithdrawalCurrencyMismatch      = errors.New("withdrawal currency does not match source account currency")
	ErrWithdrawalInsufficientFunds     = errors.New("insufficient funds in the source account")
	ErrWithdrawalProcessingFailed      = errors.New("failed to process withdrawal")
)

// --- Withdraw Service Interface ---

// IWithdrawalService defines the interface for withdrawal operations.
type IWithdrawalService interface {
	InitiateWithdrawal(ctx context.Context, userID uint, req InitiateWithdrawalRequest) (*models.Withdrawal, error)
}

// --- Withdraw Service Struct ---

// WithdrawalService handles business logic related to withdrawals.
type WithdrawalService struct {
	db *gorm.DB
	// accountService IAccountService // Optional: Inject if needed for complex balance logic/checks
}

// --- Withdraw Service Constructor ---

// NewWithdrawalService creates a new WithdrawalService.
func NewWithdrawalService(db *gorm.DB) IWithdrawalService {
	return &WithdrawalService{db: db}
}

// --- Withdraw Service Types ---

// InitiateWithdrawalRequest mirrors the proto request but includes UserID.
type InitiateWithdrawalRequest struct {
	UserID              uint    `json:"user_id"`
	SourceAccountID     uint    `json:"source_account_id"`
	Amount              float64 `json:"amount"`
	Currency            string  `json:"currency"`
	TargetBankName      string  `json:"target_bank_name"`
	TargetAccountNumber string  `json:"target_account_number"`
	TargetSortCode      string  `json:"target_sort_code,omitempty"`
}

// --- Withdraw Service Methods ---

// InitiateWithdrawal creates a withdrawal record and updates the source account balance.
func (s *WithdrawalService) InitiateWithdrawal(ctx context.Context, userID uint, req InitiateWithdrawalRequest) (*models.Withdrawal, error) {
	// 1. Validate Input
	if req.Amount <= 0 {
		return nil, ErrWithdrawalInvalidAmount
	}
	// TODO: Validate target bank details format if necessary

	// 2. Use Transaction for atomicity
	var createdWithdrawal *models.Withdrawal
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 3. Verify Source Account, Currency, and Sufficient Funds
		var sourceAccount models.Account
		err := tx.Where("id = ? AND owner_user_id = ?", req.SourceAccountID, userID).First(&sourceAccount).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrWithdrawalSourceAccountNotFound
			}
			return fmt.Errorf("db error finding source account: %w", err)
		}

		if sourceAccount.Currency != req.Currency {
			return ErrWithdrawalCurrencyMismatch
		}

		// Check balance (consider potential fees here if applicable)
		// totalDeduction := req.Amount + calculateWithdrawalFee(req.Amount)
		totalDeduction := req.Amount
		if sourceAccount.Balance < totalDeduction {
			return ErrWithdrawalInsufficientFunds
		}

		// 4. Create Initial Withdrawal Record (Status: PENDING or PROCESSING)
		// If payout is async, set status to PROCESSING. If simulated sync, set to COMPLETED later.
		withdrawal := models.Withdrawal{
			ID:              uuid.NewString(),
			UserID:          userID,
			SourceAccountID: req.SourceAccountID,
			Amount:          req.Amount,
			// Fee:              calculateWithdrawalFee(req.Amount),
			// TotalAmount:       totalDeduction,
			Currency:            req.Currency,
			TargetBankName:      req.TargetBankName,
			TargetAccountNumber: req.TargetAccountNumber,
			TargetSortCode:      req.TargetSortCode,
			Status:              models.WithdrawalStatusPending, // Or PROCESSING if async
		}
		if err := tx.Create(&withdrawal).Error; err != nil {
			return fmt.Errorf("failed to create withdrawal record: %w", err)
		}

		// 5. Deduct Funds from Source Account
		sourceAccount.Balance -= totalDeduction
		if err := tx.Save(&sourceAccount).Error; err != nil {
			// If deducting balance fails, mark withdrawal as FAILED
			withdrawal.Status = models.WithdrawalStatusFailed
			withdrawal.FailureReason = "Failed to deduct funds from source account"
			now := time.Now()
			withdrawal.FailedAt = &now
			tx.Save(&withdrawal) // Attempt to save failed status
			return fmt.Errorf("failed to update source account balance: %w", err)
		}

		// --- Simulation: Mark as Completed (if not using async tasks) ---
		// If using background tasks for actual payout, this status update would happen in the worker.
		withdrawal.Status = models.WithdrawalStatusCompleted
		now := time.Now()
		withdrawal.CompletedAt = &now
		if err := tx.Save(&withdrawal).Error; err != nil {
			// Balance deducted, but status not updated. Critical inconsistency.
			return fmt.Errorf("critical: failed to update withdrawal status after balance deduction: %w", err)
		}
		// --- End Simulation ---

		createdWithdrawal = &withdrawal // Assign successful withdrawal
		return nil                      // Commit transaction
	})

	if txErr != nil {
		// Translate specific errors
		if errors.Is(txErr, ErrWithdrawalSourceAccountNotFound) ||
			errors.Is(txErr, ErrWithdrawalCurrencyMismatch) ||
			errors.Is(txErr, ErrWithdrawalInsufficientFunds) {
			return nil, txErr
		}
		// Generic processing error
		return nil, fmt.Errorf("%w: %v", ErrWithdrawalProcessingFailed, txErr)
	}

	return createdWithdrawal, nil
}

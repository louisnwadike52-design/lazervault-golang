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

// --- Deposit Service Errors ---
var (
	ErrDepositTargetAccountNotFound = errors.New("target account not found or does not belong to user")
	ErrDepositInvalidAmount         = errors.New("deposit amount must be positive")
	ErrDepositCurrencyMismatch      = errors.New("deposit currency does not match target account currency")
	ErrDepositProcessingFailed      = errors.New("failed to process deposit and update balance")
)

// --- Deposit Service Interface ---

// IDepositService defines the interface for deposit operations.
type IDepositService interface {
	InitiateDeposit(ctx context.Context, userID uint, req InitiateDepositRequest) (*models.Deposit, error)
}

// --- Deposit Service Struct ---

// DepositService handles business logic related to deposits.
type DepositService struct {
	db *gorm.DB
	// Add IAccountService if direct balance update is preferred over raw SQL/GORM update
	// accountService IAccountService
}

// --- Deposit Service Constructor ---

// NewDepositService creates a new DepositService.
func NewDepositService(db *gorm.DB) IDepositService {
	return &DepositService{db: db}
}

// --- Deposit Service Types ---

// InitiateDepositRequest mirrors the proto request but includes UserID.
type InitiateDepositRequest struct {
	UserID          uint    `json:"user_id"` // Added from context
	TargetAccountID uint    `json:"target_account_id"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	SourceBankName  string  `json:"source_bank_name"`
}

// --- Deposit Service Methods ---

// InitiateDeposit creates a deposit record and simulates processing.
// In a real system, this would likely involve a payment gateway and background tasks.
func (s *DepositService) InitiateDeposit(ctx context.Context, userID uint, req InitiateDepositRequest) (*models.Deposit, error) {
	// 1. Validate Input
	if req.Amount <= 0 {
		return nil, ErrDepositInvalidAmount
	}
	// TODO: Add currency code validation (e.g., against a known list)

	// 2. Use Transaction for atomicity
	var createdDeposit *models.Deposit
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 3. Verify Target Account and Currency
		var targetAccount models.Account
		err := tx.Where("id = ? AND owner_user_id = ?", req.TargetAccountID, userID).First(&targetAccount).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrDepositTargetAccountNotFound
			}
			return fmt.Errorf("db error finding target account: %w", err)
		}

		if targetAccount.Currency != req.Currency {
			return ErrDepositCurrencyMismatch
		}

		// 4. Create Initial Deposit Record (Status: PENDING)
		deposit := models.Deposit{
			ID:              uuid.NewString(), // Generate UUID
			UserID:          userID,
			TargetAccountID: req.TargetAccountID,
			Amount:          req.Amount,
			Currency:        req.Currency,
			SourceBankName:  req.SourceBankName,
			Status:          models.DepositStatusPending, // Start as pending
		}

		if err := tx.Create(&deposit).Error; err != nil {
			return fmt.Errorf("failed to create deposit record: %w", err)
		}

		// --- Simulation: Immediately attempt to complete the deposit ---
		// In a real app, this logic (balance update, status change) would likely be
		// in a separate background task triggered after payment gateway confirmation.

		// 5. Update Account Balance
		targetAccount.Balance += req.Amount
		if err := tx.Save(&targetAccount).Error; err != nil {
			deposit.Status = models.DepositStatusFailed // Mark deposit as failed if balance update fails
			deposit.FailureReason = "Failed to update account balance"
			now := time.Now()
			deposit.FailedAt = &now
			tx.Save(&deposit) // Save failed status
			return fmt.Errorf("failed to update target account balance: %w", err)
		}

		// 6. Update Deposit Status to COMPLETED
		deposit.Status = models.DepositStatusCompleted
		now := time.Now()
		deposit.CompletedAt = &now
		if err := tx.Save(&deposit).Error; err != nil {
			// If this fails, the balance is updated but the deposit record isn't marked completed.
			// This indicates an inconsistency. Depending on requirements, you might:
			// - Log critical error
			// - Attempt to rollback (though balance is already saved)
			// - Enqueue a reconciliation task
			return fmt.Errorf("critical: failed to update deposit status after balance update: %w", err)
		}

		createdDeposit = &deposit // Assign the successfully created/updated deposit
		return nil                // Commit transaction
	})

	if txErr != nil {
		// Translate specific errors if needed, otherwise return the transaction error
		if errors.Is(txErr, ErrDepositTargetAccountNotFound) || errors.Is(txErr, ErrDepositCurrencyMismatch) {
			return nil, txErr
		}
		// Generic processing error
		return nil, fmt.Errorf("%w: %v", ErrDepositProcessingFailed, txErr)
	}

	return createdDeposit, nil
}

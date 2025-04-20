package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/utils"

	"gorm.io/gorm"
)

// --- Account Service Errors ---
var ErrInvalidAccountType = errors.New("invalid account type provided")

// --- Account Service Interface ---

// IAccountService defines the interface for account operations
type IAccountService interface {
	CreateAccount(ctx context.Context, req CreateAccountRequest) (*models.Account, error)
	GetAccounts(ctx context.Context, ownerUserID uint) ([]models.Account, error)
}

// --- Account Service Struct ---

// AccountService handles business logic related to user accounts.
type AccountService struct {
	db *gorm.DB
}

// --- Account Service Constructor ---

// NewAccountService creates a new AccountService.
func NewAccountService(db *gorm.DB) IAccountService { // Return interface type
	return &AccountService{db: db}
}

// --- Account Service Types ---

// CreateAccountRequest defines parameters for creating an account.
type CreateAccountRequest struct {
	OwnerUserID uint   `json:"owner_user_id" binding:"required"`
	AccountType string `json:"account_type" binding:"required"`
	Currency    string `json:"currency" binding:"required,iso4217"`
}

// --- Account Service Methods ---

// CreateAccount creates a new financial account for a user.
func (s *AccountService) CreateAccount(ctx context.Context, req CreateAccountRequest) (*models.Account, error) {
	// Validate Account Type
	switch req.AccountType {
	case models.AccountTypePersonal,
		models.AccountTypeSavings,
		models.AccountTypeInvestment:
		// Valid type, continue
	default:
		return nil, ErrInvalidAccountType
	}

	// Generate a unique LazerVault account number
	randomPart, err := utils.GenerateRandomString(8)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random part for account number: %w", err)
	}
	accountNumber := fmt.Sprintf("LV-%s-%s", req.Currency, randomPart)

	account := models.Account{
		OwnerUserID:   req.OwnerUserID,
		AccountType:   req.AccountType,
		Currency:      req.Currency,
		Balance:       0.0, // Initial balance is zero
		AccountNumber: accountNumber,
		IsActive:      true,
	}

	result := s.db.WithContext(ctx).Create(&account)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to create account: %w", result.Error)
	}

	return &account, nil
}

// GetAccounts retrieves all accounts for a specific user.
func (s *AccountService) GetAccounts(ctx context.Context, ownerUserID uint) ([]models.Account, error) {
	var accounts []models.Account

	result := s.db.WithContext(ctx).Where("owner_user_id = ?", ownerUserID).Find(&accounts)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to retrieve accounts: %w", result.Error)
	}

	return accounts, nil
}

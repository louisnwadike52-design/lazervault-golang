package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/database"
	"lazervaultGo/models"

	"gorm.io/gorm"
)

// IReferralService defines the interface for referral service
type IReferralService interface {
	ValidateReferralCode(ctx context.Context, code string) (bool, string, error)
	GetMyReferralCode(ctx context.Context, userID uint) (*models.ReferralCode, error)
	GetMyReferralStats(ctx context.Context, userID uint) (int64, int, int, string, error)
	GetMyReferrals(ctx context.Context, userID uint, page, pageSize int) ([]models.ReferralTransaction, error)
	GetReferralLeaderboard(ctx context.Context, limit int) ([]map[string]interface{}, error)
	GetCountryRewardConfig(ctx context.Context, countryCode string) (*models.CountryRewardConfig, error)
	ProcessReferral(ctx context.Context, refereeUserID uint, referralCode, currency string) error
}

type ReferralService struct {
	db *gorm.DB
}

// NewReferralService creates a new referral service
func NewReferralService(db *gorm.DB) *ReferralService {
	return &ReferralService{db: db}
}

// ValidateReferralCode validates if a referral code exists and is active
func (s *ReferralService) ValidateReferralCode(ctx context.Context, code string) (bool, string, error) {
	referralCode, err := database.FindReferralCodeByCode(s.db, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "Referral code does not exist", nil
		}
		return false, "Error validating referral code", err
	}

	if !referralCode.IsActive {
		return false, "Referral code is no longer active", nil
	}

	return true, "Referral code is valid", nil
}

// GetMyReferralCode gets the referral code for a user, creating one if it doesn't exist
func (s *ReferralService) GetMyReferralCode(ctx context.Context, userID uint) (*models.ReferralCode, error) {
	// Try to find existing referral code
	referralCode, err := database.FindReferralCodeByUserID(s.db, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Referral code doesn't exist, create one for this user

			// Get user information to generate code
			var user models.User
			if err := s.db.First(&user, userID).Error; err != nil {
				return nil, fmt.Errorf("failed to find user: %w", err)
			}

			// Determine username for code generation
			username := ""
			if user.Username != nil {
				username = *user.Username
			}
			if username == "" {
				username = user.FirstName
			}

			// Generate unique referral code
			code, err := database.GenerateUniqueReferralCode(s.db, username)
			if err != nil {
				return nil, fmt.Errorf("failed to generate referral code: %w", err)
			}

			// Create referral code record
			newReferralCode := &models.ReferralCode{
				UserID:   userID,
				Code:     code,
				IsActive: true,
			}

			if err := database.CreateReferralCode(s.db, newReferralCode); err != nil {
				return nil, fmt.Errorf("failed to create referral code: %w", err)
			}

			return newReferralCode, nil
		}
		return nil, err
	}
	return referralCode, nil
}

// GetMyReferralStats gets aggregated stats for a user's referrals
func (s *ReferralService) GetMyReferralStats(ctx context.Context, userID uint) (int64, int, int, string, error) {
	return database.GetReferralStats(s.db, userID)
}

// GetMyReferrals gets a paginated list of referrals made by a user
func (s *ReferralService) GetMyReferrals(ctx context.Context, userID uint, page, pageSize int) ([]models.ReferralTransaction, error) {
	return database.FindReferralTransactionsByReferrerID(s.db, userID, page, pageSize)
}

// GetReferralLeaderboard gets the top referrers
func (s *ReferralService) GetReferralLeaderboard(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	return database.GetReferralLeaderboard(s.db, limit)
}

// GetCountryRewardConfig gets reward configuration for a country
func (s *ReferralService) GetCountryRewardConfig(ctx context.Context, countryCode string) (*models.CountryRewardConfig, error) {
	config, err := database.FindCountryRewardConfigByCountry(s.db, countryCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Return default config if not found
			return &models.CountryRewardConfig{
				CountryCode:    countryCode,
				Currency:       "GBP",
				ReferrerReward: 10000, // £100
				RefereeReward:  5000,  // £50
				IsActive:       true,
			}, nil
		}
		return nil, err
	}
	return config, nil
}

// ProcessReferral processes a referral when a new user signs up
func (s *ReferralService) ProcessReferral(ctx context.Context, refereeUserID uint, referralCode, currency string) error {
	// 1. Validate referral code
	referralCodeRecord, err := database.FindReferralCodeByCode(s.db, referralCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("invalid referral code")
		}
		return err
	}

	if !referralCodeRecord.IsActive {
		return fmt.Errorf("referral code is no longer active")
	}

	// 2. Check if referee already used a referral code
	existingTransaction, err := database.FindReferralTransactionByRefereeID(s.db, refereeUserID)
	if err == nil && existingTransaction != nil {
		return fmt.Errorf("user has already used a referral code")
	}

	// 3. Get country reward config
	config, err := s.GetCountryRewardConfig(ctx, currency)
	if err != nil {
		return fmt.Errorf("failed to get reward configuration: %w", err)
	}

	// 4. Create referral transaction
	transaction := &models.ReferralTransaction{
		ReferrerUserID:       referralCodeRecord.UserID,
		RefereeUserID:        refereeUserID,
		ReferralCodeUsed:     referralCode,
		Status:               models.ReferralStatusPending,
		ReferrerRewardAmount: config.ReferrerReward,
		RefereeRewardAmount:  config.RefereeReward,
		Currency:             config.Currency,
	}

	// 5. Execute in database transaction
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create transaction record
		if err := database.CreateReferralTransaction(tx, transaction); err != nil {
			return fmt.Errorf("failed to create referral transaction: %w", err)
		}

		// Credit referrer's Personal account
		if err := s.creditAccount(tx, referralCodeRecord.UserID, config.ReferrerReward, config.Currency, "Referral Bonus"); err != nil {
			// Mark transaction as failed
			transaction.Status = models.ReferralStatusFailed
			failureReason := fmt.Sprintf("Failed to credit referrer: %v", err)
			transaction.FailureReason = &failureReason
			_ = tx.Save(transaction)
			return fmt.Errorf("failed to credit referrer: %w", err)
		}

		// Credit referee's Personal account
		if err := s.creditAccount(tx, refereeUserID, config.RefereeReward, config.Currency, "Referral Welcome Bonus"); err != nil {
			// Mark transaction as failed
			transaction.Status = models.ReferralStatusFailed
			failureReason := fmt.Sprintf("Failed to credit referee: %v", err)
			transaction.FailureReason = &failureReason
			_ = tx.Save(transaction)
			return fmt.Errorf("failed to credit referee: %w", err)
		}

		// Mark transaction as completed
		transaction.Status = models.ReferralStatusCompleted
		now := tx.NowFunc()
		transaction.CompletedAt = &now
		if err := tx.Save(transaction).Error; err != nil {
			return fmt.Errorf("failed to update transaction status: %w", err)
		}

		return nil
	})

	return err
}

// creditAccount credits an account with the referral reward
func (s *ReferralService) creditAccount(tx *gorm.DB, userID uint, amount int, currency, description string) error {
	// Find user's Personal account for the currency
	var account models.Account
	err := tx.Where("owner_user_id = ? AND account_type = ? AND currency = ?",
		userID, models.AccountTypePersonal, currency).
		First(&account).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Create Personal account if it doesn't exist
			account = models.Account{
				OwnerUserID: userID,
				AccountType: models.AccountTypePersonal,
				Currency:    currency,
				Balance:     0,
				Country:     getCurrencyCountry(currency),
			}
			if err := tx.Create(&account).Error; err != nil {
				return fmt.Errorf("failed to create account: %w", err)
			}
		} else {
			return fmt.Errorf("failed to find account: %w", err)
		}
	}

	// Update account balance
	account.Balance += int64(amount)
	if err := tx.Save(&account).Error; err != nil {
		return fmt.Errorf("failed to update account balance: %w", err)
	}

	// Create deposit record for audit trail
	deposit := &models.Deposit{
		UserID:          userID,
		TargetAccountID: account.ID,
		Amount:          int64(amount),
		Currency:        currency,
		Status:          "SUCCESS",
		SourceBankName:  description,
	}
	if err := tx.Create(deposit).Error; err != nil {
		return fmt.Errorf("failed to create deposit record: %w", err)
	}

	return nil
}

// getCurrencyCountry maps currency to country code
func getCurrencyCountry(currency string) string {
	currencyToCountry := map[string]string{
		"GBP": "GB",
		"USD": "US",
		"EUR": "EU",
		"NGN": "NG",
		"CAD": "CA",
		"AUD": "AU",
		"INR": "IN",
		"CNY": "CN",
		"JPY": "JP",
		"KES": "KE",
		"ZAR": "ZA",
	}

	if country, ok := currencyToCountry[currency]; ok {
		return country
	}
	return "US" // default
}

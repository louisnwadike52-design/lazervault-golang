package database

import (
	"fmt"
	"lazervaultGo/models"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"
)

// FindReferralCodeByCode finds a referral code by its code string
func FindReferralCodeByCode(db *gorm.DB, code string) (*models.ReferralCode, error) {
	var referralCode models.ReferralCode
	err := db.Where("code = ? AND is_active = ?", strings.ToUpper(code), true).
		Preload("User").
		First(&referralCode).Error
	if err != nil {
		return nil, err
	}
	return &referralCode, nil
}

// FindReferralCodeByUserID finds a referral code by user ID
func FindReferralCodeByUserID(db *gorm.DB, userID uint) (*models.ReferralCode, error) {
	var referralCode models.ReferralCode
	err := db.Where("user_id = ?", userID).
		First(&referralCode).Error
	if err != nil {
		return nil, err
	}
	return &referralCode, nil
}

// CreateReferralCode creates a new referral code
func CreateReferralCode(db *gorm.DB, referralCode *models.ReferralCode) error {
	return db.Create(referralCode).Error
}

// GenerateUniqueReferralCode generates a unique referral code for a user
func GenerateUniqueReferralCode(db *gorm.DB, username string) (string, error) {
	const maxAttempts = 10

	// Normalize username
	normalizedUsername := strings.ToUpper(username)
	if len(normalizedUsername) > 8 {
		normalizedUsername = normalizedUsername[:8]
	}
	if normalizedUsername == "" {
		// Generate random letters if no username
		letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		normalizedUsername = ""
		for i := 0; i < 4; i++ {
			normalizedUsername += string(letters[rand.Intn(len(letters))])
		}
	}

	// Try to generate unique code
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Generate 4 random digits
		rand.Seed(time.Now().UnixNano() + int64(attempt))
		randomDigits := fmt.Sprintf("%04d", rand.Intn(10000))

		// Combine username and random digits
		code := normalizedUsername + randomDigits

		// Check if code exists
		var existing models.ReferralCode
		err := db.Where("code = ?", code).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			// Code is unique
			return code, nil
		}
		if err != nil {
			return "", err
		}
		// Code exists, try again
	}

	return "", fmt.Errorf("failed to generate unique referral code after %d attempts", maxAttempts)
}

// FindReferralTransactionsByReferrerID finds all referral transactions where user is the referrer
func FindReferralTransactionsByReferrerID(db *gorm.DB, referrerUserID uint, page, pageSize int) ([]models.ReferralTransaction, error) {
	var transactions []models.ReferralTransaction
	offset := (page - 1) * pageSize

	err := db.Where("referrer_user_id = ?", referrerUserID).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Preload("Referee").
		Find(&transactions).Error
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

// FindReferralTransactionByRefereeID finds a referral transaction by referee user ID
func FindReferralTransactionByRefereeID(db *gorm.DB, refereeUserID uint) (*models.ReferralTransaction, error) {
	var transaction models.ReferralTransaction
	err := db.Where("referee_user_id = ?", refereeUserID).
		First(&transaction).Error
	if err != nil {
		return nil, err
	}
	return &transaction, nil
}

// CreateReferralTransaction creates a new referral transaction
func CreateReferralTransaction(db *gorm.DB, transaction *models.ReferralTransaction) error {
	return db.Create(transaction).Error
}

// UpdateReferralTransaction updates a referral transaction
func UpdateReferralTransaction(db *gorm.DB, transaction *models.ReferralTransaction) error {
	return db.Save(transaction).Error
}

// GetReferralStats gets aggregated referral statistics for a user
func GetReferralStats(db *gorm.DB, userID uint) (totalReferrals int64, totalRewardsEarned, pendingRewards int, currency string, err error) {
	// Count total successful referrals
	err = db.Model(&models.ReferralTransaction{}).
		Where("referrer_user_id = ? AND status = ?", userID, models.ReferralStatusCompleted).
		Count(&totalReferrals).Error
	if err != nil {
		return 0, 0, 0, "", err
	}

	// Sum total rewards earned (completed transactions)
	var result struct {
		TotalRewards int
		Currency     string
	}
	err = db.Model(&models.ReferralTransaction{}).
		Select("COALESCE(SUM(referrer_reward_amount), 0) as total_rewards, MAX(currency) as currency").
		Where("referrer_user_id = ? AND status = ?", userID, models.ReferralStatusCompleted).
		Scan(&result).Error
	if err != nil {
		return 0, 0, 0, "", err
	}
	totalRewardsEarned = result.TotalRewards
	currency = result.Currency

	// Sum pending rewards (pending transactions)
	var pendingResult struct {
		PendingRewards int
	}
	err = db.Model(&models.ReferralTransaction{}).
		Select("COALESCE(SUM(referrer_reward_amount), 0) as pending_rewards").
		Where("referrer_user_id = ? AND status = ?", userID, models.ReferralStatusPending).
		Scan(&pendingResult).Error
	if err != nil {
		return 0, 0, 0, "", err
	}
	pendingRewards = pendingResult.PendingRewards

	// Default currency if no transactions
	if currency == "" {
		currency = "GBP"
	}

	return totalReferrals, totalRewardsEarned, pendingRewards, currency, nil
}

// GetReferralLeaderboard gets top referrers ordered by total referrals
func GetReferralLeaderboard(db *gorm.DB, limit int) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	rows, err := db.Raw(`
		SELECT
			u.id as user_id,
			u.first_name,
			u.last_name,
			u.username,
			COUNT(rt.id) as total_referrals,
			ROW_NUMBER() OVER (ORDER BY COUNT(rt.id) DESC, u.created_at ASC) as rank
		FROM users u
		INNER JOIN referral_transactions rt ON u.id = rt.referrer_user_id
		WHERE rt.status = ?
		GROUP BY u.id, u.first_name, u.last_name, u.username, u.created_at
		ORDER BY total_referrals DESC, u.created_at ASC
		LIMIT ?
	`, models.ReferralStatusCompleted, limit).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var userID uint
		var firstName, lastName, username string
		var totalReferrals int64
		var rank int

		err := rows.Scan(&userID, &firstName, &lastName, &username, &totalReferrals, &rank)
		if err != nil {
			return nil, err
		}

		results = append(results, map[string]interface{}{
			"user_id":        userID,
			"first_name":     firstName,
			"last_name":      lastName,
			"username":       username,
			"total_referrals": totalReferrals,
			"rank":           rank,
		})
	}

	return results, nil
}

// FindCountryRewardConfigByCountry finds reward configuration for a country
func FindCountryRewardConfigByCountry(db *gorm.DB, countryCode string) (*models.CountryRewardConfig, error) {
	var config models.CountryRewardConfig
	err := db.Where("country_code = ? AND is_active = ?", strings.ToUpper(countryCode), true).
		Order("created_at DESC").
		First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// CreateCountryRewardConfig creates a new country reward configuration
func CreateCountryRewardConfig(db *gorm.DB, config *models.CountryRewardConfig) error {
	return db.Create(config).Error
}

// GetAllCountryRewardConfigs gets all active country reward configurations
func GetAllCountryRewardConfigs(db *gorm.DB) ([]models.CountryRewardConfig, error) {
	var configs []models.CountryRewardConfig
	err := db.Where("is_active = ?", true).Find(&configs).Error
	if err != nil {
		return nil, err
	}
	return configs, nil
}

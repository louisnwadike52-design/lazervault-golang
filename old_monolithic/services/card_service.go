package services

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"gorm.io/gorm"
	"lazervaultGo/models"
)

// CardService handles all card-related business logic
type CardService struct {
	db *gorm.DB
}

// NewCardService creates a new card service
func NewCardService(db *gorm.DB) *CardService {
	return &CardService{db: db}
}

// generateCardNumber generates a valid test card number (Luhn algorithm)
func (s *CardService) generateCardNumber() string {
	// Generate test Visa card number (starts with 4)
	prefix := "4"
	// Generate 14 random digits
	for i := 0; i < 14; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		prefix += fmt.Sprintf("%d", n)
	}
	// Calculate Luhn checksum
	checksum := s.calculateLuhnChecksum(prefix)
	return prefix + fmt.Sprintf("%d", checksum)
}

// calculateLuhnChecksum calculates the Luhn check digit
func (s *CardService) calculateLuhnChecksum(number string) int {
	sum := 0
	alternate := false
	for i := len(number) - 1; i >= 0; i-- {
		digit := int(number[i] - '0')
		if alternate {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		alternate = !alternate
	}
	return (10 - (sum % 10)) % 10
}

// generateCVV generates a random 3-digit CVV
func (s *CardService) generateCVV() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900))
	return fmt.Sprintf("%03d", n.Int64()+100)
}

// generateExpiry generates card expiry date (3 years from now)
func (s *CardService) generateExpiry() string {
	future := time.Now().AddDate(3, 0, 0)
	return future.Format("01/06") // MM/YY
}

// CreateVirtualCard creates a new virtual card
func (s *CardService) CreateVirtualCard(
	userID, accountID uint,
	cardHolderName, currency, billingAddress, nickname string,
) (*models.AccountCard, error) {
	// Verify account belongs to user
	var account models.Account
	if err := s.db.Where("id = ? AND owner_user_id = ?", accountID, userID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("account not found or does not belong to user")
		}
		return nil, err
	}

	// Generate card details
	cardNumber := s.generateCardNumber()
	cvv := s.generateCVV()
	expiry := s.generateExpiry()

	// Create card
	card := &models.AccountCard{
		AccountID:      accountID,
		UserID:         userID,
		CardType:       models.CardTypeVirtual,
		CardHolderName: cardHolderName,
		CardNumber:     cardNumber, // TODO: Encrypt before saving
		CVV:            cvv,        // TODO: Encrypt before saving
		CardExpiry:     expiry,
		Brand:          "Visa",
		IsActive:       true,
		IsDefault:      false,
		CardNickname:   nickname,
		Currency:       currency,
		BillingAddress: billingAddress,
		Status:         "active",
		SpendingLimit:  0, // No limit for virtual cards
		RemainingLimit: 0,
	}

	if err := s.db.Create(card).Error; err != nil {
		return nil, err
	}

	return card, nil
}

// CreateDisposableCard creates a new disposable card with spending limits
func (s *CardService) CreateDisposableCard(
	userID, accountID uint,
	cardHolderName, currency, billingAddress, nickname string,
	spendingLimit float64,
	maxUsageCount, expiresInHours int,
) (*models.AccountCard, error) {
	// Verify account belongs to user
	var account models.Account
	if err := s.db.Where("id = ? AND owner_user_id = ?", accountID, userID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("account not found or does not belong to user")
		}
		return nil, err
	}

	// Validate spending limit
	if spendingLimit <= 0 {
		return nil, errors.New("spending limit must be greater than 0")
	}

	// Generate card details
	cardNumber := s.generateCardNumber()
	cvv := s.generateCVV()
	expiry := s.generateExpiry()

	// Calculate expiration date
	var expiresAt *time.Time
	if expiresInHours > 0 {
		exp := time.Now().Add(time.Duration(expiresInHours) * time.Hour)
		expiresAt = &exp
	}

	// Create card
	card := &models.AccountCard{
		AccountID:      accountID,
		UserID:         userID,
		CardType:       models.CardTypeDisposable,
		CardHolderName: cardHolderName,
		CardNumber:     cardNumber, // TODO: Encrypt before saving
		CVV:            cvv,        // TODO: Encrypt before saving
		CardExpiry:     expiry,
		Brand:          "Visa",
		IsActive:       true,
		IsDefault:      false,
		CardNickname:   nickname,
		Currency:       currency,
		BillingAddress: billingAddress,
		Status:         "active",
		SpendingLimit:  spendingLimit,
		RemainingLimit: spendingLimit,
		MaxUsageCount:  maxUsageCount,
		UsageCount:     0,
		ExpiresAt:      expiresAt,
	}

	if err := s.db.Create(card).Error; err != nil {
		return nil, err
	}

	return card, nil
}

// GetUserCards retrieves all cards for a user with optional filters
func (s *CardService) GetUserCards(userID uint, cardTypeFilter, statusFilter string) ([]models.AccountCard, error) {
	var cards []models.AccountCard
	query := s.db.Where("user_id = ?", userID)

	if cardTypeFilter != "" {
		query = query.Where("card_type = ?", cardTypeFilter)
	}

	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	if err := query.Order("created_at DESC").Find(&cards).Error; err != nil {
		return nil, err
	}

	return cards, nil
}

// GetCardByUUID retrieves a specific card by UUID
func (s *CardService) GetCardByUUID(userID uint, cardUUID string) (*models.AccountCard, error) {
	var card models.AccountCard
	if err := s.db.Where("uuid = ? AND user_id = ?", cardUUID, userID).First(&card).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("card not found")
		}
		return nil, err
	}
	return &card, nil
}

// FreezeCard freezes a card to prevent transactions
func (s *CardService) FreezeCard(userID uint, cardUUID, reason string) (*models.AccountCard, error) {
	card, err := s.GetCardByUUID(userID, cardUUID)
	if err != nil {
		return nil, err
	}

	if card.Status == "frozen" {
		return nil, errors.New("card is already frozen")
	}

	if card.Status == "cancelled" {
		return nil, errors.New("cannot freeze a cancelled card")
	}

	card.Status = "frozen"
	card.FrozenReason = reason

	if err := s.db.Save(card).Error; err != nil {
		return nil, err
	}

	return card, nil
}

// UnfreezeCard unfreezes a card
func (s *CardService) UnfreezeCard(userID uint, cardUUID string) (*models.AccountCard, error) {
	card, err := s.GetCardByUUID(userID, cardUUID)
	if err != nil {
		return nil, err
	}

	if card.Status != "frozen" {
		return nil, errors.New("card is not frozen")
	}

	card.Status = "active"
	card.FrozenReason = ""

	if err := s.db.Save(card).Error; err != nil {
		return nil, err
	}

	return card, nil
}

// CancelCard permanently cancels a card
func (s *CardService) CancelCard(userID uint, cardUUID, reason string) error {
	card, err := s.GetCardByUUID(userID, cardUUID)
	if err != nil {
		return err
	}

	if card.Status == "cancelled" {
		return errors.New("card is already cancelled")
	}

	card.Status = "cancelled"
	card.CancelledReason = reason
	card.IsActive = false

	if err := s.db.Save(card).Error; err != nil {
		return err
	}

	return nil
}

// UpdateCardNickname updates a card's nickname
func (s *CardService) UpdateCardNickname(userID uint, cardUUID, nickname string) (*models.AccountCard, error) {
	card, err := s.GetCardByUUID(userID, cardUUID)
	if err != nil {
		return nil, err
	}

	card.CardNickname = nickname

	if err := s.db.Save(card).Error; err != nil {
		return nil, err
	}

	return card, nil
}

// UpdateCardSpendingLimit updates spending limit for disposable cards
func (s *CardService) UpdateCardSpendingLimit(userID uint, cardUUID string, newLimit float64) (*models.AccountCard, error) {
	card, err := s.GetCardByUUID(userID, cardUUID)
	if err != nil {
		return nil, err
	}

	if card.CardType != models.CardTypeDisposable {
		return nil, errors.New("can only update spending limit for disposable cards")
	}

	if newLimit <= 0 {
		return nil, errors.New("spending limit must be greater than 0")
	}

	// Update both limits
	difference := newLimit - card.SpendingLimit
	card.SpendingLimit = newLimit
	card.RemainingLimit += difference

	if err := s.db.Save(card).Error; err != nil {
		return nil, err
	}

	return card, nil
}

// SetDefaultCard sets a card as the default payment method
func (s *CardService) SetDefaultCard(userID uint, cardUUID string) (*models.AccountCard, error) {
	card, err := s.GetCardByUUID(userID, cardUUID)
	if err != nil {
		return nil, err
	}

	// Begin transaction
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Unset all other default cards for this account
	if err := tx.Model(&models.AccountCard{}).
		Where("account_id = ? AND id != ?", card.AccountID, card.ID).
		Update("is_default", false).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Set this card as default
	card.IsDefault = true
	if err := tx.Save(card).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	tx.Commit()
	return card, nil
}

// GetCardStatistics calculates card statistics for a user
func (s *CardService) GetCardStatistics(userID uint) (map[string]interface{}, error) {
	var stats struct {
		TotalCards          int64
		ActiveCards         int64
		VirtualCards        int64
		DisposableCards     int64
		FrozenCards         int64
		TotalSpendingLimit  float64
		TotalRemainingLimit float64
	}

	// Total cards
	s.db.Model(&models.AccountCard{}).Where("user_id = ?", userID).Count(&stats.TotalCards)

	// Active cards
	s.db.Model(&models.AccountCard{}).Where("user_id = ? AND status = ?", userID, "active").Count(&stats.ActiveCards)

	// Virtual cards
	s.db.Model(&models.AccountCard{}).Where("user_id = ? AND card_type = ?", userID, models.CardTypeVirtual).Count(&stats.VirtualCards)

	// Disposable cards
	s.db.Model(&models.AccountCard{}).Where("user_id = ? AND card_type = ?", userID, models.CardTypeDisposable).Count(&stats.DisposableCards)

	// Frozen cards
	s.db.Model(&models.AccountCard{}).Where("user_id = ? AND status = ?", userID, "frozen").Count(&stats.FrozenCards)

	// Spending limits
	var limitData struct {
		TotalSpendingLimit  float64
		TotalRemainingLimit float64
	}
	s.db.Model(&models.AccountCard{}).
		Where("user_id = ? AND card_type = ?", userID, models.CardTypeDisposable).
		Select("COALESCE(SUM(spending_limit), 0) as total_spending_limit, COALESCE(SUM(remaining_limit), 0) as total_remaining_limit").
		Scan(&limitData)

	stats.TotalSpendingLimit = limitData.TotalSpendingLimit
	stats.TotalRemainingLimit = limitData.TotalRemainingLimit

	return map[string]interface{}{
		"total_cards":           stats.TotalCards,
		"active_cards":          stats.ActiveCards,
		"virtual_cards":         stats.VirtualCards,
		"disposable_cards":      stats.DisposableCards,
		"frozen_cards":          stats.FrozenCards,
		"total_spending_limit":  stats.TotalSpendingLimit,
		"total_remaining_limit": stats.TotalRemainingLimit,
	}, nil
}

// GetCardTransactions retrieves transactions for a specific card
func (s *CardService) GetCardTransactions(userID uint, cardUUID string, page, limit int) ([]models.CardTransaction, int64, error) {
	// Get card first
	card, err := s.GetCardByUUID(userID, cardUUID)
	if err != nil {
		return nil, 0, err
	}

	var transactions []models.CardTransaction
	var total int64

	// Count total transactions
	s.db.Model(&models.CardTransaction{}).Where("card_id = ?", card.ID).Count(&total)

	// Calculate offset
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	// Fetch transactions
	if err := s.db.Where("card_id = ?", card.ID).
		Order("transaction_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error; err != nil {
		return nil, 0, err
	}

	return transactions, total, nil
}

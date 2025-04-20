package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/models" // Keep if needed for other types, maybe remove later
	"lazervaultGo/utils"

	"gorm.io/gorm"
)

var ErrInvalidCardType = errors.New("invalid card type provided")
var ErrCardNotFound = errors.New("card not found or permission denied")
var ErrInvalidCardNumber = errors.New("invalid card number")
var ErrInvalidExpiryFormat = errors.New("invalid expiry format")
var ErrExpiryDateInPast = errors.New("expiry date is in the past")
var ErrEncryptionFailed = errors.New("encryption failed")
var ErrAccountNotFound = errors.New("account not found or permission denied")
var ErrDecryptionFailed = errors.New("failed to decrypt card details")

// IAccountCardService defines the interface for account card operations
type IAccountCardService interface {
	AddAccountCard(ctx context.Context, req AddAccountCardRequest) (*models.AccountCard, error)
	GetAccountCards(ctx context.Context, req GetAccountCardsRequest) ([]models.AccountCard, error)
	UpdateAccountCardDefaultStatus(ctx context.Context, req UpdateAccountCardDefaultStatusRequest) (*models.AccountCard, error)
	DeleteAccountCard(ctx context.Context, req DeleteAccountCardRequest) error
}

// AccountCardService handles business logic for account cards.
type AccountCardService struct {
	db            *gorm.DB
	encryptionKey []byte
	// config        *configs.Config // Keep config if needed for other settings
}

// Ensure implementation satisfies the interface
var _ IAccountCardService = (*AccountCardService)(nil)

// NewAccountCardService creates a new AccountCardService.
func NewAccountCardService(db *gorm.DB, config *configs.Config) IAccountCardService {
	// Use the helper method to get the key bytes
	key, err := config.GetAESKeyBytes()
	if err != nil {
		// Log a fatal error or handle more gracefully - server shouldn't start without a key
		panic(fmt.Sprintf("FATAL: Failed to get encryption key: %v", err))
	}
	return &AccountCardService{
		db:            db,
		encryptionKey: key,
		// config:        config,
	}
}

// AddAccountCardRequest defines parameters for adding a card.
type AddAccountCardRequest struct {
	OwnerUserID    uint   `json:"owner_user_id"`
	AccountID      uint   `json:"account_id"`
	CardHolderName string `json:"card_holder_name"`
	CardNumber     string `json:"card_number"`
	CardExpiry     string `json:"card_expiry"`
	CardType       string `json:"card_type"`
	MakeDefault    bool   `json:"make_default"`
}

// AddAccountCard handles adding a new card to an account.
func (s *AccountCardService) AddAccountCard(ctx context.Context, req AddAccountCardRequest) (*models.AccountCard, error) {
	// 1. Validate card details
	if !utils.IsValidLuhn(req.CardNumber) {
		return nil, ErrInvalidCardNumber
	}
	if !utils.IsValidExpiryFormat(req.CardExpiry) {
		return nil, ErrInvalidExpiryFormat
	}
	if !utils.IsExpiryDateInFuture(req.CardExpiry) {
		return nil, ErrExpiryDateInPast
	}
	// Validate Card Type
	switch req.CardType {
	case models.CardTypeVirtual,
		models.CardTypeDisposable,
		models.CardTypePermanent:
		// Valid type, continue
	default:
		return nil, ErrInvalidCardType
	}

	// 2. Check if account exists and belongs to the user
	var account models.Account // Use the defined Account model
	if err := s.db.WithContext(ctx).Where("id = ? AND owner_user_id = ?", req.AccountID, req.OwnerUserID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("failed to verify account ownership: %w", err)
	}

	// 3. Encrypt sensitive data (Card Number)
	encryptedCardNumber, err := utils.Encrypt([]byte(req.CardNumber), s.encryptionKey)
	if err != nil {
		fmt.Printf("ERROR: Failed to encrypt card number for user %d, account %d: %v\n", req.OwnerUserID, req.AccountID, err)
		return nil, ErrEncryptionFailed
	}

	// 4. Determine card brand and last 4 digits
	brand := utils.DetectCardBrand(req.CardNumber)
	last4 := ""
	if len(req.CardNumber) >= 4 {
		last4 = req.CardNumber[len(req.CardNumber)-4:]
	}

	// 5. Start transaction
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if tx.Error != nil {
			tx.Rollback()
		}
	}()

	// 6. If MakeDefault is true, unset default on other cards for the same account
	if req.MakeDefault {
		if err := tx.Model(&models.AccountCard{}).Where("account_id = ?", req.AccountID).Update("is_default", false).Error; err != nil {
			return nil, fmt.Errorf("failed to unset other default cards: %w", err)
		}
	}

	// 7. Create the AccountCard model
	newCard := models.AccountCard{
		AccountID:      req.AccountID,
		CardHolderName: req.CardHolderName,
		CardNumber:     encryptedCardNumber, // Store encrypted value in CardNumber field
		Brand:          brand,
		Last4:          last4,
		CardExpiry:     req.CardExpiry,
		CardType:       req.CardType,
		IsActive:       true,
		IsDefault:      req.MakeDefault,
	}

	// 8. Save the new card
	if err := tx.Create(&newCard).Error; err != nil {
		return nil, fmt.Errorf("failed to save new card: %w", err)
	}

	// 9. Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Return the card model (sensitive data like encrypted number is not exposed via proto conversion)
	return &newCard, nil
}

// GetAccountCardsRequest defines parameters for getting cards.
type GetAccountCardsRequest struct {
	OwnerUserID uint `json:"owner_user_id"`
	AccountID   uint `json:"account_id"`
}

// GetAccountCards retrieves all non-deleted cards for a specific account owned by the user.
func (s *AccountCardService) GetAccountCards(ctx context.Context, req GetAccountCardsRequest) ([]models.AccountCard, error) {
	// 1. Verify account ownership first
	var account models.Account // Use the defined Account model
	if err := s.db.WithContext(ctx).Select("id").Where("id = ? AND owner_user_id = ?", req.AccountID, req.OwnerUserID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("failed to verify account ownership for GetAccountCards: %w", err)
	}

	// 2. Retrieve cards for the verified account
	var cards []models.AccountCard
	// We select all fields EXCEPT the encrypted number
	if err := s.db.WithContext(ctx).Select("id", "account_id", "card_holder_name", "brand", "last4", "card_expiry", "is_active", "is_default", "created_at", "updated_at", "card_type").Where("account_id = ?", req.AccountID).Find(&cards).Error; err != nil {
		// Don't return ErrRecordNotFound if no cards exist, just an empty slice
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("failed to retrieve account cards: %w", err)
		}
	}

	return cards, nil
}

// UpdateAccountCardDefaultStatusRequest defines parameters for setting default card.
type UpdateAccountCardDefaultStatusRequest struct {
	OwnerUserID uint `json:"owner_user_id"`
	AccountID   uint `json:"account_id"`
	CardID      uint `json:"card_id"`
}

// UpdateAccountCardDefaultStatus sets the specified card as the default for the account.
func (s *AccountCardService) UpdateAccountCardDefaultStatus(ctx context.Context, req UpdateAccountCardDefaultStatusRequest) (*models.AccountCard, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if tx.Error != nil {
			tx.Rollback()
		}
	}()

	// 1. Find the target card, ensuring it belongs to the account and user
	var cardToUpdate models.AccountCard
	if err := tx.Joins("JOIN accounts ON accounts.id = account_cards.account_id").
		Where("account_cards.id = ? AND account_cards.account_id = ? AND accounts.owner_user_id = ?", req.CardID, req.AccountID, req.OwnerUserID).
		First(&cardToUpdate).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCardNotFound // Use the specific error
		}
		return nil, fmt.Errorf("failed to find card to update: %w", err)
	}

	// 2. Unset default status on other cards for the same account
	if err := tx.Model(&models.AccountCard{}).
		Where("account_id = ? AND id != ?", req.AccountID, req.CardID).
		Update("is_default", false).Error; err != nil {
		return nil, fmt.Errorf("failed to unset other default cards: %w", err)
	}

	// 3. Set the target card as default
	if err := tx.Model(&cardToUpdate).Update("is_default", true).Error; err != nil {
		return nil, fmt.Errorf("failed to set target card as default: %w", err)
	}

	// 4. Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Reload the updated card to get the latest state (optional but good practice)
	if err := s.db.WithContext(ctx).First(&cardToUpdate, req.CardID).Error; err != nil {
		// Log error, but proceed as the update likely succeeded
		fmt.Printf("Warning: Failed to reload card %d after update: %v\n", req.CardID, err)
	}

	return &cardToUpdate, nil
}

// DeleteAccountCardRequest defines parameters for deleting a card.
type DeleteAccountCardRequest struct {
	OwnerUserID uint `json:"owner_user_id"`
	AccountID   uint `json:"account_id"`
	CardID      uint `json:"card_id"`
}

// DeleteAccountCard removes a card from an account.
func (s *AccountCardService) DeleteAccountCard(ctx context.Context, req DeleteAccountCardRequest) error {
	// 1. Find the target card, ensuring it belongs to the account and user
	// Use a transaction for potential future logic (like setting new default)
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if tx.Error != nil {
			tx.Rollback()
		}
	}()

	var cardToDelete models.AccountCard
	if err := tx.Joins("JOIN accounts ON accounts.id = account_cards.account_id").
		Where("account_cards.id = ? AND account_cards.account_id = ? AND accounts.owner_user_id = ?", req.CardID, req.AccountID, req.OwnerUserID).
		First(&cardToDelete).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCardNotFound
		}
		return fmt.Errorf("failed to find card to delete: %w", err)
	}

	// 2. Perform the delete (soft delete)
	if err := tx.Delete(&cardToDelete).Error; err != nil {
		return fmt.Errorf("failed to delete card: %w", err)
	}

	// 3. TODO: Handle setting a new default if the deleted card was the default.
	// This might involve finding another card on the account and calling UpdateDefaultStatus.
	if cardToDelete.IsDefault {
		fmt.Printf("INFO: Deleted card %d was the default for account %d. Need logic to set a new default.\n", req.CardID, req.AccountID)
		// Add logic here to find and set a new default if necessary
	}

	// 4. Commit transaction
	return tx.Commit().Error
}

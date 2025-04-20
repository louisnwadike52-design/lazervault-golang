package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"

	"gorm.io/gorm"
)

// --- Recipient Service Errors ---
var ErrRecipientNotFound = errors.New("recipient not found or permission denied")

// --- Recipient Service Interface ---

// IRecipientService defines the interface for recipient operations
type IRecipientService interface {
	AddRecipient(ctx context.Context, req AddRecipientRequest) (*models.Recipient, error)
	GetRecipients(ctx context.Context, ownerUserID uint, onlyFavorites bool) ([]models.Recipient, error)
	UpdateRecipientFavoriteStatus(ctx context.Context, req UpdateRecipientFavoriteStatusRequest) (*models.Recipient, error)
}

// --- Recipient Service Struct ---

// RecipientService handles business logic related to recipients.
type RecipientService struct {
	db *gorm.DB
}

// --- Recipient Service Constructor ---

// NewRecipientService creates a new RecipientService.
func NewRecipientService(db *gorm.DB) IRecipientService { // Return interface type
	return &RecipientService{db: db}
}

// --- Recipient Service Types ---

// AddRecipientRequest defines parameters for adding a recipient.
type AddRecipientRequest struct {
	OwnerUserID   uint   `json:"owner_user_id" binding:"required"`
	Name          string `json:"name" binding:"required"`
	AccountNumber string `json:"account_number" binding:"required"`
	SortCode      string `json:"sort_code"`
	BankName      string `json:"bank_name" binding:"required"`
	IsFavorite    bool   `json:"is_favorite"`
}

// --- Recipient Service Methods ---

// AddRecipient creates a new recipient for a user.
func (s *RecipientService) AddRecipient(ctx context.Context, req AddRecipientRequest) (*models.Recipient, error) {
	recipient := models.Recipient{
		OwnerUserID:   req.OwnerUserID,
		Name:          req.Name,
		AccountNumber: req.AccountNumber,
		SortCode:      req.SortCode,
		BankName:      req.BankName,
		IsFavorite:    req.IsFavorite,
	}

	// TODO: Add validation (e.g., check if recipient with same details already exists for user)

	result := s.db.WithContext(ctx).Create(&recipient)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to create recipient: %w", result.Error)
	}

	return &recipient, nil
}

// GetRecipients retrieves recipients for a user, optionally filtering by favorite status.
func (s *RecipientService) GetRecipients(ctx context.Context, ownerUserID uint, onlyFavorites bool) ([]models.Recipient, error) {
	var recipients []models.Recipient

	query := s.db.WithContext(ctx).Where("owner_user_id = ?", ownerUserID)

	if onlyFavorites {
		query = query.Where("is_favorite = ?", true)
	}

	result := query.Order("name ASC").Find(&recipients)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to retrieve recipients: %w", result.Error)
	}

	return recipients, nil
}

// UpdateRecipientFavoriteStatusRequest defines parameters for updating favorite status.
type UpdateRecipientFavoriteStatusRequest struct {
	OwnerUserID uint `json:"owner_user_id" binding:"required"`
	RecipientID uint `json:"recipient_id" binding:"required"`
	IsFavorite  bool `json:"is_favorite"` // Use a pointer if you need to distinguish between false and not provided? For a toggle, bool is fine.
}

// UpdateRecipientFavoriteStatus updates the favorite status of a specific recipient.
func (s *RecipientService) UpdateRecipientFavoriteStatus(ctx context.Context, req UpdateRecipientFavoriteStatusRequest) (*models.Recipient, error) {
	var recipient models.Recipient

	// Find the specific recipient ensuring it belongs to the requesting user
	result := s.db.WithContext(ctx).Where("id = ? AND owner_user_id = ?", req.RecipientID, req.OwnerUserID).First(&recipient)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecipientNotFound
		}
		return nil, fmt.Errorf("failed to find recipient: %w", result.Error)
	}

	// Update the favorite status
	updateResult := s.db.WithContext(ctx).Model(&recipient).Update("is_favorite", req.IsFavorite)
	if updateResult.Error != nil {
		return nil, fmt.Errorf("failed to update recipient favorite status: %w", updateResult.Error)
	}

	// Return the updated recipient model
	return &recipient, nil
}

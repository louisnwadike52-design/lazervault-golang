package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// --- Service Errors ---
var (
	ErrRecipientNotFound       = errors.New("recipient service: recipient not found")
	ErrRecipientAccessDenied   = errors.New("recipient service: access denied to recipient")
	ErrRecipientCreateFailed   = errors.New("recipient service: failed to create recipient")
	ErrRecipientUpdateFailed   = errors.New("recipient service: failed to update recipient")
	ErrRecipientDeleteFailed   = errors.New("recipient service: failed to delete recipient")
	ErrInvalidRecipientType    = errors.New("recipient service: invalid recipient type specified")
	ErrMissingInternalID       = errors.New("recipient service: internal_account_id required for internal recipient")
	ErrMissingExternalDetails  = errors.New("recipient service: account number and bank name required for external recipient")
	ErrInternalAccountNotFound = errors.New("recipient service: linked internal account not found")
)

// --- Service Interface ---
type IRecipientService interface {
	CreateRecipient(ctx context.Context, userID uint, req *pb.CreateRecipientRequest) (*models.Recipient, error)
	ListRecipients(ctx context.Context, userID uint) ([]*models.Recipient, error)
	UpdateRecipient(ctx context.Context, userID uint, req *pb.UpdateRecipientRequest) (*models.Recipient, error)
	DeleteRecipient(ctx context.Context, userID uint, recipientID uint) error
	GetRecipientByID(ctx context.Context, recipientID uint, requestingUserID uint) (*models.Recipient, error)
}

// --- Service Struct ---
type RecipientService struct {
	db *gorm.DB
}

// --- Constructor ---
func NewRecipientService(db *gorm.DB) IRecipientService {
	return &RecipientService{db: db}
}

// --- Methods ---

func (s *RecipientService) CreateRecipient(ctx context.Context, userID uint, req *pb.CreateRecipientRequest) (*models.Recipient, error) {
	recipientType := strings.ToLower(req.GetType()) // Get type, convert to lowercase
	if recipientType == "" {
		recipientType = "external" // Default to external if not provided
	}

	recipient := models.Recipient{
		OwnerUserID: userID,
		Name:        req.GetName(),
		IsFavorite:  req.GetIsFavorite(),
		Type:        recipientType,
	}

	// Validate and populate based on type
	switch recipientType {
	case "internal":
		internalAccountID := uint(req.GetInternalAccountId())
		if internalAccountID == 0 {
			return nil, ErrMissingInternalID
		}

		// Check if the internal account exists and belongs to someone (doesn't have to be the creator)
		var targetAccount models.Account
		if err := s.db.WithContext(ctx).Where("id = ?", internalAccountID).First(&targetAccount).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w: %d", ErrInternalAccountNotFound, internalAccountID)
			}
			return nil, fmt.Errorf("db error checking internal account: %w", err)
		}

		recipient.InternalAccountID = &internalAccountID
		recipient.InternalUserID = &targetAccount.OwnerUserID // Store owner ID for convenience
		// Clear external fields for internal type
		recipient.AccountNumber = ""
		recipient.SortCode = ""
		recipient.BankName = ""
		recipient.CountryCode = ""

	case "external":
		accountNumber := req.GetAccountNumber()
		bankName := req.GetBankName()
		if accountNumber == "" || bankName == "" {
			return nil, ErrMissingExternalDetails
		}
		recipient.AccountNumber = accountNumber
		recipient.SortCode = req.GetSortCode() // Optional
		recipient.BankName = bankName
		// recipient.CountryCode = req.GetCountryCode() // Add if you have this field
		// Clear internal fields for external type
		recipient.InternalAccountID = nil
		recipient.InternalUserID = nil

	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidRecipientType, req.GetType())
	}

	if err := s.db.WithContext(ctx).Create(&recipient).Error; err != nil {
		// TODO: Handle potential duplicate errors based on unique constraints if any
		return nil, fmt.Errorf("%w: %v", ErrRecipientCreateFailed, err)
	}
	return &recipient, nil
}

func (s *RecipientService) ListRecipients(ctx context.Context, userID uint) ([]*models.Recipient, error) {
	var recipients []*models.Recipient
	if err := s.db.WithContext(ctx).Where("owner_user_id = ?", userID).Order("is_favorite DESC, name ASC").Find(&recipients).Error; err != nil {
		return nil, fmt.Errorf("db error listing recipients: %w", err)
	}
	return recipients, nil
}

func (s *RecipientService) UpdateRecipient(ctx context.Context, userID uint, req *pb.UpdateRecipientRequest) (*models.Recipient, error) {
	recipientID := uint(req.GetRecipientId())

	var recipient models.Recipient
	// Find the recipient and verify ownership
	if err := s.db.WithContext(ctx).Where("id = ? AND owner_user_id = ?", recipientID, userID).First(&recipient).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecipientNotFound
		}
		return nil, fmt.Errorf("db error finding recipient: %w", err)
	}

	// Apply updates from request (only if fields are present in proto request)
	updated := false
	if req.Name != nil {
		recipient.Name = *req.Name
		updated = true
	}
	if req.AccountNumber != nil {
		recipient.AccountNumber = *req.AccountNumber
		updated = true
	}
	if req.SortCode != nil {
		recipient.SortCode = *req.SortCode
		updated = true
	}
	if req.BankName != nil {
		recipient.BankName = *req.BankName
		updated = true
	}
	if req.IsFavorite != nil {
		recipient.IsFavorite = *req.IsFavorite
		updated = true
	}

	if !updated {
		return &recipient, nil // No changes to apply
	}

	if err := s.db.WithContext(ctx).Save(&recipient).Error; err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecipientUpdateFailed, err)
	}

	return &recipient, nil
}

func (s *RecipientService) DeleteRecipient(ctx context.Context, userID uint, recipientID uint) error {
	result := s.db.WithContext(ctx).Where("id = ? AND owner_user_id = ?", recipientID, userID).Delete(&models.Recipient{})
	if result.Error != nil {
		return fmt.Errorf("%w: %v", ErrRecipientDeleteFailed, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecipientNotFound // Or AccessDenied, depending on which is more likely
	}
	return nil
}

func (s *RecipientService) GetRecipientByID(ctx context.Context, recipientID uint, requestingUserID uint) (*models.Recipient, error) {
	var recipient models.Recipient
	err := s.db.WithContext(ctx).Where("id = ?", recipientID).First(&recipient).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecipientNotFound
		}
		return nil, fmt.Errorf("db error finding recipient: %w", err)
	}

	// IMPORTANT: Check if the user requesting the details actually owns this recipient record
	if recipient.OwnerUserID != requestingUserID {
		return nil, ErrRecipientAccessDenied
	}

	// Optionally preload internal account details if needed immediately
	// err = s.db.WithContext(ctx).Preload("InternalAccount").First(&recipient, recipientID).Error
	// Handle preload errors if necessary

	return &recipient, nil
}

// Helper to convert Recipient model to proto message (consider placing in controller or shared converter)
func ConvertRecipientToProto(r *models.Recipient) *pb.Recipient {
	if r == nil {
		return nil
	}
	return &pb.Recipient{
		Id:            uint64(r.ID),
		Name:          r.Name,
		AccountNumber: r.AccountNumber, // Mask sensitive details?
		SortCode:      r.SortCode,
		BankName:      r.BankName,
		IsFavorite:    r.IsFavorite,
		CreatedAt:     timestamppb.New(r.CreatedAt),
		UpdatedAt:     timestamppb.New(r.UpdatedAt),
	}
}

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
	GetSimilarRecipientsByName(ctx context.Context, name string, userID uint) ([]*models.Recipient, error)
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
		IsFavorite:  req.GetIsFavorite(), // Defaults to false if not set
		Type:        recipientType,
	}

	// Validate and populate based on type
	switch recipientType {
	case "internal":
		internalAccountID := uint(req.GetInternalAccountId()) // GetInternalAccountId returns 0 if not set
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
		// Get external fields directly from the request
		accountNumber := req.GetAccountNumber() // Returns "" if not set
		bankName := req.GetBankName()           // Returns "" if not set
		if accountNumber == "" || bankName == "" {
			return nil, ErrMissingExternalDetails
		}
		recipient.AccountNumber = accountNumber
		recipient.BankName = bankName
		recipient.SortCode = req.GetSortCode()       // Optional, defaults to ""
		recipient.CountryCode = req.GetCountryCode() // Optional, defaults to ""

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

	// Apply updates from request using wrappers
	updated := false
	if req.Name != nil { // Check if the wrapper message itself is present
		recipient.Name = req.GetName().GetValue() // Get value from wrapper
		updated = true
	}
	if req.IsFavorite != nil {
		recipient.IsFavorite = req.GetIsFavorite().GetValue()
		updated = true
	}

	// Only update external fields if the recipient is external
	if recipient.Type == "external" {
		if req.AccountNumber != nil {
			recipient.AccountNumber = req.GetAccountNumber().GetValue()
			updated = true
		}
		if req.SortCode != nil {
			recipient.SortCode = req.GetSortCode().GetValue()
			updated = true
		}
		if req.BankName != nil {
			recipient.BankName = req.GetBankName().GetValue()
			updated = true
		}
		if req.CountryCode != nil {
			recipient.CountryCode = req.GetCountryCode().GetValue()
			updated = true
		}
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

func (s *RecipientService) GetSimilarRecipientsByName(ctx context.Context, name string, userID uint) ([]*models.Recipient, error) {
	var recipients []*models.Recipient
	searchTerm := "%" + strings.ToLower(name) + "%"

	// Search term against the Name field of the recipients table, scoped to the OwnerUserID.
	err := s.db.WithContext(ctx).
		Select("id", "name"). // Select only ID and Name as per new requirement for response
		Where("owner_user_id = ? AND LOWER(name) ILIKE ?", userID, searchTerm).
		Limit(10). // Limit results to a reasonable number
		Find(&recipients).Error

	if err != nil {
		return nil, fmt.Errorf("database error searching recipients by name: %w", err)
	}

	return recipients, nil
}

// Helper to convert Recipient model to proto message
func ConvertRecipientToProto(r *models.Recipient) *pb.Recipient {
	if r == nil {
		return nil
	}
	protoRecipient := &pb.Recipient{
		Id:            uint64(r.ID),
		Name:          r.Name,
		IsFavorite:    r.IsFavorite,
		Type:          r.Type,
		AccountNumber: r.AccountNumber, // Populate external fields
		SortCode:      r.SortCode,
		BankName:      r.BankName,
		CountryCode:   r.CountryCode,
		CreatedAt:     timestamppb.New(r.CreatedAt),
		UpdatedAt:     timestamppb.New(r.UpdatedAt),
	}

	// Populate internal fields only if they are not nil in the model
	if r.InternalAccountID != nil {
		internalAccountId := uint64(*r.InternalAccountID)
		protoRecipient.InternalAccountId = &internalAccountId
	}
	if r.InternalUserID != nil {
		internalUserId := uint64(*r.InternalUserID)
		protoRecipient.InternalUserId = &internalUserId
	}

	return protoRecipient
}

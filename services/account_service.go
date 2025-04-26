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

// --- Account Service Errors ---
var (
	ErrSvcAccountNotFound      = errors.New("account service: account not found")
	ErrSvcAccountAccessDenied  = errors.New("account service: user does not own this account")
	ErrSvcInvalidAccountStatus = errors.New("account service: invalid account status value")
)

// --- Account Service Interface ---

// IAccountService defines the interface for account operations
type IAccountService interface {
	GetAccountsByUserID(ctx context.Context, userID uint) ([]*pb.AccountSummary, error)
	GetAccountDetails(ctx context.Context, userID uint, accountID uint) (*pb.AccountDetails, error)
	CreateAccount(ctx context.Context, userID uint, req *pb.CreateAccountRequest) (*models.Account, error)
	UpdateAccountStatus(ctx context.Context, userID uint, accountID uint, status string, reason string) (*models.Account, error)
	UpdateSecuritySettings(ctx context.Context, userID uint, accountID uint, settings *pb.SecuritySettings) (*models.Account, error)
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

// --- Helper Functions ---

// MaskAccountNumber hides parts of the account number for display.
func MaskAccountNumber(fullNumber string) string {
	if len(fullNumber) > 4 {
		return "•••• " + fullNumber[len(fullNumber)-4:]
	}
	return "••••"
}

// ConvertAccountToProtoDetails converts a GORM model to Protobuf details.
// Renamed to be public.
func ConvertAccountToProtoDetails(acc *models.Account) *pb.AccountDetails {
	// Handle potential nil pointers for IBAN and BICSwift
	iban := ""
	if acc.IBAN != nil {
		iban = *acc.IBAN
	}
	bicSwift := ""
	if acc.BICSwift != nil {
		bicSwift = *acc.BICSwift
	}

	return &pb.AccountDetails{
		Id:                   uint64(acc.ID),
		AccountType:          acc.AccountType,
		Currency:             acc.Currency,
		Balance:              acc.Balance,
		Status:               acc.Status,
		CardHolderName:       acc.CardHolderName,
		CardType:             acc.CardType,
		ExpiryDate:           acc.ExpiryDate,
		DailyLimit:           acc.DailyLimit,
		MonthlyLimit:         acc.MonthlyLimit,
		Enable_3DSecure:      acc.Enable3DSecure,       // Field name uses snake_case in proto
		EnableContactless:    acc.EnableContactless,    // Check generated proto field names if needed
		EnableOnlinePayments: acc.EnableOnlinePayments, // Check generated proto field names if needed
		AccountNumber:        acc.AccountNumber,        // Full number for details
		Iban:                 iban,                     // Use handled value
		BicSwift:             bicSwift,                 // Use handled value
		CreatedAt:            timestamppb.New(acc.CreatedAt),
		UpdatedAt:            timestamppb.New(acc.UpdatedAt),
	}
}

// --- Account Service Methods ---

// GetAccountsByUserID retrieves a summary list of accounts for a user.
func (s *AccountService) GetAccountsByUserID(ctx context.Context, userID uint) ([]*pb.AccountSummary, error) {
	var accounts []*models.Account
	if err := s.db.WithContext(ctx).Where("owner_user_id = ?", userID).Find(&accounts).Error; err != nil {
		return nil, fmt.Errorf("db error finding accounts: %w", err)
	}

	var summaries []*pb.AccountSummary
	for _, acc := range accounts {
		summaries = append(summaries, &pb.AccountSummary{
			Id:                  uint64(acc.ID),
			AccountType:         acc.AccountType,
			Currency:            acc.Currency,
			Balance:             acc.Balance,
			MaskedAccountNumber: MaskAccountNumber(acc.AccountNumber),
			Status:              acc.Status,
		})
	}

	return summaries, nil
}

// GetAccountDetails retrieves detailed information for a specific account owned by the user.
func (s *AccountService) GetAccountDetails(ctx context.Context, userID uint, accountID uint) (*pb.AccountDetails, error) {
	var account models.Account
	if err := s.db.WithContext(ctx).Where("id = ?", accountID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSvcAccountNotFound
		}
		return nil, fmt.Errorf("db error finding account: %w", err)
	}

	// Verify ownership
	if account.OwnerUserID != userID {
		return nil, ErrSvcAccountAccessDenied
	}

	return ConvertAccountToProtoDetails(&account), nil
}

// CreateAccount creates a new account record in the database.
// Typically called internally, e.g., during user signup.
func (s *AccountService) CreateAccount(ctx context.Context, userID uint, req *pb.CreateAccountRequest) (*models.Account, error) {
	account := models.Account{
		OwnerUserID: userID,
		AccountType: req.GetAccountType(),
		Currency:    req.GetCurrency(),
		Status:      models.AccountStatusActive, // Default to active
		// BeforeCreate hook will set CardHolderName, ExpiryDate, AccountNumber
	}

	if err := s.db.WithContext(ctx).Create(&account).Error; err != nil {
		// TODO: Handle potential unique constraint violations (e.g., AccountNumber)
		return nil, fmt.Errorf("failed to create account record: %w", err)
	}

	return &account, nil
}

// UpdateAccountStatus updates the status of a specific account owned by the user.
func (s *AccountService) UpdateAccountStatus(ctx context.Context, userID uint, accountID uint, status string, reason string) (*models.Account, error) {
	// Validate status
	validStatuses := map[string]bool{
		models.AccountStatusActive:           true,
		models.AccountStatusBlockedTemporary: true,
		models.AccountStatusBlockedPermanent: true,
		models.AccountStatusBlockedStolen:    true,
	}
	if !validStatuses[status] {
		return nil, fmt.Errorf("%w: %s", ErrSvcInvalidAccountStatus, status)
	}

	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	var account models.Account
	// Lock the row for update
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", accountID).First(&account).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSvcAccountNotFound
		}
		return nil, fmt.Errorf("db error finding account for update: %w", err)
	}

	// Verify ownership
	if account.OwnerUserID != userID {
		tx.Rollback()
		return nil, ErrSvcAccountAccessDenied
	}

	// Prevent changing status from permanent blocks
	if strings.HasPrefix(account.Status, "blocked_permanent") || account.Status == models.AccountStatusBlockedStolen {
		if account.Status != status { // Allow idempotency
			tx.Rollback()
			return nil, fmt.Errorf("cannot change status from %s", account.Status)
		}
	}

	// Update status
	account.Status = status
	// TODO: Potentially log the reason somewhere if needed

	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update account status: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &account, nil
}

// UpdateSecuritySettings updates the security flags for a specific account.
func (s *AccountService) UpdateSecuritySettings(ctx context.Context, userID uint, accountID uint, settings *pb.SecuritySettings) (*models.Account, error) {
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	var account models.Account
	// Lock the row for update
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", accountID).First(&account).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSvcAccountNotFound
		}
		return nil, fmt.Errorf("db error finding account for update: %w", err)
	}

	// Verify ownership
	if account.OwnerUserID != userID {
		tx.Rollback()
		return nil, ErrSvcAccountAccessDenied
	}

	// Apply updates
	account.Enable3DSecure = settings.GetEnable_3DSecure()
	account.EnableContactless = settings.GetEnableContactless()
	account.EnableOnlinePayments = settings.GetEnableOnlinePayments()

	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update account security settings: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &account, nil
}

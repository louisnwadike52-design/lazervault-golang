package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/tasks"
	"lazervaultGo/utils"
	"regexp"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	// bcrypt should be here if used by HashPassword
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// --- Account Service Errors ---
var (
	ErrSvcAccountNotFound       = errors.New("account service: account not found")
	ErrSvcAccountAccessDenied   = errors.New("account service: user does not own this account")
	ErrSvcInvalidAccountStatus  = errors.New("account service: invalid account status value")
	ErrSvcInvalidPINFormat      = errors.New("account service: PIN must be exactly 4 digits")
	ErrSvcPINHashingFailed      = errors.New("account service: failed to hash PIN")
	ErrSvcAccountCreationFailed = errors.New("account service: failed to create account record")
	ErrSvcAccountTypeExists     = errors.New("account service: account type already exists for this user")
	ErrSvcInsufficientFunds     = errors.New("account service: insufficient funds")
)

// --- Account Service Interface ---

// IAccountService defines the interface for account operations
type IAccountService interface {
	GetUserAccounts(ctx context.Context, userID uint) ([]*pb.AccountSummary, error)
	GetAccountDetails(ctx context.Context, userID uint, accountID uint) (*pb.AccountDetails, error)
	CreateAccount(ctx context.Context, userID uint, req *pb.CreateAccountRequest) (*models.Account, error)
	UpdateAccountStatus(ctx context.Context, userID uint, accountID uint, status string, reason string) (*models.Account, error)
	UpdateSecuritySettings(ctx context.Context, userID uint, accountID uint, settings *pb.SecuritySettings) (*models.Account, error)
	CheckAccountOwnership(ctx context.Context, accountID uint, userID uint) error
	CheckSufficientBalance(ctx context.Context, tx *gorm.DB, accountID uint, requiredAmount int64) error
}

// --- Account Service Struct ---

// AccountService handles business logic related to user accounts.
type AccountService struct {
	db          *gorm.DB
	distributor tasks.TaskDistributor
}

// --- Account Service Constructor ---

// NewAccountService creates a new AccountService.
func NewAccountService(db *gorm.DB, distributor tasks.TaskDistributor) IAccountService { // Return interface type
	return &AccountService{
		db:          db,
		distributor: distributor,
	}
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
func ConvertAccountToProtoDetails(acc *models.Account) *pb.AccountDetails {
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
		Balance:              uint64(acc.Balance),
		Status:               acc.Status,
		CardHolderName:       acc.CardHolderName,
		CardType:             acc.CardType,
		ExpiryDate:           acc.ExpiryDate,
		DailyLimit:           uint64(acc.DailyLimit),
		MonthlyLimit:         uint64(acc.MonthlyLimit),
		Enable_3DSecure:      acc.Enable3DSecure,
		EnableContactless:    acc.EnableContactless,
		EnableOnlinePayments: acc.EnableOnlinePayments,
		AccountNumber:        acc.AccountNumber,
		Iban:                 iban,
		BicSwift:             bicSwift,
		CreatedAt:            timestamppb.New(acc.CreatedAt),
		UpdatedAt:            timestamppb.New(acc.UpdatedAt),
	}
}

// --- Account Service Methods ---

// GetUserAccounts retrieves a summary list of accounts for a user.
func (s *AccountService) GetUserAccounts(ctx context.Context, userID uint) ([]*pb.AccountSummary, error) {
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
			Balance:             uint64(acc.Balance),
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

// isFourDigitPIN checks if a string is exactly four digits.
var isFourDigitPIN = regexp.MustCompile(`^[0-9]{4}$`).MatchString

// CreateAccount creates a new account record in the database.
func (s *AccountService) CreateAccount(ctx context.Context, userID uint, req *pb.CreateAccountRequest) (*models.Account, error) {
	// TODO: Add validation for currency and account type against allowed values.
	accountType := req.GetAccountType()

	// --- Check if account type already exists for this user ---
	var existingAccount models.Account
	err := s.db.WithContext(ctx).Where("owner_user_id = ? AND LOWER(account_type) = ?", userID, strings.ToLower(accountType)).First(&existingAccount).Error
	if err == nil {
		// Found an existing account of the same type for this user
		return nil, ErrSvcAccountTypeExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// An actual DB error occurred during the check
		return nil, fmt.Errorf("db error checking for existing account type: %w", err)
	}
	// --- End Check ---

	account := models.Account{
		OwnerUserID: userID,
		AccountType: accountType, // Use validated/requested account type
		Currency:    req.GetCurrency(),
		Status:      models.AccountStatusActive, // Default to active
		// BeforeCreate hook will set CardHolderName, ExpiryDate, AccountNumber
	}

	// Handle optional PIN
	if req.Pin != nil { // Check if optional field is set
		pin := req.GetPin() // Get the value
		if !isFourDigitPIN(pin) {
			return nil, ErrSvcInvalidPINFormat
		}
		hashedPIN, err := utils.HashPassword(pin) // Assuming a HashPassword util exists
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSvcPINHashingFailed, err)
		}
		account.PINHash = hashedPIN // Assign the hash to the model field
	}

	if err := s.db.WithContext(ctx).Create(&account).Error; err != nil {
		// TODO: Handle potential unique constraint violations (e.g., AccountNumber)
		return nil, fmt.Errorf("%w: %v", ErrSvcAccountCreationFailed, err)
	}

	// Enqueue Tx File Update Task (AFTER successful creation)
	txFilePayloadBytes, err := tasks.NewGenerateTxDataFileTask(userID)
	if err != nil {
		fmt.Printf("CRITICAL ERROR: Failed creating tx file generation payload for user %d after account creation %d: %v\n", userID, account.ID, err)
	} else {
		opts := []asynq.Option{
			asynq.MaxRetry(3),
			asynq.Timeout(10 * time.Minute),
			asynq.Queue(tasks.QueueLow),
		}
		if err := s.distributor.DistributeTask(ctx, tasks.TypeGenerateTxDataFile, txFilePayloadBytes, opts...); err != nil {
			fmt.Printf("CRITICAL ERROR: Failed enqueuing tx file generation task for user %d after account creation %d: %v\n", userID, account.ID, err)
		}
	}

	// Reload the account to get all fields populated by hooks/db defaults
	// Use the ID from the created account object
	if err := s.db.WithContext(ctx).First(&account, account.ID).Error; err != nil {
		// Log this error but potentially return the partially populated account
		// Or handle it more robustly depending on requirements
		fmt.Printf("Warning: failed to reload created account %d: %v\n", account.ID, err)
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

// CheckAccountOwnership verifies if the user owns the specified account.
func (s *AccountService) CheckAccountOwnership(ctx context.Context, accountID uint, userID uint) error {
	var account models.Account
	if err := s.db.WithContext(ctx).Select("owner_user_id").Where("id = ?", accountID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSvcAccountNotFound
		}
		return fmt.Errorf("db error checking account ownership: %w", err)
	}

	if account.OwnerUserID != userID {
		return ErrSvcAccountAccessDenied
	}
	return nil
}

// CheckSufficientBalance checks if an account has enough balance, performing the check within a transaction.
func (s *AccountService) CheckSufficientBalance(ctx context.Context, tx *gorm.DB, accountID uint, requiredAmount int64) error {
	if tx == nil {
		return errors.New("transaction object is required for balance check")
	}

	var account models.Account
	// Lock the row within the transaction to prevent race conditions
	if err := tx.WithContext(ctx).Set("gorm:query_option", "FOR UPDATE").Select("balance").Where("id = ?", accountID).First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSvcAccountNotFound // Account not found during balance check
		}
		return fmt.Errorf("db error locking account for balance check: %w", err)
	}

	if account.Balance < requiredAmount {
		return ErrSvcInsufficientFunds
	}
	return nil
}

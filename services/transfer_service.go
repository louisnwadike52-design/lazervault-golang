package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/tasks"
	"time"

	"github.com/hibiken/asynq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

var (
	ErrTransferFromAccountNotFound = errors.New("transfer service: source account not found or invalid")
	ErrTransferToAccountNotFound   = errors.New("transfer service: destination account not found")
	ErrTransferAccountLookupFailed = errors.New("transfer service: failed to look up accounts")
	ErrCannotTransferToSelfAccount = errors.New("transfer service: cannot transfer to the same account")
	ErrTransferNotFound            = errors.New("transfer service: transfer not found")
	ErrTransferAccessDenied        = errors.New("transfer service: access denied to transfer details")
	// Errors are defined in their respective service packages (account, recipient)
)

type ITransferService interface {
	InitiateTransfer(ctx context.Context, fromUserID uint, req TransferRequest) (*TransferResponse, error)
	GetTransferDetails(ctx context.Context, transferID uint, userID uint) (*models.Transfer, error)
}

type TransferService struct {
	db               *gorm.DB
	config           *configs.Config
	redisWorker      tasks.TaskDistributor
	recipientService IRecipientService
	accountService   IAccountService
}

type TransferRequest struct {
	FromUserID uint // From auth context

	// Input fields matching refined proto
	FromAccountID uint       `json:"from_account_id" validate:"required"`
	Amount        int64      `json:"amount" validate:"required,gt=0"`
	Reference     string     `json:"reference"`
	Category      string     `json:"category"`
	ScheduledAt   *time.Time `json:"scheduled_at"` // Optional

	// --- Destination (Use ONE) ---
	ToAccountID *uint `json:"to_account_id"` // Optional: Direct internal account ID
	RecipientID *uint `json:"recipient_id"`  // Optional: ID of the saved recipient
}

type TransferResponse struct {
	TransferID  uint      `json:"transfer_id"`
	Status      string    `json:"status"`
	Amount      int64     `json:"amount"`
	Fee         int64     `json:"fee"`
	TotalAmount int64     `json:"total_amount"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewTransferService(db *gorm.DB, config *configs.Config, redisWorker tasks.TaskDistributor, recipientService IRecipientService, accountService IAccountService) *TransferService {
	return &TransferService{
		db:               db,
		config:           config,
		redisWorker:      redisWorker,
		recipientService: recipientService,
		accountService:   accountService,
	}
}

func (s *TransferService) InitiateTransfer(ctx context.Context, fromUserID uint, req TransferRequest) (*TransferResponse, error) {
	// --- Basic Validation ---
	if req.FromAccountID == 0 {
		return nil, status.Error(codes.InvalidArgument, "from_account_id is required")
	}
	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	// Validate that EXACTLY ONE destination type is provided
	destinationProvided := false
	if req.ToAccountID != nil && *req.ToAccountID > 0 {
		destinationProvided = true
	}
	if req.RecipientID != nil && *req.RecipientID > 0 {
		if destinationProvided {
			// Both were provided
			return nil, status.Error(codes.InvalidArgument, "provide either to_account_id OR recipient_id, not both")
		}
		destinationProvided = true
	}
	if !destinationProvided {
		return nil, status.Error(codes.InvalidArgument, "either to_account_id OR recipient_id must be provided")
	}

	var fromCurrency string
	var toCurrency string
	var toAccountID_ptr *uint // Use pointers for model
	var toUserID_ptr *uint    // Use pointers for model
	var recipientID_ptr *uint // Use pointers for model
	var isExternal bool = false

	// --- Start Transaction ---
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil || tx.Error != nil {
			tx.Rollback()
		}
	}()

	// --- Get From Account & Validate Ownership ---
	var fromAccount models.Account
	err := s.accountService.CheckAccountOwnership(ctx, req.FromAccountID, fromUserID)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, ErrSvcAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, "source account %d not found", req.FromAccountID)
		} else if errors.Is(err, ErrSvcAccountAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied to source account %d", req.FromAccountID)
		}
		return nil, fmt.Errorf("source account validation failed: %w", err)
	}
	if err := tx.Where("id = ?", req.FromAccountID).First(&fromAccount).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to fetch source account details: %w", err)
	}
	fromCurrency = fromAccount.Currency

	// --- Handle Destination Logic (Either ToAccountID or RecipientID) ---

	if req.ToAccountID != nil && *req.ToAccountID > 0 {
		// Scenario 1: Direct Internal Transfer via ToAccountID
		internalToAccountID := *req.ToAccountID
		isExternal = false
		recipientID_ptr = nil // No recipient involved

		if req.FromAccountID == internalToAccountID {
			tx.Rollback()
			return nil, ErrCannotTransferToSelfAccount
		}

		var toAccount models.Account
		if err := tx.Where("id = ?", internalToAccountID).First(&toAccount).Error; err != nil {
			tx.Rollback()
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, status.Errorf(codes.NotFound, "destination account %d not found", internalToAccountID)
			}
			return nil, fmt.Errorf("db error fetching destination account: %w", err)
		}
		// Assign pointers for the model
		toAccountID_ptr = &internalToAccountID
		toUserID_ptr = &toAccount.OwnerUserID
		toCurrency = toAccount.Currency

	} else if req.RecipientID != nil && *req.RecipientID > 0 {
		// Scenario 2: Transfer via RecipientID (Internal or External)
		recipientIDValue := *req.RecipientID
		recipientID_ptr = &recipientIDValue // Assign pointer for model
		toAccountID_ptr = nil               // Assume nil unless recipient is internal

		recipient, err := s.recipientService.GetRecipientByID(ctx, recipientIDValue, fromUserID)
		if err != nil {
			tx.Rollback()
			// Return the wrapped error. Let the controller handle specific mapping.
			return nil, fmt.Errorf("failed to get recipient details for id %d: %w", recipientIDValue, err)
		}

		if recipient.Type == "internal" {
			if recipient.InternalAccountID == nil || *recipient.InternalAccountID == 0 {
				tx.Rollback()
				return nil, fmt.Errorf("internal recipient (ID: %d) is missing linked account ID", recipientIDValue)
			}
			// Saved Internal Recipient
			isExternal = false
			internalToAccountID := *recipient.InternalAccountID

			if req.FromAccountID == internalToAccountID {
				tx.Rollback()
				return nil, ErrCannotTransferToSelfAccount
			}

			var toAccount models.Account
			if err := tx.Where("id = ?", internalToAccountID).First(&toAccount).Error; err != nil {
				tx.Rollback()
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("linked internal account (ID: %d) for recipient (ID: %d) not found", internalToAccountID, recipientIDValue)
				}
				return nil, fmt.Errorf("db error fetching linked internal account: %w", err)
			}
			// Assign pointers for the model
			toAccountID_ptr = &internalToAccountID // Set ToAccountID for internal recipient transfer
			toUserID_ptr = &toAccount.OwnerUserID
			toCurrency = toAccount.Currency

		} else if recipient.Type == "external" {
			// Saved External Recipient
			isExternal = true
			toAccountID_ptr = nil // Explicitly nil
			toUserID_ptr = nil    // Explicitly nil

			if recipient.AccountNumber == "" || recipient.BankName == "" {
				tx.Rollback()
				return nil, fmt.Errorf("external recipient (ID: %d) is missing required bank details (account number/bank name)", recipientIDValue)
			}
			toCurrency = fromCurrency

		} else {
			tx.Rollback()
			return nil, fmt.Errorf("invalid or unsupported recipient type '%s' for recipient ID: %d", recipient.Type, recipientIDValue)
		}
	} // End of RecipientID handling

	// --- Currency Check ---
	if fromCurrency != toCurrency {
		tx.Rollback()
		return nil, fmt.Errorf("cross-currency transfers not yet supported (%s to %s)", fromCurrency, toCurrency)
	}

	// --- Calculate Fee & Total ---
	var fee int64
	if isExternal {
		fee = (req.Amount * 20) / 1000
	} else {
		fee = (req.Amount * 5) / 1000
	}
	totalAmount := req.Amount + fee

	// --- Check Balance ---
	err = s.accountService.CheckSufficientBalance(ctx, tx, req.FromAccountID, totalAmount)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, ErrSvcInsufficientFunds) {
			return nil, status.Errorf(codes.FailedPrecondition, "insufficient funds in source account %d", req.FromAccountID)
		}
		return nil, fmt.Errorf("balance check failed: %w", err)
	}

	// --- Create Transfer Record ---
	transfer := &models.Transfer{
		FromUserID:    fromUserID,
		FromAccountID: req.FromAccountID,
		ToUserID:      toUserID_ptr,    // Set based on destination logic
		ToAccountID:   toAccountID_ptr, // Set based on destination logic
		RecipientID:   recipientID_ptr, // Set based on destination logic
		Amount:        req.Amount,
		Fee:           fee,
		TotalAmount:   totalAmount,
		// Status set below
		Reference:   req.Reference,
		Category:    req.Category,
		ScheduledAt: req.ScheduledAt,
	}

	// Determine initial status and queue options
	opts := []asynq.Option{}
	if req.ScheduledAt != nil && req.ScheduledAt.After(time.Now()) {
		transfer.Status = models.TransferStatusScheduled
		opts = append(opts, asynq.ProcessAt(*req.ScheduledAt))
	} else {
		transfer.Status = models.TransferStatusProcessing
		// No specific options needed for immediate processing
	}

	if err := tx.Create(&transfer).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create transfer record: %w", err)
	}

	// --- Immediate Debit for Processing Transfers ---
	if transfer.Status == models.TransferStatusProcessing {
		// Debit the total amount directly from the source account within the transaction
		// We use a direct update as IAccountService doesn't expose a balance update method.
		// CheckSufficientBalance was already performed earlier.
		result := tx.Model(&models.Account{}).Where("id = ?", transfer.FromAccountID).Update("balance", gorm.Expr("balance - ?", transfer.TotalAmount))
		if result.Error != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to debit source account %d for immediate transfer %d: %w", transfer.FromAccountID, transfer.ID, result.Error)
		}
		// Optional: Check if exactly one row was affected
		if result.RowsAffected == 0 {
			tx.Rollback()
			// This shouldn't happen if CheckAccountOwnership and Create worked, but good to check.
			return nil, fmt.Errorf("failed to debit source account %d: account not found during update (transfer %d)", transfer.FromAccountID, transfer.ID)
		}
	}

	// Update log message now that transfer.ID is available
	if transfer.Status == models.TransferStatusScheduled {
		fmt.Printf("Scheduling transfer %d for %v\n", transfer.ID, *req.ScheduledAt)
	}

	// --- Queue Background Task (Only if Processing or Scheduled) ---
	var taskErr error
	if transfer.Status == models.TransferStatusProcessing || transfer.Status == models.TransferStatusScheduled {
		if isExternal {
			taskPayload := &tasks.PayloadProcessExternalTransfer{
				TransferID: fmt.Sprint(transfer.ID),
			}
			taskErr = s.redisWorker.DistributeTaskProcessExternalTransfer(ctx, taskPayload, opts...)
		} else {
			taskPayload := &tasks.PayloadProcessTransfer{
				TransferID: fmt.Sprint(transfer.ID),
			}
			taskErr = s.redisWorker.DistributeTaskProcessTransfer(ctx, taskPayload, opts...)
		}
	}

	if taskErr != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to distribute transfer task: %w", taskErr)
	}

	// --- Commit Transaction ---
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transfer initiation: %w", err)
	}

	// --- Return Response ---
	return &TransferResponse{
		TransferID:  transfer.ID,
		Status:      string(transfer.Status),
		Amount:      transfer.Amount,
		Fee:         transfer.Fee,
		TotalAmount: transfer.TotalAmount,
		CreatedAt:   transfer.CreatedAt,
	}, nil
}

func (s *TransferService) GetTransferDetails(ctx context.Context, transferID uint, userID uint) (*models.Transfer, error) {
	var transfer models.Transfer
	// Preload FromUser and ToUser if needed for the response later
	err := s.db.WithContext(ctx).Preload("FromUser").Preload("ToUser").Where("id = ?", transferID).First(&transfer).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTransferNotFound
		}
		return nil, fmt.Errorf("db error finding transfer: %w", err)
	}

	// Check ownership: User must be sender or receiver (handle nil ToUserID)
	isReceiver := false
	if transfer.ToUserID != nil && *transfer.ToUserID == userID { // Dereference pointer safely
		isReceiver = true
	}
	if transfer.FromUserID != userID && !isReceiver {
		return nil, ErrTransferAccessDenied
	}

	return &transfer, nil
}

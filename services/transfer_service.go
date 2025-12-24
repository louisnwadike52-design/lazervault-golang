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
	InitiateSplitBillBatch(ctx context.Context, fromUserID uint, req SplitBillRequest) (*BatchTransferResponse, error)
	InitiateBatchTransfer(ctx context.Context, fromUserID uint, req BatchTransferRequest) (*BatchTransferFullResponse, error)
	GetBatchTransferStatus(ctx context.Context, batchID uint, userID uint) (*BatchTransferFullResponse, error)
	GetBatchTransferHistory(ctx context.Context, userID uint, page, pageSize int32, statusFilter string) (*BatchTransferHistoryResponse, error)
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
	FromAccountID uint    `json:"from_account_id" validate:"required"`
	Amount        int64   `json:"amount" validate:"required,gt=0"`
	Reference     string  `json:"reference"`
	Category      string  `json:"category"`
	ScheduledAt   *string `json:"scheduled_at,omitempty"` // Optional string, format: "2006-01-02T15:04:05Z07:00"

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

// SplitBillSplit represents a single split in a split bill batch transfer
type SplitBillSplit struct {
	ToAccountID uint   `json:"to_account_id" validate:"required"`
	Amount      int64  `json:"amount" validate:"required,gt=0"`
	Description string `json:"description"`
}

// SplitBillRequest represents a batch transfer request for split bills
type SplitBillRequest struct {
	FromAccountID  uint             `json:"from_account_id" validate:"required"`
	TotalAmount    int64            `json:"total_amount" validate:"required,gt=0"`
	Description    string           `json:"description"`
	Splits         []SplitBillSplit `json:"splits" validate:"required,min=1"`
	IdempotencyKey string           `json:"idempotency_key" validate:"required"`
}

// BatchTransferItem represents a single transfer in a batch
type BatchTransferItem struct {
	TransferID  uint   `json:"transfer_id"`
	RecipientID uint   `json:"recipient_id"`
	Amount      int64  `json:"amount"`
	Status      string `json:"status"`
}

// BatchTransferResponse represents the response for batch transfer
type BatchTransferResponse struct {
	BatchID     string              `json:"batch_id"`
	TotalAmount int64               `json:"total_amount"`
	SplitCount  int                 `json:"split_count"`
	Transfers   []BatchTransferItem `json:"transfers"`
	Status      string              `json:"status"`
	CreatedAt   time.Time           `json:"created_at"`
}

// --- Batch Transfer Types (General, not split-bill specific) ---

// BatchTransferRecipient represents a single recipient in a batch transfer
type BatchTransferRecipient struct {
	RecipientID  *uint  `json:"recipient_id"`   // Optional: ID of saved recipient
	ToAccountID  *uint  `json:"to_account_id"`  // Optional: Direct account ID
	Amount       int64  `json:"amount" validate:"required,gt=0"`
	Reference    string `json:"reference"`
	Category     string `json:"category"`
}

// BatchTransferRequest represents a general batch transfer request
type BatchTransferRequest struct {
	FromAccountID uint                      `json:"from_account_id" validate:"required"`
	Recipients    []BatchTransferRecipient  `json:"recipients" validate:"required,min=1"`
	ScheduledAt   *string                   `json:"scheduled_at,omitempty"` // Optional: ISO 8601 format
}

// BatchTransferResultItem represents the result of a single transfer within a batch
type BatchTransferResultItem struct {
	TransferID      uint   `json:"transfer_id"`
	Status          string `json:"status"` // "completed" or "failed"
	Amount          int64  `json:"amount"`
	Fee             int64  `json:"fee"`
	RecipientName   string `json:"recipient_name"`
	RecipientAccount string `json:"recipient_account"` // Masked account number
	FailureReason   string `json:"failure_reason"`    // Empty if successful
}

// BatchTransferFullResponse represents the complete response for a batch transfer
type BatchTransferFullResponse struct {
	BatchID              uint                       `json:"batch_id"`
	Status               string                     `json:"status"` // "processing", "completed", "partial_success", "failed"
	TotalAmount          int64                      `json:"total_amount"`
	TotalFee             int64                      `json:"total_fee"`
	TotalAmountWithFee   int64                      `json:"total_amount_with_fee"`
	SuccessfulTransfers  int32                      `json:"successful_transfers"`
	FailedTransfers      int32                      `json:"failed_transfers"`
	TotalTransfers       int32                      `json:"total_transfers"`
	Results              []BatchTransferResultItem  `json:"results"`
	CreatedAt            time.Time                  `json:"created_at"`
	CompletedAt          *time.Time                 `json:"completed_at"` // Null if still processing
}

// BatchTransferHistoryItem represents a single batch in history
type BatchTransferHistoryItem struct {
	BatchID              uint      `json:"batch_id"`
	Status               string    `json:"status"`
	TotalAmount          int64     `json:"total_amount"`
	TotalFee             int64     `json:"total_fee"`
	TotalAmountWithFee   int64     `json:"total_amount_with_fee"`
	SuccessfulTransfers  int32     `json:"successful_transfers"`
	FailedTransfers      int32     `json:"failed_transfers"`
	TotalTransfers       int32     `json:"total_transfers"`
	CreatedAt            time.Time `json:"created_at"`
	CompletedAt          *time.Time `json:"completed_at"`
}

// BatchTransferHistoryResponse represents paginated batch transfer history
type BatchTransferHistoryResponse struct {
	Batches      []BatchTransferHistoryItem `json:"batches"`
	CurrentPage  int32                      `json:"current_page"`
	TotalPages   int32                      `json:"total_pages"`
	TotalItems   int32                      `json:"total_items"`
	ItemsPerPage int32                      `json:"items_per_page"`
	HasNext      bool                       `json:"has_next"`
	HasPrev      bool                       `json:"has_prev"`
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
	// --- COMPREHENSIVE EDGE CASE VALIDATION ---

	// 1. Validate required fields
	if req.FromAccountID == 0 {
		return nil, status.Error(codes.InvalidArgument, "from_account_id is required")
	}
	if req.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	// 2. Validate minimum transfer amount (e.g., 100 = £1.00 in minor units)
	const minTransferAmount int64 = 1 // 0.01 in minor units (1 cent/pence)
	if req.Amount < minTransferAmount {
		return nil, status.Errorf(codes.InvalidArgument, "transfer amount must be at least %d (minimum 0.01)", minTransferAmount)
	}

	// 3. Validate maximum single transfer amount (e.g., 10,000,000 = £100,000.00)
	const maxTransferAmount int64 = 1000000000 // £10,000.00 in minor units
	if req.Amount > maxTransferAmount {
		return nil, status.Errorf(codes.InvalidArgument, "transfer amount exceeds maximum allowed (%d)", maxTransferAmount)
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

	// --- EDGE CASE: Check account status (frozen, locked, closed) ---
	// Assuming Account model has a Status field (e.g., "active", "frozen", "locked", "closed")
	if fromAccount.Status != "active" {
		tx.Rollback()
		return nil, status.Errorf(codes.FailedPrecondition, "source account is %s and cannot perform transfers", fromAccount.Status)
	}

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

	// --- EDGE CASE: Prevent duplicate transfers (check for identical recent transfers) ---
	// Look for recent transfers (last 2 minutes) with same amount, from account, and destination
	var recentDuplicateCount int64
	duplicateCheckQuery := tx.Model(&models.Transfer{}).
		Where("from_account_id = ?", req.FromAccountID).
		Where("amount = ?", req.Amount).
		Where("created_at > ?", time.Now().Add(-2*time.Minute))

	// Check based on destination type
	if toAccountID_ptr != nil {
		duplicateCheckQuery = duplicateCheckQuery.Where("to_account_id = ?", *toAccountID_ptr)
	} else if recipientID_ptr != nil {
		duplicateCheckQuery = duplicateCheckQuery.Where("recipient_id = ?", *recipientID_ptr)
	}

	if err := duplicateCheckQuery.Count(&recentDuplicateCount).Error; err != nil {
		// Log error but don't fail the transfer
		fmt.Printf("Warning: duplicate check failed: %v\n", err)
	} else if recentDuplicateCount > 0 {
		tx.Rollback()
		return nil, status.Error(codes.AlreadyExists, "duplicate transfer detected - identical transfer initiated within the last 2 minutes")
	}

	// --- EDGE CASE: Check daily transfer limits ---
	const dailyTransferLimit int64 = 10000000000 // £100,000.00 daily limit
	var todayTotalAmount int64
	todayStart := time.Now().UTC().Truncate(24 * time.Hour)

	if err := tx.Model(&models.Transfer{}).
		Where("from_account_id = ?", req.FromAccountID).
		Where("created_at >= ?", todayStart).
		Where("status NOT IN (?)", []string{string(models.TransferStatusFailed), string(models.TransferStatusReverted)}).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&todayTotalAmount).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to check daily transfer limit: %w", err)
	}

	if todayTotalAmount+totalAmount > dailyTransferLimit {
		tx.Rollback()
		remainingLimit := dailyTransferLimit - todayTotalAmount
		return nil, status.Errorf(codes.ResourceExhausted, "daily transfer limit exceeded - remaining today: %d", remainingLimit)
	}

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
		ScheduledAt: req.ScheduledAt, // Store the original string
	}

	// Determine initial status and queue options
	opts := []asynq.Option{}
	if req.ScheduledAt != nil && *req.ScheduledAt != "" {
		// Parse the scheduled time in ISO 8601 UTC format
		parsedTime, err := time.Parse(time.RFC3339, *req.ScheduledAt)
		if err != nil {
			tx.Rollback()
			return nil, status.Error(codes.InvalidArgument, "invalid scheduled_at format, expected ISO 8601 UTC format (e.g., 2024-03-20T15:04:05Z)")
		}

		// Convert to UTC if not already
		parsedTime = parsedTime.UTC()

		if parsedTime.After(time.Now().UTC()) {
			transfer.Status = models.TransferStatusScheduled
			opts = append(opts, asynq.ProcessAt(parsedTime))
		} else {
			transfer.Status = models.TransferStatusProcessing
		}
	} else {
		transfer.Status = models.TransferStatusProcessing
	}

	if err := tx.Create(&transfer).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create transfer record: %w", err)
	}

	// --- Immediate Debit for Processing Transfers ---
	if transfer.Status == models.TransferStatusProcessing {
		// Debit the total amount directly from the source account within the transaction
		result := tx.Model(&models.Account{}).Where("id = ?", transfer.FromAccountID).Update("balance", gorm.Expr("balance - ?", transfer.TotalAmount))
		if result.Error != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to debit source account %d for immediate transfer %d: %w", transfer.FromAccountID, transfer.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			tx.Rollback()
			return nil, fmt.Errorf("failed to debit source account %d: account not found during update (transfer %d)", transfer.FromAccountID, transfer.ID)
		}
	}

	// Update log message now that transfer.ID is available
	if transfer.Status == models.TransferStatusScheduled && req.ScheduledAt != nil {
		fmt.Printf("Scheduling transfer %d for %s (UTC)\n", transfer.ID, *req.ScheduledAt)
	}

	// --- Queue Background Task with Production-Grade Retry Logic ---
	var taskErr error
	if transfer.Status == models.TransferStatusProcessing || transfer.Status == models.TransferStatusScheduled {
		// PRODUCTION-GRADE: Configure retry options
		retryOpts := []asynq.Option{
			asynq.MaxRetry(5),                // Retry up to 5 times
			asynq.Timeout(3 * time.Minute),   // 3-minute timeout per attempt
			asynq.Deadline(time.Now().Add(1 * time.Hour)), // Must complete within 1 hour
			asynq.Queue(tasks.QueueCritical), // High priority queue
		}

		// Merge with provided opts (e.g., scheduling)
		retryOpts = append(retryOpts, opts...)

		if isExternal {
			taskPayload := &tasks.PayloadProcessExternalTransfer{
				TransferID: fmt.Sprint(transfer.ID),
			}
			taskErr = s.redisWorker.DistributeTaskProcessExternalTransfer(ctx, taskPayload, retryOpts...)
		} else {
			taskPayload := &tasks.PayloadProcessTransfer{
				TransferID: fmt.Sprint(transfer.ID),
			}
			taskErr = s.redisWorker.DistributeTaskProcessTransfer(ctx, taskPayload, retryOpts...)
		}
	}

	if taskErr != nil {
		tx.Rollback()
		fmt.Printf("CRITICAL: Failed to queue transfer task %d: %v\n", transfer.ID, taskErr)
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

// InitiateSplitBillBatch creates multiple transfers in a single batch for split bills
func (s *TransferService) InitiateSplitBillBatch(ctx context.Context, fromUserID uint, req SplitBillRequest) (*BatchTransferResponse, error) {
	// --- VALIDATION ---

	// 1. Validate required fields
	if req.FromAccountID == 0 {
		return nil, status.Error(codes.InvalidArgument, "from_account_id is required")
	}
	if req.TotalAmount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "total_amount must be positive")
	}
	if len(req.Splits) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one split is required")
	}
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}

	// 2. Validate each split
	var totalSplitAmount int64 = 0
	for i, split := range req.Splits {
		if split.ToAccountID == 0 {
			return nil, status.Errorf(codes.InvalidArgument, "split %d: to_account_id is required", i)
		}
		if split.Amount <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "split %d: amount must be positive", i)
		}
		totalSplitAmount += split.Amount
	}

	// 3. Validate that total splits equal total amount (allow 1 cent tolerance)
	if totalSplitAmount != req.TotalAmount {
		return nil, status.Errorf(codes.InvalidArgument,
			"total split amounts (%d) do not equal total amount (%d)",
			totalSplitAmount, req.TotalAmount)
	}

	// 4. Check for duplicate idempotency key
	var existingTransfer models.Transfer
	err := s.db.WithContext(ctx).Where("idempotency_key = ?", req.IdempotencyKey).First(&existingTransfer).Error
	if err == nil {
		// Idempotency key already exists - return existing batch response
		return s.getBatchResponseByIdempotencyKey(ctx, req.IdempotencyKey)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check idempotency: %w", err)
	}

	// 5. Verify source account exists and belongs to user
	var fromAccount models.Account
	if err := s.db.WithContext(ctx).First(&fromAccount, req.FromAccountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "source account not found")
		}
		return nil, fmt.Errorf("failed to fetch source account: %w", err)
	}
	if fromAccount.OwnerUserID != fromUserID {
		return nil, status.Error(codes.PermissionDenied, "source account does not belong to user")
	}

	// 6. Check sufficient balance (including fees)
	// Calculate total fees (0.5% per transfer)
	var totalFees int64 = 0
	for _, split := range req.Splits {
		fee := int64(float64(split.Amount) * 0.005) // 0.5% fee
		totalFees += fee
	}
	totalRequired := req.TotalAmount + totalFees

	if fromAccount.Balance < totalRequired {
		return nil, status.Errorf(codes.FailedPrecondition,
			"insufficient funds: have %d, need %d (amount %d + fees %d)",
			fromAccount.Balance, totalRequired, req.TotalAmount, totalFees)
	}

	// --- CREATE BATCH TRANSFERS IN TRANSACTION ---
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	batchID := fmt.Sprintf("batch_%s_%d", req.IdempotencyKey, time.Now().Unix())
	transfers := make([]BatchTransferItem, 0, len(req.Splits))
	now := time.Now()

	// Create each transfer
	for i, split := range req.Splits {
		// Verify destination account exists
		var toAccount models.Account
		if err := s.db.WithContext(ctx).First(&toAccount, split.ToAccountID).Error; err != nil {
			tx.Rollback()
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, status.Errorf(codes.NotFound, "split %d: destination account not found", i)
			}
			return nil, fmt.Errorf("split %d: failed to fetch destination account: %w", i, err)
		}

		// Calculate fee for this transfer
		fee := int64(float64(split.Amount) * 0.005)
		totalAmount := split.Amount + fee

		// Create transfer description
		description := split.Description
		if description == "" {
			description = fmt.Sprintf("%s - Split %d/%d", req.Description, i+1, len(req.Splits))
		}

		// Create transfer record (CreatedAt and UpdatedAt are handled by GORM)
		transfer := models.Transfer{
			FromUserID:     fromUserID,
			FromAccountID:  req.FromAccountID,
			ToUserID:       &toAccount.OwnerUserID,
			ToAccountID:    &split.ToAccountID,
			Amount:         split.Amount,
			Fee:            fee,
			TotalAmount:    totalAmount,
			Reference:      description,
			Category:       "split_bill",
			Status:         models.TransferStatusPending,
			IdempotencyKey: &req.IdempotencyKey,
			BatchID:        &batchID,
		}

		if err := tx.Create(&transfer).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create transfer %d: %w", i, err)
		}

		// Queue transfer for processing
		taskPayload := &tasks.ProcessTransferPayload{
			TransferID: transfer.ID,
		}
		taskOpts := []asynq.Option{
			asynq.MaxRetry(5),
			asynq.ProcessIn(2 * time.Second),
			asynq.Queue("critical"),
			asynq.Retention(24 * time.Hour),
		}

		taskErr := s.redisWorker.DistributeProcessTransferTask(ctx, taskPayload, taskOpts...)
		if taskErr != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to queue transfer %d: %w", i, taskErr)
		}

		transfers = append(transfers, BatchTransferItem{
			TransferID:  transfer.ID,
			RecipientID: split.ToAccountID,
			Amount:      split.Amount,
			Status:      string(models.TransferStatusPending),
		})
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit batch transfer: %w", err)
	}

	// Return batch response
	return &BatchTransferResponse{
		BatchID:     batchID,
		TotalAmount: req.TotalAmount,
		SplitCount:  len(req.Splits),
		Transfers:   transfers,
		Status:      "processing",
		CreatedAt:   now,
	}, nil
}

// getBatchResponseByIdempotencyKey retrieves existing batch transfer response by idempotency key
func (s *TransferService) getBatchResponseByIdempotencyKey(ctx context.Context, idempotencyKey string) (*BatchTransferResponse, error) {
	var transfers []models.Transfer
	err := s.db.WithContext(ctx).Where("idempotency_key = ?", idempotencyKey).Find(&transfers).Error
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve batch transfers: %w", err)
	}

	if len(transfers) == 0 {
		return nil, status.Error(codes.NotFound, "batch transfer not found")
	}

	batchID := *transfers[0].BatchID
	var totalAmount int64
	items := make([]BatchTransferItem, len(transfers))

	for i, t := range transfers {
		totalAmount += t.Amount
		items[i] = BatchTransferItem{
			TransferID:  t.ID,
			RecipientID: *t.ToAccountID,
			Amount:      t.Amount,
			Status:      string(t.Status),
		}
	}

	return &BatchTransferResponse{
		BatchID:     batchID,
		TotalAmount: totalAmount,
		SplitCount:  len(transfers),
		Transfers:   items,
		Status:      "existing",
		CreatedAt:   transfers[0].CreatedAt,
	}, nil
}

// InitiateBatchTransfer creates multiple transfers in a single batch
func (s *TransferService) InitiateBatchTransfer(ctx context.Context, fromUserID uint, req BatchTransferRequest) (*BatchTransferFullResponse, error) {
	// --- VALIDATION ---

	// 1. Validate required fields
	if req.FromAccountID == 0 {
		return nil, status.Error(codes.InvalidArgument, "from_account_id is required")
	}
	if len(req.Recipients) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one recipient is required")
	}
	if len(req.Recipients) > 100 {
		return nil, status.Error(codes.InvalidArgument, "maximum 100 recipients allowed per batch")
	}

	// 2. Validate each recipient
	for i, recipient := range req.Recipients {
		if recipient.Amount <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "recipient %d: amount must be positive", i)
		}

		// Validate that EXACTLY ONE destination type is provided
		hasRecipientID := recipient.RecipientID != nil && *recipient.RecipientID > 0
		hasToAccountID := recipient.ToAccountID != nil && *recipient.ToAccountID > 0

		if !hasRecipientID && !hasToAccountID {
			return nil, status.Errorf(codes.InvalidArgument, "recipient %d: either recipient_id OR to_account_id must be provided", i)
		}
		if hasRecipientID && hasToAccountID {
			return nil, status.Errorf(codes.InvalidArgument, "recipient %d: provide either recipient_id OR to_account_id, not both", i)
		}
	}

	// 3. Verify source account exists and belongs to user
	err := s.accountService.CheckAccountOwnership(ctx, req.FromAccountID, fromUserID)
	if err != nil {
		if errors.Is(err, ErrSvcAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, "source account %d not found", req.FromAccountID)
		} else if errors.Is(err, ErrSvcAccountAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied to source account %d", req.FromAccountID)
		}
		return nil, fmt.Errorf("source account validation failed: %w", err)
	}

	var fromAccount models.Account
	if err := s.db.WithContext(ctx).First(&fromAccount, req.FromAccountID).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch source account details: %w", err)
	}

	// Check account status
	if fromAccount.Status != "active" {
		return nil, status.Errorf(codes.FailedPrecondition, "source account is %s and cannot perform transfers", fromAccount.Status)
	}

	// 4. Calculate total amount and fees
	var totalAmount int64
	var totalFee int64
	for _, recipient := range req.Recipients {
		totalAmount += recipient.Amount
		// Internal fee: 0.5%, External fee: 2%
		// We'll determine if external when processing each recipient
		fee := (recipient.Amount * 5) / 1000 // Assume internal for now, will recalculate
		totalFee += fee
	}
	totalRequired := totalAmount + totalFee

	// 5. Check sufficient balance
	err = s.accountService.CheckSufficientBalance(ctx, s.db, req.FromAccountID, totalRequired)
	if err != nil {
		if errors.Is(err, ErrSvcInsufficientFunds) {
			return nil, status.Errorf(codes.FailedPrecondition,
				"insufficient funds: have %d, need %d (amount %d + fees %d)",
				fromAccount.Balance, totalRequired, totalAmount, totalFee)
		}
		return nil, fmt.Errorf("balance check failed: %w", err)
	}

	// --- CREATE BATCH TRANSFERS IN TRANSACTION ---
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Generate unique batch ID
	batchID := fmt.Sprintf("batch_%d_%d", fromUserID, time.Now().UnixNano())
	results := make([]BatchTransferResultItem, 0, len(req.Recipients))
	successCount := int32(0)
	failCount := int32(0)
	actualTotalAmount := int64(0)
	actualTotalFee := int64(0)

	// Create each transfer
	for _, recipient := range req.Recipients {
		var toAccountID_ptr *uint
		var toUserID_ptr *uint
		var recipientID_ptr *uint
		var isExternal bool = false
		var recipientName string = "Unknown"
		var recipientAccount string = "****"
		var toCurrency string = fromAccount.Currency

		// Resolve destination
		if recipient.ToAccountID != nil && *recipient.ToAccountID > 0 {
			// Direct internal transfer
			internalToAccountID := *recipient.ToAccountID
			isExternal = false
			recipientID_ptr = nil

			if req.FromAccountID == internalToAccountID {
				failCount++
				results = append(results, BatchTransferResultItem{
					Status:        "failed",
					Amount:        recipient.Amount,
					Fee:           0,
					RecipientName: recipientName,
					RecipientAccount: recipientAccount,
					FailureReason: "cannot transfer to the same account",
				})
				continue
			}

			var toAccount models.Account
			if err := tx.Where("id = ?", internalToAccountID).First(&toAccount).Error; err != nil {
				failCount++
				results = append(results, BatchTransferResultItem{
					Status:        "failed",
					Amount:        recipient.Amount,
					Fee:           0,
					RecipientName: recipientName,
					RecipientAccount: recipientAccount,
					FailureReason: fmt.Sprintf("destination account %d not found", internalToAccountID),
				})
				continue
			}

			toAccountID_ptr = &internalToAccountID
			toUserID_ptr = &toAccount.OwnerUserID
			toCurrency = toAccount.Currency
			recipientAccount = fmt.Sprintf("****%s", toAccount.AccountNumber[len(toAccount.AccountNumber)-4:])

			// Get recipient name from user
			var toUser models.User
			if err := tx.Where("id = ?", toAccount.OwnerUserID).First(&toUser).Error; err == nil {
				recipientName = fmt.Sprintf("%s %s", toUser.FirstName, toUser.LastName)
			}

		} else if recipient.RecipientID != nil && *recipient.RecipientID > 0 {
			// Transfer via saved recipient
			recipientIDValue := *recipient.RecipientID
			recipientID_ptr = &recipientIDValue

			recipientModel, err := s.recipientService.GetRecipientByID(ctx, recipientIDValue, fromUserID)
			if err != nil {
				failCount++
				results = append(results, BatchTransferResultItem{
					Status:        "failed",
					Amount:        recipient.Amount,
					Fee:           0,
					RecipientName: recipientName,
					RecipientAccount: recipientAccount,
					FailureReason: fmt.Sprintf("recipient %d not found", recipientIDValue),
				})
				continue
			}

			recipientName = recipientModel.Name

			if recipientModel.Type == "internal" {
				if recipientModel.InternalAccountID == nil || *recipientModel.InternalAccountID == 0 {
					failCount++
					results = append(results, BatchTransferResultItem{
						Status:        "failed",
						Amount:        recipient.Amount,
						Fee:           0,
						RecipientName: recipientName,
						RecipientAccount: recipientAccount,
						FailureReason: fmt.Sprintf("internal recipient missing account ID"),
					})
					continue
				}

				isExternal = false
				internalToAccountID := *recipientModel.InternalAccountID

				var toAccount models.Account
				if err := tx.Where("id = ?", internalToAccountID).First(&toAccount).Error; err != nil {
					failCount++
					results = append(results, BatchTransferResultItem{
						Status:        "failed",
						Amount:        recipient.Amount,
						Fee:           0,
						RecipientName: recipientName,
						RecipientAccount: recipientAccount,
						FailureReason: fmt.Sprintf("linked account not found"),
					})
					continue
				}

				toAccountID_ptr = &internalToAccountID
				toUserID_ptr = &toAccount.OwnerUserID
				toCurrency = toAccount.Currency
				recipientAccount = fmt.Sprintf("****%s", toAccount.AccountNumber[len(toAccount.AccountNumber)-4:])

			} else if recipientModel.Type == "external" {
				isExternal = true
				toAccountID_ptr = nil
				toUserID_ptr = nil
				toCurrency = fromAccount.Currency

				if recipientModel.AccountNumber != "" && len(recipientModel.AccountNumber) >= 4 {
					recipientAccount = fmt.Sprintf("****%s", recipientModel.AccountNumber[len(recipientModel.AccountNumber)-4:])
				}
			}
		}

		// Currency check
		if fromAccount.Currency != toCurrency {
			failCount++
			results = append(results, BatchTransferResultItem{
				Status:        "failed",
				Amount:        recipient.Amount,
				Fee:           0,
				RecipientName: recipientName,
				RecipientAccount: recipientAccount,
				FailureReason: fmt.Sprintf("cross-currency transfers not supported"),
			})
			continue
		}

		// Calculate fee
		var fee int64
		if isExternal {
			fee = (recipient.Amount * 20) / 1000 // 2%
		} else {
			fee = (recipient.Amount * 5) / 1000 // 0.5%
		}
		transferTotalAmount := recipient.Amount + fee

		// Create transfer record
		transfer := models.Transfer{
			FromUserID:    fromUserID,
			FromAccountID: req.FromAccountID,
			ToUserID:      toUserID_ptr,
			ToAccountID:   toAccountID_ptr,
			RecipientID:   recipientID_ptr,
			Amount:        recipient.Amount,
			Fee:           fee,
			TotalAmount:   transferTotalAmount,
			Reference:     recipient.Reference,
			Category:      recipient.Category,
			BatchID:       &batchID,
			ScheduledAt:   req.ScheduledAt,
		}

		// Determine status
		if req.ScheduledAt != nil && *req.ScheduledAt != "" {
			parsedTime, err := time.Parse(time.RFC3339, *req.ScheduledAt)
			if err == nil && parsedTime.After(time.Now().UTC()) {
				transfer.Status = models.TransferStatusScheduled
			} else {
				transfer.Status = models.TransferStatusProcessing
			}
		} else {
			transfer.Status = models.TransferStatusProcessing
		}

		if err := tx.Create(&transfer).Error; err != nil {
			failCount++
			results = append(results, BatchTransferResultItem{
				Status:        "failed",
				Amount:        recipient.Amount,
				Fee:           fee,
				RecipientName: recipientName,
				RecipientAccount: recipientAccount,
				FailureReason: fmt.Sprintf("failed to create transfer: %v", err),
			})
			continue
		}

		// Debit immediately for processing transfers
		if transfer.Status == models.TransferStatusProcessing {
			result := tx.Model(&models.Account{}).Where("id = ?", req.FromAccountID).Update("balance", gorm.Expr("balance - ?", transferTotalAmount))
			if result.Error != nil || result.RowsAffected == 0 {
				tx.Rollback()
				return nil, fmt.Errorf("failed to debit source account")
			}
		}

		// Queue background task
		opts := []asynq.Option{
			asynq.MaxRetry(5),
			asynq.Timeout(3 * time.Minute),
			asynq.Queue(tasks.QueueCritical),
		}

		if transfer.Status == models.TransferStatusScheduled && req.ScheduledAt != nil {
			parsedTime, _ := time.Parse(time.RFC3339, *req.ScheduledAt)
			opts = append(opts, asynq.ProcessAt(parsedTime))
		}

		var taskErr error
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

		if taskErr != nil {
			failCount++
			results = append(results, BatchTransferResultItem{
				TransferID:    transfer.ID,
				Status:        "failed",
				Amount:        recipient.Amount,
				Fee:           fee,
				RecipientName: recipientName,
				RecipientAccount: recipientAccount,
				FailureReason: fmt.Sprintf("failed to queue task: %v", taskErr),
			})
			continue
		}

		// Success
		successCount++
		actualTotalAmount += recipient.Amount
		actualTotalFee += fee
		results = append(results, BatchTransferResultItem{
			TransferID:    transfer.ID,
			Status:        "completed",
			Amount:        recipient.Amount,
			Fee:           fee,
			RecipientName: recipientName,
			RecipientAccount: recipientAccount,
			FailureReason: "",
		})
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit batch transfer: %w", err)
	}

	// Determine overall batch status
	batchStatus := "processing"
	if failCount > 0 && successCount == 0 {
		batchStatus = "failed"
	} else if failCount > 0 && successCount > 0 {
		batchStatus = "partial_success"
	} else if successCount == int32(len(req.Recipients)) {
		batchStatus = "completed"
	}

	// For the batch ID in response, we need to store it as a uint
	// Let's create a simple hash or use the first transfer's ID as batch ID reference
	var batchIDUint uint = 0
	if len(results) > 0 && results[0].TransferID > 0 {
		batchIDUint = results[0].TransferID // Use first transfer ID as batch identifier
	}

	return &BatchTransferFullResponse{
		BatchID:             batchIDUint,
		Status:              batchStatus,
		TotalAmount:         actualTotalAmount,
		TotalFee:            actualTotalFee,
		TotalAmountWithFee:  actualTotalAmount + actualTotalFee,
		SuccessfulTransfers: successCount,
		FailedTransfers:     failCount,
		TotalTransfers:      int32(len(req.Recipients)),
		Results:             results,
		CreatedAt:           time.Now(),
		CompletedAt:         nil,
	}, nil
}

// GetBatchTransferStatus retrieves the status of a batch transfer by batch ID
func (s *TransferService) GetBatchTransferStatus(ctx context.Context, batchID uint, userID uint) (*BatchTransferFullResponse, error) {
	// Find all transfers with the given batch ID
	var transfers []models.Transfer

	// We need to find transfers where the first transfer ID matches batchID
	// or where BatchID string contains this ID
	var firstTransfer models.Transfer
	if err := s.db.WithContext(ctx).Where("id = ?", batchID).First(&firstTransfer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "batch transfer not found")
		}
		return nil, fmt.Errorf("failed to find batch transfer: %w", err)
	}

	// Verify ownership
	if firstTransfer.FromUserID != userID {
		return nil, status.Error(codes.PermissionDenied, "access denied to batch transfer")
	}

	if firstTransfer.BatchID == nil || *firstTransfer.BatchID == "" {
		return nil, status.Error(codes.NotFound, "transfer is not part of a batch")
	}

	batchIDStr := *firstTransfer.BatchID

	// Get all transfers in this batch
	err := s.db.WithContext(ctx).
		Preload("ToUser").
		Preload("ToAccount").
		Preload("Recipient").
		Where("batch_id = ?", batchIDStr).
		Find(&transfers).Error

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve batch transfers: %w", err)
	}

	if len(transfers) == 0 {
		return nil, status.Error(codes.NotFound, "batch transfer not found")
	}

	// Build response
	results := make([]BatchTransferResultItem, len(transfers))
	var totalAmount int64
	var totalFee int64
	var successCount int32
	var failCount int32
	var completedAt *time.Time

	allCompleted := true
	for i, transfer := range transfers {
		recipientName := "Unknown"
		recipientAccount := "****"

		// Get recipient info
		if transfer.ToUser != nil {
			recipientName = fmt.Sprintf("%s %s", transfer.ToUser.FirstName, transfer.ToUser.LastName)
		}
		if transfer.ToAccount != nil && len(transfer.ToAccount.AccountNumber) >= 4 {
			recipientAccount = fmt.Sprintf("****%s", transfer.ToAccount.AccountNumber[len(transfer.ToAccount.AccountNumber)-4:])
		} else if transfer.Recipient != nil && transfer.Recipient.AccountNumber != "" && len(transfer.Recipient.AccountNumber) >= 4 {
			recipientAccount = fmt.Sprintf("****%s", transfer.Recipient.AccountNumber[len(transfer.Recipient.AccountNumber)-4:])
			recipientName = transfer.Recipient.Name
		}

		totalAmount += transfer.Amount
		totalFee += transfer.Fee

		transferStatus := string(transfer.Status)
		if transfer.Status == models.TransferStatusCompleted {
			successCount++
			if transfer.CompletedAt != nil {
				completedAt = transfer.CompletedAt
			}
		} else if transfer.Status == models.TransferStatusFailed {
			failCount++
		} else {
			allCompleted = false
		}

		results[i] = BatchTransferResultItem{
			TransferID:      transfer.ID,
			Status:          transferStatus,
			Amount:          transfer.Amount,
			Fee:             transfer.Fee,
			RecipientName:   recipientName,
			RecipientAccount: recipientAccount,
			FailureReason:   transfer.FailureReason,
		}
	}

	// Determine overall batch status
	batchStatus := "processing"
	if allCompleted {
		if failCount > 0 && successCount == 0 {
			batchStatus = "failed"
		} else if failCount > 0 && successCount > 0 {
			batchStatus = "partial_success"
		} else {
			batchStatus = "completed"
		}
	}

	return &BatchTransferFullResponse{
		BatchID:             batchID,
		Status:              batchStatus,
		TotalAmount:         totalAmount,
		TotalFee:            totalFee,
		TotalAmountWithFee:  totalAmount + totalFee,
		SuccessfulTransfers: successCount,
		FailedTransfers:     failCount,
		TotalTransfers:      int32(len(transfers)),
		Results:             results,
		CreatedAt:           transfers[0].CreatedAt,
		CompletedAt:         completedAt,
	}, nil
}

// GetBatchTransferHistory retrieves paginated batch transfer history for a user
func (s *TransferService) GetBatchTransferHistory(ctx context.Context, userID uint, page, pageSize int32, statusFilter string) (*BatchTransferHistoryResponse, error) {
	// Validate pagination parameters
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Query for distinct batch IDs from user's transfers
	query := s.db.WithContext(ctx).
		Model(&models.Transfer{}).
		Where("from_user_id = ? AND batch_id IS NOT NULL AND batch_id != ''", userID)

	// Apply status filter if provided
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	// Get distinct batch IDs with pagination
	// Use a struct to hold the results
	type BatchIDResult struct {
		BatchID      string
		MaxCreatedAt time.Time
	}
	var batchResults []BatchIDResult

	if err := query.
		Select("batch_id, MAX(created_at) as max_created_at").
		Group("batch_id").
		Order("max_created_at DESC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Scan(&batchResults).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve batch IDs: %w", err)
	}

	// Extract batch IDs
	batchIDs := make([]string, len(batchResults))
	for i, result := range batchResults {
		batchIDs[i] = result.BatchID
	}

	// Get total count for pagination
	var totalCount int64
	countQuery := s.db.WithContext(ctx).
		Model(&models.Transfer{}).
		Where("from_user_id = ? AND batch_id IS NOT NULL AND batch_id != ''", userID)

	if statusFilter != "" {
		countQuery = countQuery.Where("status = ?", statusFilter)
	}

	if err := countQuery.Select("COUNT(DISTINCT batch_id)").Count(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count batches: %w", err)
	}

	// Build batch history items
	batches := make([]BatchTransferHistoryItem, 0, len(batchIDs))

	for _, batchIDStr := range batchIDs {
		var transfers []models.Transfer
		err := s.db.WithContext(ctx).
			Where("batch_id = ?", batchIDStr).
			Find(&transfers).Error

		if err != nil {
			continue // Skip this batch on error
		}

		if len(transfers) == 0 {
			continue
		}

		// Aggregate batch statistics
		var totalAmount int64
		var totalFee int64
		var successCount int32
		var failCount int32
		var completedAt *time.Time
		allCompleted := true

		for _, transfer := range transfers {
			totalAmount += transfer.Amount
			totalFee += transfer.Fee

			if transfer.Status == models.TransferStatusCompleted {
				successCount++
				if transfer.CompletedAt != nil {
					completedAt = transfer.CompletedAt
				}
			} else if transfer.Status == models.TransferStatusFailed {
				failCount++
			} else {
				allCompleted = false
			}
		}

		// Determine batch status
		batchStatus := "processing"
		if allCompleted {
			if failCount > 0 && successCount == 0 {
				batchStatus = "failed"
			} else if failCount > 0 && successCount > 0 {
				batchStatus = "partial_success"
			} else {
				batchStatus = "completed"
			}
		}

		batches = append(batches, BatchTransferHistoryItem{
			BatchID:             transfers[0].ID, // Use first transfer ID as batch identifier
			Status:              batchStatus,
			TotalAmount:         totalAmount,
			TotalFee:            totalFee,
			TotalAmountWithFee:  totalAmount + totalFee,
			SuccessfulTransfers: successCount,
			FailedTransfers:     failCount,
			TotalTransfers:      int32(len(transfers)),
			CreatedAt:           transfers[0].CreatedAt,
			CompletedAt:         completedAt,
		})
	}

	// Calculate pagination info
	totalPages := int32((totalCount + int64(pageSize) - 1) / int64(pageSize))
	hasNext := page < totalPages
	hasPrev := page > 1

	return &BatchTransferHistoryResponse{
		Batches:      batches,
		CurrentPage:  page,
		TotalPages:   totalPages,
		TotalItems:   int32(totalCount),
		ItemsPerPage: pageSize,
		HasNext:      hasNext,
		HasPrev:      hasPrev,
	}, nil
}

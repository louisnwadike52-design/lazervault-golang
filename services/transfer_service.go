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
	"gorm.io/gorm"
)

var (
	ErrTransferFromAccountNotFound = errors.New("transfer service: source account not found or invalid")
	ErrTransferToAccountNotFound   = errors.New("transfer service: destination account not found")
	ErrTransferAccountLookupFailed = errors.New("transfer service: failed to look up accounts")
	ErrCannotTransferToSelfAccount = errors.New("transfer service: cannot transfer to the same account")
	ErrTransferNotFound            = errors.New("transfer service: transfer not found")
	ErrTransferAccessDenied        = errors.New("transfer service: access denied to transfer details")
)

type ITransferService interface {
	InitiateTransfer(ctx context.Context, fromUserID uint, req TransferRequest) (*TransferResponse, error)
	GetTransferDetails(ctx context.Context, transferID uint, userID uint) (*models.Transfer, error)
}

type TransferService struct {
	db          *gorm.DB
	config      *configs.Config
	redisWorker tasks.TaskDistributor
}

type TransferRequest struct {
	FromUserID    uint       // Keep FromUserID (from auth context)
	FromAccountID uint       `json:"from_account_id" validate:"required"` // Use this
	ToAccountID   uint       `json:"to_account_id" validate:"required"`   // Use this
	Amount        int64      `json:"amount" validate:"required,gt=0"`
	Reference     string     `json:"reference"`
	Category      string     `json:"category"`
	ScheduledAt   *time.Time `json:"scheduled_at"`
}

type TransferResponse struct {
	TransferID  uint      `json:"transfer_id"`
	Status      string    `json:"status"`
	Amount      int64     `json:"amount"`
	Fee         int64     `json:"fee"`
	TotalAmount int64     `json:"total_amount"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewTransferService(db *gorm.DB, config *configs.Config, redisWorker tasks.TaskDistributor) *TransferService {
	return &TransferService{
		db:          db,
		config:      config,
		redisWorker: redisWorker,
	}
}

func (s *TransferService) InitiateTransfer(ctx context.Context, fromUserID uint, req TransferRequest) (*TransferResponse, error) {
	// Basic validation
	if req.FromAccountID == 0 || req.ToAccountID == 0 {
		return nil, fmt.Errorf("from_account_id and to_account_id are required")
	}
	if req.FromAccountID == req.ToAccountID {
		return nil, ErrCannotTransferToSelfAccount
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}

	var toUserID uint
	var fromCurrency string
	var toCurrency string

	// Start a transaction
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		} else if tx.Error != nil {
			tx.Rollback()
		}
	}()

	// --- Validate Accounts and Get Necessary Info within Transaction ---
	// Fetch FromAccount and verify owner
	var fromAccount models.Account
	if err := tx.Where("id = ? AND owner_user_id = ?", req.FromAccountID, fromUserID).First(&fromAccount).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			tx.Rollback()
			return nil, ErrTransferFromAccountNotFound
		}
		tx.Rollback()
		return nil, fmt.Errorf("%w: finding from_account: %w", ErrTransferAccountLookupFailed, err)
	}
	fromCurrency = fromAccount.Currency

	// Fetch ToAccount and get its owner ID (ToUserID)
	var toAccount models.Account
	if err := tx.Where("id = ?", req.ToAccountID).First(&toAccount).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			tx.Rollback()
			return nil, ErrTransferToAccountNotFound
		}
		tx.Rollback()
		return nil, fmt.Errorf("%w: finding to_account: %w", ErrTransferAccountLookupFailed, err)
	}
	toUserID = toAccount.OwnerUserID
	toCurrency = toAccount.Currency

	// --- Currency Check (Example: Allow only same-currency transfers for now) ---
	if fromCurrency != toCurrency {
		tx.Rollback()
		return nil, fmt.Errorf("cross-currency transfers not supported (from %s to %s)", fromCurrency, toCurrency)
		// TODO: Implement currency conversion logic if needed
	}

	// Calculate fee using integer arithmetic
	fee := (req.Amount * 15) / 1000 // Example: 1.5%
	totalAmount := req.Amount + fee

	// Create transfer record using Account IDs and fetched ToUserID
	transfer := &models.Transfer{
		FromUserID:    fromUserID,
		ToUserID:      toUserID, // Use fetched ToUserID
		FromAccountID: req.FromAccountID,
		ToAccountID:   req.ToAccountID,
		Amount:        req.Amount,
		Fee:           fee,
		TotalAmount:   totalAmount,
		Status:        models.TransferStatusPending,
		Reference:     req.Reference,
		Category:      req.Category,
		ScheduledAt:   req.ScheduledAt,
	}

	if err := tx.Create(&transfer).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create transfer record: %w", err)
	}

	// Queue the transfer task
	payload := &tasks.PayloadProcessTransfer{
		TransferID: fmt.Sprint(transfer.ID),
	}

	var opts []asynq.Option
	if req.ScheduledAt != nil {
		opts = append(opts, asynq.ProcessAt(*req.ScheduledAt))
	}

	// Use the specific distributor method
	if err := s.redisWorker.DistributeTaskProcessTransfer(ctx, payload, opts...); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to distribute transfer task: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transfer initiation: %w", err)
	}

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

	// Fetch the transfer record. Preload related accounts if their info (like currency) is needed
	// For now, assume currency is derived or not strictly needed in response directly from account
	err := s.db.WithContext(ctx).Where("id = ?", transferID).First(&transfer).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTransferNotFound
		}
		return nil, fmt.Errorf("db error finding transfer: %w", err)
	}

	// Check ownership: User must be sender or receiver
	if transfer.FromUserID != userID && transfer.ToUserID != userID {
		return nil, ErrTransferAccessDenied
	}

	return &transfer, nil
}

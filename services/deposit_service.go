package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/tasks"
	"time"

	// bcrypt should be here if used by HashPassword
	"github.com/hibiken/asynq"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// --- Deposit Service Errors ---
var (
	ErrDepositTargetAccountNotFound = errors.New("deposit service: target account not found or does not belong to user")
	ErrDepositInvalidAmount         = errors.New("deposit service: deposit amount must be positive")
	ErrDepositCurrencyMismatch      = errors.New("deposit service: deposit currency does not match target account currency")
	ErrDepositProcessingFailed      = errors.New("deposit service: failed to process deposit and update balance")
	ErrDepositAccountInactive       = errors.New("deposit service: cannot deposit into inactive/blocked account")

	ErrDepositInitiationFailed  = errors.New("deposit service: failed to initiate deposit record")
	ErrDepositEnqueueTaskFailed = errors.New("deposit service: failed to enqueue processing task")
	ErrDepositCurrencyMissing   = errors.New("deposit service: currency is required")
	ErrDepositSourceBankMissing = errors.New("deposit service: source bank name is required")

	// New errors for GetDepositDetails
	ErrDepositNotFound     = errors.New("deposit service: deposit not found")
	ErrDepositAccessDenied = errors.New("deposit service: access denied to deposit details")
)

// --- Deposit Service Interface ---

type IDepositService interface {
	InitiateDeposit(ctx context.Context, userID uint, req *pb.InitiateDepositRequest) (*pb.InitiateDepositResponse, error)
	GetDepositDetails(ctx context.Context, depositID string, userID uint) (*pb.GetDepositDetailsResponse, error)
}

// --- Deposit Service Struct ---

type DepositService struct {
	db          *gorm.DB
	distributor tasks.TaskDistributor
	// We need AccountService ONLY for the converter helper.
	// This isn't ideal. Consider moving the converter.
	accountService IAccountService // Added temporarily
}

// --- Deposit Service Constructor ---

// Updated constructor to accept IAccountService
func NewDepositService(db *gorm.DB, distributor tasks.TaskDistributor, accountService IAccountService) IDepositService {
	return &DepositService{db: db, distributor: distributor, accountService: accountService}
}

// --- Deposit Service Methods ---

// InitiateDeposit creates a deposit record and enqueues a task for processing.
func (s *DepositService) InitiateDeposit(ctx context.Context, userID uint, req *pb.InitiateDepositRequest) (*pb.InitiateDepositResponse, error) {
	// 1. Validate Input
	if req.Amount == 0 {
		return nil, ErrDepositInvalidAmount
	}
	// Store amount directly as int64 (minor units)
	amountInt := int64(req.Amount)
	if amountInt <= 0 { // Redundant check, but safe
		return nil, ErrDepositInvalidAmount
	}
	if req.Currency == "" {
		return nil, ErrDepositCurrencyMissing
	}
	if req.SourceBankName == "" {
		return nil, ErrDepositSourceBankMissing
	}
	targetAccountID := uint(req.GetTargetAccountId())
	if targetAccountID == 0 {
		return nil, fmt.Errorf("target_account_id is required")
	}

	// 2. Create Deposit Record
	deposit := models.Deposit{
		UserID:          userID,
		TargetAccountID: targetAccountID,
		Amount:          amountInt, // Use int64 amount
		Currency:        req.Currency,
		SourceBankName:  req.SourceBankName,
		Status:          models.DepositStatusPending,
	}

	// Use transaction here to ensure task is only queued if record is saved
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&deposit).Error; err != nil {
			return fmt.Errorf("%w: %v", ErrDepositInitiationFailed, err)
		}

		// 3. Enqueue Processing Task within the same transaction
		depositPayload := &tasks.DepositProcessPayload{DepositID: deposit.ID}

		opts := []asynq.Option{
			asynq.MaxRetry(5),
			asynq.ProcessAt(time.Now().Add(5 * time.Second)),
		}

		if err := s.distributor.DistributeTaskDepositProcess(ctx, depositPayload, opts...); err != nil {
			fmt.Printf("CRITICAL: Error distributing deposit task for deposit %s: %v\n", deposit.ID, err)
			return fmt.Errorf("%w: %v", ErrDepositEnqueueTaskFailed, err)
		}

		return nil // Commit transaction
	})

	if txErr != nil {
		return nil, txErr
	}

	// 4. Construct and Return Acknowledgement Response
	resp := &pb.InitiateDepositResponse{
		DepositId: deposit.ID,
		Status:    pb.DepositStatus_DEPOSIT_STATUS_PENDING,
		Message:   "Deposit initiated and is processing asynchronously.",
	}

	return resp, nil
}

// GetDepositDetails retrieves details for a specific deposit.
func (s *DepositService) GetDepositDetails(ctx context.Context, depositID string, userID uint) (*pb.GetDepositDetailsResponse, error) {
	var deposit models.Deposit
	var failedDeposit models.FailedDeposit
	var account models.Account
	foundInDeposits := false
	foundInFailed := false

	// Check active deposits first
	err := s.db.WithContext(ctx).Where("id = ?", depositID).First(&deposit).Error
	if err == nil {
		foundInDeposits = true
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("db error finding deposit: %w", err)
	}

	// If not found in active, check failed deposits
	if !foundInDeposits {
		err = s.db.WithContext(ctx).Where("original_deposit_id = ?", depositID).First(&failedDeposit).Error
		if err == nil {
			foundInFailed = true
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("db error finding failed deposit: %w", err)
		}
	}

	if !foundInDeposits && !foundInFailed {
		return nil, ErrDepositNotFound
	}

	resp := &pb.GetDepositDetailsResponse{}
	var depositUserID uint

	if foundInDeposits {
		depositUserID = deposit.UserID
		resp.DepositId = deposit.ID
		resp.TargetAccountId = uint64(deposit.TargetAccountID)
		resp.Amount = uint64(deposit.Amount) // Amount is now int64, cast to uint64
		resp.Currency = deposit.Currency
		resp.SourceBankName = deposit.SourceBankName
		resp.Status = convertModelStatusToProto(deposit.Status)
		resp.CreatedAt = timestamppb.New(deposit.CreatedAt)
		if deposit.ProcessingAt != nil {
			resp.ProcessingAt = timestamppb.New(*deposit.ProcessingAt)
		}
		if deposit.CompletedAt != nil {
			resp.CompletedAt = timestamppb.New(*deposit.CompletedAt)
		}
		if deposit.FailedAt != nil {
			resp.FailedAt = timestamppb.New(*deposit.FailedAt)
		}
		if deposit.FailureReason != nil {
			resp.FailureReason = *deposit.FailureReason
		}
		if deposit.ExternalTransactionID != nil {
			resp.ExternalTransactionId = *deposit.ExternalTransactionID
		}
	} else { // Found in failed
		depositUserID = failedDeposit.UserID
		resp.DepositId = failedDeposit.OriginalDepositID
		resp.TargetAccountId = uint64(failedDeposit.TargetAccountID)
		resp.Amount = uint64(failedDeposit.Amount) // Amount is now int64, cast to uint64
		resp.Currency = failedDeposit.Currency
		resp.SourceBankName = failedDeposit.SourceBankName
		resp.Status = pb.DepositStatus_DEPOSIT_STATUS_FAILED
		resp.CreatedAt = timestamppb.New(failedDeposit.AttemptedAt)
		resp.FailedAt = timestamppb.New(failedDeposit.FailedAt)
		resp.FailureReason = failedDeposit.FailureReason
		if failedDeposit.ExternalTransactionID != nil {
			resp.ExternalTransactionId = *failedDeposit.ExternalTransactionID
		}
	}

	if depositUserID != userID {
		return nil, ErrDepositAccessDenied
	}

	if foundInDeposits && deposit.Status == models.DepositStatusCompleted {
		err = s.db.WithContext(ctx).Where("id = ?", deposit.TargetAccountID).First(&account).Error
		if err != nil {
			fmt.Printf("WARN: Failed to fetch account details for completed deposit %s: %v\n", depositID, err)
		} else {
			resp.UpdatedAccount = ConvertAccountToProtoDetails(&account) // Use external helper
		}
	}

	return resp, nil
}

// Helper to convert model status to proto status
func convertModelStatusToProto(status models.DepositStatus) pb.DepositStatus {
	switch status {
	case models.DepositStatusPending:
		return pb.DepositStatus_DEPOSIT_STATUS_PENDING
	case models.DepositStatusProcessing:
		return pb.DepositStatus_DEPOSIT_STATUS_PROCESSING
	case models.DepositStatusCompleted:
		return pb.DepositStatus_DEPOSIT_STATUS_COMPLETED
	case models.DepositStatusFailed:
		return pb.DepositStatus_DEPOSIT_STATUS_FAILED // Should ideally not be stored long term in Deposit table
	default:
		return pb.DepositStatus_DEPOSIT_STATUS_UNSPECIFIED
	}
}

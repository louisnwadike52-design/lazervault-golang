package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"math"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// LockFundsService handles lock funds operations
type LockFundsService struct {
	db *gorm.DB
}

// NewLockFundsService creates a new lock funds service
func NewLockFundsService(db *gorm.DB) *LockFundsService {
	return &LockFundsService{db: db}
}

// CreateLockFund creates a new lock fund
func (s *LockFundsService) CreateLockFund(ctx context.Context, userID uint, req *pb.CreateLockFundRequest) (*models.LockFund, error) {
	// Validate request
	if req.Amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}

	if req.LockDurationDays <= 0 {
		return nil, errors.New("lock duration must be greater than zero")
	}

	// Calculate interest rate based on lock type and duration
	interestRate := s.calculateInterestRate(req.LockType, req.LockDurationDays)
	earlyUnlockPenalty := s.getEarlyUnlockPenalty(req.LockType)

	now := time.Now()
	unlockAt := now.AddDate(0, 0, int(req.LockDurationDays))

	lockFund := &models.LockFund{
		UserID:                    userID,
		LockType:                  s.convertProtoLockType(req.LockType),
		Amount:                    req.Amount,
		Currency:                  req.Currency,
		LockDurationDays:          req.LockDurationDays,
		InterestRate:              interestRate,
		LockedAt:                  now,
		UnlockAt:                  unlockAt,
		Status:                    models.LockStatusActive,
		AutoRenew:                 req.AutoRenew,
		GoalName:                  req.GoalName,
		GoalDescription:           req.GoalDescription,
		EarlyUnlockPenaltyPercent: earlyUnlockPenalty,
		AccruedInterest:           0,
		PaymentMethod:             req.PaymentMethod,
	}

	// Create lock fund in database
	if err := s.db.WithContext(ctx).Create(lockFund).Error; err != nil {
		return nil, fmt.Errorf("failed to create lock fund: %w", err)
	}

	// Create initial transaction
	transaction := &models.LockFundTransaction{
		LockFundID:      lockFund.ID,
		UserID:          userID,
		TransactionType: "LOCK",
		Amount:          req.Amount,
		Currency:        req.Currency,
		PaymentMethod:   req.PaymentMethod,
		Status:          "COMPLETED",
		TransactionDate: now,
		Description:     fmt.Sprintf("Locked %s %s for %d days", req.Currency, fmt.Sprintf("%.2f", req.Amount), req.LockDurationDays),
	}

	if err := s.db.WithContext(ctx).Create(transaction).Error; err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	lockFund.TransactionID = transaction.ID
	s.db.WithContext(ctx).Save(lockFund)

	return lockFund, nil
}

// GetLockFunds retrieves all lock funds for a user
func (s *LockFundsService) GetLockFunds(ctx context.Context, userID uint, status *pb.LockStatus, page, perPage int32) ([]*models.LockFund, int64, error) {
	var lockFunds []*models.LockFund
	var total int64

	query := s.db.WithContext(ctx).Where("user_id = ?", userID)

	if status != nil && *status != pb.LockStatus_LOCK_STATUS_UNSPECIFIED {
		query = query.Where("status = ?", s.convertProtoLockStatus(*status))
	}

	// Get total count
	if err := query.Model(&models.LockFund{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count lock funds: %w", err)
	}

	// Apply pagination
	offset := (page - 1) * perPage
	query = query.Offset(int(offset)).Limit(int(perPage))

	// Fetch lock funds
	if err := query.Order("created_at DESC").Find(&lockFunds).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch lock funds: %w", err)
	}

	// Update accrued interest for active locks
	for _, lockFund := range lockFunds {
		if lockFund.Status == models.LockStatusActive {
			lockFund.UpdateAccruedInterest()
			s.db.WithContext(ctx).Save(lockFund)
		}
	}

	return lockFunds, total, nil
}

// GetLockFund retrieves a single lock fund by ID
func (s *LockFundsService) GetLockFund(ctx context.Context, userID uint, lockFundID string) (*models.LockFund, error) {
	var lockFund models.LockFund

	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", lockFundID, userID).
		First(&lockFund).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("lock fund not found")
		}
		return nil, fmt.Errorf("failed to fetch lock fund: %w", err)
	}

	// Update accrued interest
	if lockFund.Status == models.LockStatusActive {
		lockFund.UpdateAccruedInterest()
		s.db.WithContext(ctx).Save(&lockFund)
	}

	return &lockFund, nil
}

// UnlockFund unlocks a fund
func (s *LockFundsService) UnlockFund(ctx context.Context, userID uint, lockFundID string, forceEarlyUnlock bool) (float64, float64, float64, *models.LockFund, error) {
	var lockFund models.LockFund

	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", lockFundID, userID).
		First(&lockFund).Error; err != nil {
		return 0, 0, 0, nil, errors.New("lock fund not found")
	}

	if lockFund.Status != models.LockStatusActive {
		return 0, 0, 0, nil, errors.New("lock fund is not active")
	}

	now := time.Now()
	isEarlyUnlock := now.Before(lockFund.UnlockAt)

	if isEarlyUnlock && !forceEarlyUnlock {
		return 0, 0, 0, nil, errors.New("lock has not matured yet. Set force_early_unlock to true to unlock early")
	}

	if isEarlyUnlock && !lockFund.CanUnlockEarly() {
		return 0, 0, 0, nil, errors.New("early unlock not allowed for this lock type")
	}

	// Calculate final interest
	lockFund.UpdateAccruedInterest()
	interestEarned := lockFund.AccruedInterest

	// Calculate penalty if early unlock
	var penaltyAmount float64
	if isEarlyUnlock {
		penaltyAmount = lockFund.Amount * (lockFund.EarlyUnlockPenaltyPercent / 100.0)
	}

	amountReturned := lockFund.Amount - penaltyAmount + interestEarned

	// Update lock fund status
	lockFund.Status = models.LockStatusUnlocked
	if err := s.db.WithContext(ctx).Save(&lockFund).Error; err != nil {
		return 0, 0, 0, nil, fmt.Errorf("failed to update lock fund: %w", err)
	}

	// Create unlock transaction
	transaction := &models.LockFundTransaction{
		LockFundID:      lockFund.ID,
		UserID:          userID,
		TransactionType: "UNLOCK",
		Amount:          amountReturned,
		Currency:        lockFund.Currency,
		PaymentMethod:   lockFund.PaymentMethod,
		Status:          "COMPLETED",
		TransactionDate: now,
		Description:     fmt.Sprintf("Unlocked fund with interest: %s %.2f", lockFund.Currency, interestEarned),
	}

	if err := s.db.WithContext(ctx).Create(transaction).Error; err != nil {
		return 0, 0, 0, nil, fmt.Errorf("failed to create unlock transaction: %w", err)
	}

	// Create penalty transaction if applicable
	if penaltyAmount > 0 {
		penaltyTx := &models.LockFundTransaction{
			LockFundID:      lockFund.ID,
			UserID:          userID,
			TransactionType: "PENALTY",
			Amount:          penaltyAmount,
			Currency:        lockFund.Currency,
			Status:          "COMPLETED",
			TransactionDate: now,
			Description:     fmt.Sprintf("Early unlock penalty: %s %.2f", lockFund.Currency, penaltyAmount),
		}
		s.db.WithContext(ctx).Create(penaltyTx)
	}

	return amountReturned, penaltyAmount, interestEarned, &lockFund, nil
}

// GetLockTransactions retrieves transactions for lock funds
func (s *LockFundsService) GetLockTransactions(ctx context.Context, userID uint, lockFundID *string, page, perPage int32) ([]*models.LockFundTransaction, int64, error) {
	var transactions []*models.LockFundTransaction
	var total int64

	query := s.db.WithContext(ctx).Where("user_id = ?", userID)

	if lockFundID != nil && *lockFundID != "" {
		query = query.Where("lock_fund_id = ?", *lockFundID)
	}

	// Get total count
	if err := query.Model(&models.LockFundTransaction{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count transactions: %w", err)
	}

	// Apply pagination
	offset := (page - 1) * perPage
	query = query.Offset(int(offset)).Limit(int(perPage))

	// Fetch transactions
	if err := query.Order("transaction_date DESC").Find(&transactions).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch transactions: %w", err)
	}

	return transactions, total, nil
}

// CalculateInterest calculates estimated interest
func (s *LockFundsService) CalculateInterest(lockType pb.LockType, amount float64, durationDays int32) (float64, float64, float64, float64, error) {
	if amount <= 0 || durationDays <= 0 {
		return 0, 0, 0, 0, errors.New("invalid amount or duration")
	}

	interestRate := s.calculateInterestRate(lockType, durationDays)

	// Calculate simple interest
	dailyRate := interestRate / 365.0 / 100.0
	estimatedInterest := amount * dailyRate * float64(durationDays)
	totalReturn := amount + estimatedInterest

	// Calculate APY (Annual Percentage Yield)
	apy := (math.Pow(1+(dailyRate), 365) - 1) * 100

	return interestRate, estimatedInterest, totalReturn, apy, nil
}

// RenewLockFund renews a matured lock fund
func (s *LockFundsService) RenewLockFund(ctx context.Context, userID uint, lockFundID string, newDurationDays int32) (*models.LockFund, error) {
	var lockFund models.LockFund

	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", lockFundID, userID).
		First(&lockFund).Error; err != nil {
		return nil, errors.New("lock fund not found")
	}

	if lockFund.Status != models.LockStatusMatured && lockFund.Status != models.LockStatusActive {
		return nil, errors.New("can only renew matured or active locks")
	}

	// Add accrued interest to principal
	lockFund.UpdateAccruedInterest()
	newAmount := lockFund.Amount + lockFund.AccruedInterest

	now := time.Now()
	lockFund.Amount = newAmount
	lockFund.AccruedInterest = 0
	lockFund.LockedAt = now
	lockFund.UnlockAt = now.AddDate(0, 0, int(newDurationDays))
	lockFund.LockDurationDays = newDurationDays
	lockFund.InterestRate = s.calculateInterestRate(s.convertModelLockTypetToProto(lockFund.LockType), newDurationDays)
	lockFund.Status = models.LockStatusActive

	if err := s.db.WithContext(ctx).Save(&lockFund).Error; err != nil {
		return nil, fmt.Errorf("failed to renew lock fund: %w", err)
	}

	return &lockFund, nil
}

// CancelLockFund cancels a lock fund
func (s *LockFundsService) CancelLockFund(ctx context.Context, userID uint, lockFundID string, reason string) (float64, *models.LockFund, error) {
	var lockFund models.LockFund

	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", lockFundID, userID).
		First(&lockFund).Error; err != nil {
		return 0, nil, errors.New("lock fund not found")
	}

	if lockFund.Status != models.LockStatusActive {
		return 0, nil, errors.New("can only cancel active locks")
	}

	// Calculate refund (principal only, no interest)
	refundAmount := lockFund.Amount * 0.95 // 5% cancellation fee

	lockFund.Status = models.LockStatusCancelled

	if err := s.db.WithContext(ctx).Save(&lockFund).Error; err != nil {
		return 0, nil, fmt.Errorf("failed to cancel lock fund: %w", err)
	}

	// Create cancellation transaction
	transaction := &models.LockFundTransaction{
		LockFundID:      lockFund.ID,
		UserID:          userID,
		TransactionType: "CANCEL",
		Amount:          refundAmount,
		Currency:        lockFund.Currency,
		Status:          "COMPLETED",
		TransactionDate: time.Now(),
		Description:     fmt.Sprintf("Lock fund cancelled: %s", reason),
	}

	s.db.WithContext(ctx).Create(transaction)

	return refundAmount, &lockFund, nil
}

// Helper methods

func (s *LockFundsService) calculateInterestRate(lockType pb.LockType, durationDays int32) float64 {
	baseRate := 0.0

	switch lockType {
	case pb.LockType_LOCK_TYPE_SAVINGS:
		baseRate = 3.0 // 3% annual
	case pb.LockType_LOCK_TYPE_INVESTMENT:
		baseRate = 6.0 // 6% annual
	case pb.LockType_LOCK_TYPE_EMERGENCY_FUND:
		baseRate = 1.5 // 1.5% annual
	case pb.LockType_LOCK_TYPE_GOAL_BASED:
		baseRate = 4.0 // 4% annual
	default:
		baseRate = 2.0
	}

	// Bonus for longer duration
	if durationDays >= 365 {
		baseRate += 1.0
	} else if durationDays >= 180 {
		baseRate += 0.5
	}

	return baseRate
}

func (s *LockFundsService) getEarlyUnlockPenalty(lockType pb.LockType) float64 {
	switch lockType {
	case pb.LockType_LOCK_TYPE_SAVINGS:
		return 2.0 // 2%
	case pb.LockType_LOCK_TYPE_INVESTMENT:
		return 10.0 // 10% - high penalty
	case pb.LockType_LOCK_TYPE_EMERGENCY_FUND:
		return 0.0 // No penalty
	case pb.LockType_LOCK_TYPE_GOAL_BASED:
		return 5.0 // 5%
	default:
		return 3.0
	}
}

func (s *LockFundsService) convertProtoLockType(lockType pb.LockType) models.LockType {
	switch lockType {
	case pb.LockType_LOCK_TYPE_SAVINGS:
		return models.LockTypeSavings
	case pb.LockType_LOCK_TYPE_INVESTMENT:
		return models.LockTypeInvestment
	case pb.LockType_LOCK_TYPE_EMERGENCY_FUND:
		return models.LockTypeEmergencyFund
	case pb.LockType_LOCK_TYPE_GOAL_BASED:
		return models.LockTypeGoalBased
	default:
		return models.LockTypeSavings
	}
}

func (s *LockFundsService) convertModelLockTypetToProto(lockType models.LockType) pb.LockType {
	switch lockType {
	case models.LockTypeSavings:
		return pb.LockType_LOCK_TYPE_SAVINGS
	case models.LockTypeInvestment:
		return pb.LockType_LOCK_TYPE_INVESTMENT
	case models.LockTypeEmergencyFund:
		return pb.LockType_LOCK_TYPE_EMERGENCY_FUND
	case models.LockTypeGoalBased:
		return pb.LockType_LOCK_TYPE_GOAL_BASED
	default:
		return pb.LockType_LOCK_TYPE_SAVINGS
	}
}

func (s *LockFundsService) convertProtoLockStatus(status pb.LockStatus) models.LockStatus {
	switch status {
	case pb.LockStatus_LOCK_STATUS_ACTIVE:
		return models.LockStatusActive
	case pb.LockStatus_LOCK_STATUS_MATURED:
		return models.LockStatusMatured
	case pb.LockStatus_LOCK_STATUS_UNLOCKED:
		return models.LockStatusUnlocked
	case pb.LockStatus_LOCK_STATUS_CANCELLED:
		return models.LockStatusCancelled
	default:
		return models.LockStatusActive
	}
}

// ConvertToProto converts model to protobuf message
func (s *LockFundsService) ConvertToProto(lockFund *models.LockFund) *pb.LockFund {
	return &pb.LockFund{
		Id:                        lockFund.ID,
		UserId:                    uint64(lockFund.UserID),
		LockType:                  s.convertModelLockTypetToProto(lockFund.LockType),
		Amount:                    lockFund.Amount,
		Currency:                  lockFund.Currency,
		LockDurationDays:          lockFund.LockDurationDays,
		InterestRate:              lockFund.InterestRate,
		LockedAt:                  timestamppb.New(lockFund.LockedAt),
		UnlockAt:                  timestamppb.New(lockFund.UnlockAt),
		Status:                    s.convertModelStatusToProto(lockFund.Status),
		AutoRenew:                 lockFund.AutoRenew,
		GoalName:                  lockFund.GoalName,
		GoalDescription:           lockFund.GoalDescription,
		EarlyUnlockPenaltyPercent: lockFund.EarlyUnlockPenaltyPercent,
		AccruedInterest:           lockFund.AccruedInterest,
		PaymentMethod:             lockFund.PaymentMethod,
		TransactionId:             lockFund.TransactionID,
		CreatedAt:                 timestamppb.New(lockFund.CreatedAt),
		UpdatedAt:                 timestamppb.New(lockFund.UpdatedAt),
		DaysRemaining:             lockFund.DaysRemaining(),
		ProgressPercent:           lockFund.ProgressPercent(),
		TotalValue:                lockFund.TotalValue(),
		CanUnlockEarly:            lockFund.CanUnlockEarly(),
	}
}

func (s *LockFundsService) convertModelStatusToProto(status models.LockStatus) pb.LockStatus {
	switch status {
	case models.LockStatusActive:
		return pb.LockStatus_LOCK_STATUS_ACTIVE
	case models.LockStatusMatured:
		return pb.LockStatus_LOCK_STATUS_MATURED
	case models.LockStatusUnlocked:
		return pb.LockStatus_LOCK_STATUS_UNLOCKED
	case models.LockStatusCancelled:
		return pb.LockStatus_LOCK_STATUS_CANCELLED
	default:
		return pb.LockStatus_LOCK_STATUS_ACTIVE
	}
}

// ConvertTransactionToProto converts transaction model to protobuf
func (s *LockFundsService) ConvertTransactionToProto(tx *models.LockFundTransaction) *pb.LockTransaction {
	return &pb.LockTransaction{
		Id:              tx.ID,
		LockFundId:      tx.LockFundID,
		UserId:          uint64(tx.UserID),
		TransactionType: tx.TransactionType,
		Amount:          tx.Amount,
		Currency:        tx.Currency,
		PaymentMethod:   tx.PaymentMethod,
		Status:          tx.Status,
		TransactionDate: timestamppb.New(tx.TransactionDate),
		Description:     tx.Description,
	}
}

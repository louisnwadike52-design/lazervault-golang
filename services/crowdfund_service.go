package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"math"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrCrowdfundNotFound       = errors.New("crowdfund not found")
	ErrCrowdfundAccessDenied   = errors.New("access denied to crowdfund")
	ErrCrowdfundNotActive      = errors.New("crowdfund is not active")
	ErrCrowdfundDeadlinePassed = errors.New("crowdfund deadline has passed")
	ErrDonationNotFound        = errors.New("donation not found")
	ErrReceiptNotFound         = errors.New("receipt not found")
	ErrUserNotVerified         = errors.New("only verified users can create crowdfunds - please complete identity verification")
	ErrCrowdfundInvalidAmount  = errors.New("donation amount must be greater than 0")
	ErrCrowdfundUnauthorized   = errors.New("unauthorized access")
)

// ============================================================================
// INTERFACE
// ============================================================================

type ICrowdfundService interface {
	// Crowdfund Management
	CreateCrowdfund(ctx context.Context, userID uint, req *pb.CreateCrowdfundRequest) (*pb.CrowdfundMessage, error)
	GetCrowdfund(ctx context.Context, userID uint, crowdfundID string) (*pb.CrowdfundMessage, error)
	ListCrowdfunds(ctx context.Context, userID uint, page, pageSize int, statusFilter, categoryFilter string, myOnly bool) ([]*pb.CrowdfundMessage, int64, error)
	SearchCrowdfunds(ctx context.Context, userID uint, query string, limit int) ([]*pb.CrowdfundMessage, error)
	UpdateCrowdfund(ctx context.Context, userID uint, req *pb.UpdateCrowdfundRequest) (*pb.CrowdfundMessage, error)
	DeleteCrowdfund(ctx context.Context, userID uint, crowdfundID string) error

	// Donation Operations
	MakeDonation(ctx context.Context, userID uint, req *pb.MakeDonationRequest) (*pb.CrowdfundDonationMessage, error)
	GetCrowdfundDonations(ctx context.Context, userID uint, crowdfundID string, page, pageSize int) ([]*pb.CrowdfundDonationMessage, int64, error)
	GetUserDonations(ctx context.Context, userID uint, page, pageSize int) ([]*pb.CrowdfundDonationMessage, int64, error)

	// Receipt Operations
	GenerateDonationReceipt(ctx context.Context, userID uint, donationID string) (*pb.CrowdfundReceiptMessage, error)
	GetUserReceipts(ctx context.Context, userID uint, page, pageSize int) ([]*pb.CrowdfundReceiptMessage, int64, error)

	// Statistics
	GetCrowdfundStatistics(ctx context.Context, userID uint, crowdfundID string) (*pb.GetCrowdfundStatisticsResponse, error)
}

// ============================================================================
// SERVICE STRUCT
// ============================================================================

type CrowdfundService struct {
	db *gorm.DB
}

func NewCrowdfundService(db *gorm.DB) ICrowdfundService {
	return &CrowdfundService{db: db}
}

// ============================================================================
// CROWDFUND MANAGEMENT
// ============================================================================

// CreateCrowdfund creates a new crowdfund campaign
// CRITICAL: Only verified users can create crowdfunds
func (s *CrowdfundService) CreateCrowdfund(ctx context.Context, userID uint, req *pb.CreateCrowdfundRequest) (*pb.CrowdfundMessage, error) {
	// CRITICAL: Verify user is verified before allowing crowdfund creation
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCrowdfundUnauthorized
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if !user.Verified {
		return nil, ErrUserNotVerified
	}

	// Generate unique crowdfund code
	crowdfundCode := models.GenerateCrowdfundCode()

	// Ensure code is unique
	for {
		var existing models.Crowdfund
		err := s.db.WithContext(ctx).Where("crowdfund_code = ?", crowdfundCode).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			break // Code is unique
		}
		crowdfundCode = models.GenerateCrowdfundCode() // Try again
	}

	// Create crowdfund
	crowdfund := &models.Crowdfund{
		CreatorUserID: userID,
		Title:         req.Title,
		Description:   req.Description,
		Story:         req.Story,
		CrowdfundCode: crowdfundCode,
		TargetAmount:  int64(req.TargetAmount),
		CurrentAmount: 0,
		Currency:      req.Currency,
		Category:      req.Category,
		Status:        models.CrowdfundStatusActive,
		Visibility:    convertVisibility(req.Visibility),
		DonorCount:    0,
	}

	if req.Deadline != nil {
		deadline := req.Deadline.AsTime()
		crowdfund.Deadline = &deadline
	}

	if req.ImageUrl != "" {
		crowdfund.ImageURL = &req.ImageUrl
	}

	if req.Metadata != "" {
		var metadata models.JSONB
		if err := json.Unmarshal([]byte(req.Metadata), &metadata); err == nil {
			crowdfund.Metadata = metadata
		}
	}

	if err := s.db.WithContext(ctx).Create(crowdfund).Error; err != nil {
		return nil, fmt.Errorf("failed to create crowdfund: %w", err)
	}

	// Reload with creator
	if err := s.db.WithContext(ctx).Preload("Creator").First(crowdfund, "id = ?", crowdfund.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload crowdfund: %w", err)
	}

	return s.crowdfundToProto(crowdfund, userID), nil
}

// GetCrowdfund retrieves a single crowdfund by ID
func (s *CrowdfundService) GetCrowdfund(ctx context.Context, userID uint, crowdfundID string) (*pb.CrowdfundMessage, error) {
	var crowdfund models.Crowdfund
	err := s.db.WithContext(ctx).
		Preload("Creator").
		Preload("Donations", func(db *gorm.DB) *gorm.DB {
			return db.Order("donation_date DESC").Limit(5).Preload("Donor")
		}).
		Where("id = ?", crowdfundID).
		First(&crowdfund).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCrowdfundNotFound
		}
		return nil, fmt.Errorf("failed to get crowdfund: %w", err)
	}

	return s.crowdfundToProto(&crowdfund, userID), nil
}

// ListCrowdfunds retrieves a paginated list of crowdfunds
func (s *CrowdfundService) ListCrowdfunds(ctx context.Context, userID uint, page, pageSize int, statusFilter, categoryFilter string, myOnly bool) ([]*pb.CrowdfundMessage, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	query := s.db.WithContext(ctx).Model(&models.Crowdfund{})

	// Apply filters
	if myOnly {
		query = query.Where("creator_user_id = ?", userID)
	} else {
		// Only show public crowdfunds for general listing
		query = query.Where("visibility = ?", models.CrowdfundVisibilityPublic)
	}

	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	if categoryFilter != "" {
		query = query.Where("category = ?", categoryFilter)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count crowdfunds: %w", err)
	}

	// Get paginated results
	var crowdfunds []models.Crowdfund
	offset := (page - 1) * pageSize
	err := query.
		Preload("Creator").
		Preload("Donations", func(db *gorm.DB) *gorm.DB {
			return db.Order("donation_date DESC").Limit(3).Preload("Donor")
		}).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&crowdfunds).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to list crowdfunds: %w", err)
	}

	// Convert to proto
	protoMessages := make([]*pb.CrowdfundMessage, len(crowdfunds))
	for i, cf := range crowdfunds {
		protoMessages[i] = s.crowdfundToProto(&cf, userID)
	}

	return protoMessages, total, nil
}

// SearchCrowdfunds searches for crowdfunds by username or crowdfund code
// Priority order:
// 1. Exact match on crowdfund_code
// 2. Creator username match (ILIKE)
// 3. Full-text search on title/description
// 4. Order by relevance, then creation date DESC
func (s *CrowdfundService) SearchCrowdfunds(ctx context.Context, userID uint, query string, limit int) ([]*pb.CrowdfundMessage, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return []*pb.CrowdfundMessage{}, nil
	}

	var crowdfunds []models.Crowdfund

	// Search strategy with priority ordering
	err := s.db.WithContext(ctx).
		Preload("Creator").
		Preload("Donations", func(db *gorm.DB) *gorm.DB {
			return db.Order("donation_date DESC").Limit(5).Preload("Donor")
		}).
		Joins("LEFT JOIN users ON crowdfunds.creator_user_id = users.id").
		Where(`
			crowdfunds.status = ? AND
			crowdfunds.visibility = ? AND
			(
				crowdfunds.crowdfund_code ILIKE ? OR
				users.username ILIKE ? OR
				users.first_name ILIKE ? OR
				users.last_name ILIKE ? OR
				crowdfunds.title ILIKE ? OR
				crowdfunds.description ILIKE ?
			)
		`,
			models.CrowdfundStatusActive,
			models.CrowdfundVisibilityPublic,
			"%"+query+"%",
			"%"+query+"%",
			"%"+query+"%",
			"%"+query+"%",
			"%"+query+"%",
			"%"+query+"%",
		).
		Order(fmt.Sprintf("CASE WHEN crowdfunds.crowdfund_code ILIKE '%s%%' THEN 1 ELSE 2 END", query)).
		Order("crowdfunds.created_at DESC").
		Limit(limit).
		Find(&crowdfunds).Error

	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert to proto messages
	protoMessages := make([]*pb.CrowdfundMessage, len(crowdfunds))
	for i, cf := range crowdfunds {
		protoMessages[i] = s.crowdfundToProto(&cf, userID)
	}

	return protoMessages, nil
}

// UpdateCrowdfund updates a crowdfund (only creator can update)
func (s *CrowdfundService) UpdateCrowdfund(ctx context.Context, userID uint, req *pb.UpdateCrowdfundRequest) (*pb.CrowdfundMessage, error) {
	var crowdfund models.Crowdfund
	if err := s.db.WithContext(ctx).First(&crowdfund, "id = ?", req.CrowdfundId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCrowdfundNotFound
		}
		return nil, fmt.Errorf("failed to get crowdfund: %w", err)
	}

	// Check if user is creator
	if crowdfund.CreatorUserID != userID {
		return nil, ErrCrowdfundAccessDenied
	}

	// Update fields
	if req.Title != "" {
		crowdfund.Title = req.Title
	}
	if req.Description != "" {
		crowdfund.Description = req.Description
	}
	if req.Story != "" {
		crowdfund.Story = req.Story
	}
	if req.Deadline != nil {
		deadline := req.Deadline.AsTime()
		crowdfund.Deadline = &deadline
	}
	if req.Status != pb.CrowdfundStatus_CROWDFUND_STATUS_UNSPECIFIED {
		crowdfund.Status = convertStatusFromProto(req.Status)
	}
	if req.ImageUrl != "" {
		crowdfund.ImageURL = &req.ImageUrl
	}
	if req.Metadata != "" {
		var metadata models.JSONB
		if err := json.Unmarshal([]byte(req.Metadata), &metadata); err == nil {
			crowdfund.Metadata = metadata
		}
	}

	if err := s.db.WithContext(ctx).Save(&crowdfund).Error; err != nil {
		return nil, fmt.Errorf("failed to update crowdfund: %w", err)
	}

	// Reload with relationships
	if err := s.db.WithContext(ctx).Preload("Creator").First(&crowdfund, "id = ?", crowdfund.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload crowdfund: %w", err)
	}

	return s.crowdfundToProto(&crowdfund, userID), nil
}

// DeleteCrowdfund soft deletes a crowdfund (only creator can delete)
func (s *CrowdfundService) DeleteCrowdfund(ctx context.Context, userID uint, crowdfundID string) error {
	var crowdfund models.Crowdfund
	if err := s.db.WithContext(ctx).First(&crowdfund, "id = ?", crowdfundID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCrowdfundNotFound
		}
		return fmt.Errorf("failed to get crowdfund: %w", err)
	}

	// Check if user is creator
	if crowdfund.CreatorUserID != userID {
		return ErrCrowdfundAccessDenied
	}

	if err := s.db.WithContext(ctx).Delete(&crowdfund).Error; err != nil {
		return fmt.Errorf("failed to delete crowdfund: %w", err)
	}

	return nil
}

// ============================================================================
// DONATION OPERATIONS
// ============================================================================

// MakeDonation processes a donation to a crowdfund
// Uses database transaction for atomicity
func (s *CrowdfundService) MakeDonation(ctx context.Context, userID uint, req *pb.MakeDonationRequest) (*pb.CrowdfundDonationMessage, error) {
	if req.Amount <= 0 {
		return nil, ErrCrowdfundInvalidAmount
	}

	// Start database transaction
	tx := s.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Verify crowdfund exists and is active
	var crowdfund models.Crowdfund
	if err := tx.First(&crowdfund, "id = ? AND status = ?", req.CrowdfundId, models.CrowdfundStatusActive).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCrowdfundNotActive
		}
		return nil, fmt.Errorf("failed to get crowdfund: %w", err)
	}

	// Check if deadline has passed
	if crowdfund.Deadline != nil && time.Now().After(*crowdfund.Deadline) {
		tx.Rollback()
		return nil, ErrCrowdfundDeadlinePassed
	}

	// Create donation record
	donation := &models.CrowdfundDonation{
		CrowdfundID:  req.CrowdfundId,
		DonorUserID:  userID,
		Amount:       int64(req.Amount),
		Currency:     crowdfund.Currency,
		DonationDate: time.Now(),
		Status:       models.DonationStatusPending,
		IsAnonymous:  req.IsAnonymous,
		PaymentMethod: "account_transfer",
	}

	if req.SourceAccountId > 0 {
		sourceID := uint(req.SourceAccountId)
		donation.SourceAccountID = &sourceID
	}

	if req.Message != "" {
		donation.Message = &req.Message
	}

	// Generate transaction ID
	transactionID := models.GenerateTransactionID("DON")
	donation.TransactionID = &transactionID

	if err := tx.Create(donation).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create donation: %w", err)
	}

	// Update crowdfund totals
	crowdfund.CurrentAmount += donation.Amount
	crowdfund.DonorCount += 1

	// Check if target reached
	if crowdfund.CurrentAmount >= crowdfund.TargetAmount {
		crowdfund.Status = models.CrowdfundStatusCompleted
	}

	if err := tx.Save(&crowdfund).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update crowdfund: %w", err)
	}

	// Mark donation as completed
	donation.Status = models.DonationStatusCompleted
	if err := tx.Save(donation).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update donation status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit donation: %w", err)
	}

	// Reload with relationships
	s.db.WithContext(ctx).Preload("Donor").Preload("Crowdfund.Creator").First(donation, "id = ?", donation.ID)

	return s.donationToProto(donation, userID, crowdfund.CreatorUserID), nil
}

// GetCrowdfundDonations retrieves donations for a crowdfund
func (s *CrowdfundService) GetCrowdfundDonations(ctx context.Context, userID uint, crowdfundID string, page, pageSize int) ([]*pb.CrowdfundDonationMessage, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	// Get crowdfund to check creator
	var crowdfund models.Crowdfund
	if err := s.db.WithContext(ctx).First(&crowdfund, "id = ?", crowdfundID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, ErrCrowdfundNotFound
		}
		return nil, 0, fmt.Errorf("failed to get crowdfund: %w", err)
	}

	query := s.db.WithContext(ctx).Model(&models.CrowdfundDonation{}).Where("crowdfund_id = ?", crowdfundID)

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count donations: %w", err)
	}

	// Get paginated results
	var donations []models.CrowdfundDonation
	offset := (page - 1) * pageSize
	err := query.
		Preload("Donor").
		Preload("Crowdfund.Creator").
		Order("donation_date DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&donations).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to list donations: %w", err)
	}

	// Convert to proto
	protoMessages := make([]*pb.CrowdfundDonationMessage, len(donations))
	for i, d := range donations {
		protoMessages[i] = s.donationToProto(&d, userID, crowdfund.CreatorUserID)
	}

	return protoMessages, total, nil
}

// GetUserDonations retrieves donations made by a user
func (s *CrowdfundService) GetUserDonations(ctx context.Context, userID uint, page, pageSize int) ([]*pb.CrowdfundDonationMessage, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	query := s.db.WithContext(ctx).Model(&models.CrowdfundDonation{}).Where("donor_user_id = ?", userID)

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count donations: %w", err)
	}

	// Get paginated results
	var donations []models.CrowdfundDonation
	offset := (page - 1) * pageSize
	err := query.
		Preload("Crowdfund.Creator").
		Preload("Donor").
		Order("donation_date DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&donations).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to list user donations: %w", err)
	}

	// Convert to proto
	protoMessages := make([]*pb.CrowdfundDonationMessage, len(donations))
	for i, d := range donations {
		protoMessages[i] = s.donationToProto(&d, userID, d.Crowdfund.CreatorUserID)
	}

	return protoMessages, total, nil
}

// ============================================================================
// RECEIPT OPERATIONS
// ============================================================================

// GenerateDonationReceipt generates a receipt for a donation
func (s *CrowdfundService) GenerateDonationReceipt(ctx context.Context, userID uint, donationID string) (*pb.CrowdfundReceiptMessage, error) {
	var donation models.CrowdfundDonation
	err := s.db.WithContext(ctx).
		Preload("Crowdfund").
		Preload("Donor").
		First(&donation, "id = ?", donationID).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDonationNotFound
		}
		return nil, fmt.Errorf("failed to get donation: %w", err)
	}

	// Check if user is donor
	if donation.DonorUserID != userID {
		return nil, ErrUnauthorized
	}

	// Check if receipt already exists
	var existingReceipt models.CrowdfundReceipt
	err = s.db.WithContext(ctx).Where("donation_id = ?", donationID).First(&existingReceipt).Error
	if err == nil {
		// Receipt exists, return it
		return s.receiptToProto(&existingReceipt), nil
	}

	// Generate new receipt
	receipt := &models.CrowdfundReceipt{
		DonationID:    donationID,
		CrowdfundID:   donation.CrowdfundID,
		DonorUserID:   donation.DonorUserID,
		Amount:        donation.Amount,
		Currency:      donation.Currency,
		DonationDate:  donation.DonationDate,
		GeneratedAt:   time.Now(),
		ReceiptNumber: models.GenerateReceiptNumber(),
	}

	// Build receipt data
	receiptData := map[string]interface{}{
		"donation_id":    donationID,
		"crowdfund_id":   donation.CrowdfundID,
		"crowdfund_title": donation.Crowdfund.Title,
		"amount":         donation.Amount,
		"currency":       donation.Currency,
		"donation_date":  donation.DonationDate,
		"transaction_id": donation.TransactionID,
	}
	receipt.ReceiptData = receiptData

	if err := s.db.WithContext(ctx).Create(receipt).Error; err != nil {
		return nil, fmt.Errorf("failed to create receipt: %w", err)
	}

	return s.receiptToProto(receipt), nil
}

// GetUserReceipts retrieves receipts for a user's donations
func (s *CrowdfundService) GetUserReceipts(ctx context.Context, userID uint, page, pageSize int) ([]*pb.CrowdfundReceiptMessage, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	query := s.db.WithContext(ctx).Model(&models.CrowdfundReceipt{}).Where("donor_user_id = ?", userID)

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count receipts: %w", err)
	}

	// Get paginated results
	var receipts []models.CrowdfundReceipt
	offset := (page - 1) * pageSize
	err := query.
		Preload("Crowdfund").
		Preload("Donor").
		Order("generated_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&receipts).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to list receipts: %w", err)
	}

	// Convert to proto
	protoMessages := make([]*pb.CrowdfundReceiptMessage, len(receipts))
	for i, r := range receipts {
		protoMessages[i] = s.receiptToProto(&r)
	}

	return protoMessages, total, nil
}

// ============================================================================
// STATISTICS
// ============================================================================

// GetCrowdfundStatistics retrieves statistics for a crowdfund
func (s *CrowdfundService) GetCrowdfundStatistics(ctx context.Context, userID uint, crowdfundID string) (*pb.GetCrowdfundStatisticsResponse, error) {
	var crowdfund models.Crowdfund
	if err := s.db.WithContext(ctx).First(&crowdfund, "id = ?", crowdfundID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCrowdfundNotFound
		}
		return nil, fmt.Errorf("failed to get crowdfund: %w", err)
	}

	// Calculate progress percentage
	progressPercentage := 0.0
	if crowdfund.TargetAmount > 0 {
		progressPercentage = (float64(crowdfund.CurrentAmount) / float64(crowdfund.TargetAmount)) * 100
	}

	// Get donation statistics
	var averageDonation int64
	var largestDonation int64

	s.db.WithContext(ctx).
		Model(&models.CrowdfundDonation{}).
		Where("crowdfund_id = ? AND status = ?", crowdfundID, models.DonationStatusCompleted).
		Select("AVG(amount)").
		Scan(&averageDonation)

	s.db.WithContext(ctx).
		Model(&models.CrowdfundDonation{}).
		Where("crowdfund_id = ? AND status = ?", crowdfundID, models.DonationStatusCompleted).
		Select("MAX(amount)").
		Scan(&largestDonation)

	// Calculate days remaining
	daysRemaining := 0
	if crowdfund.Deadline != nil {
		duration := time.Until(*crowdfund.Deadline)
		daysRemaining = int(math.Ceil(duration.Hours() / 24))
		if daysRemaining < 0 {
			daysRemaining = 0
		}
	}

	return &pb.GetCrowdfundStatisticsResponse{
		CrowdfundId:        crowdfundID,
		TotalRaised:        uint64(crowdfund.CurrentAmount),
		TargetAmount:       uint64(crowdfund.TargetAmount),
		ProgressPercentage: progressPercentage,
		DonorCount:         int32(crowdfund.DonorCount),
		AverageDonation:    uint64(averageDonation),
		LargestDonation:    uint64(largestDonation),
		DaysRemaining:      int32(daysRemaining),
		IsCompleted:        crowdfund.Status == models.CrowdfundStatusCompleted,
		CreatedAt:          timestamppb.New(crowdfund.CreatedAt),
	}, nil
}

// ============================================================================
// HELPER METHODS - PROTO CONVERSION
// ============================================================================

// crowdfundToProto converts a Crowdfund model to proto message
func (s *CrowdfundService) crowdfundToProto(crowdfund *models.Crowdfund, currentUserID uint) *pb.CrowdfundMessage {
	msg := &pb.CrowdfundMessage{
		Id:            crowdfund.ID,
		CreatorUserId: uint64(crowdfund.CreatorUserID),
		Title:         crowdfund.Title,
		Description:   crowdfund.Description,
		Story:         crowdfund.Story,
		CrowdfundCode: crowdfund.CrowdfundCode,
		TargetAmount:  uint64(crowdfund.TargetAmount),
		CurrentAmount: uint64(crowdfund.CurrentAmount),
		Currency:      crowdfund.Currency,
		Category:      crowdfund.Category,
		Status:        convertStatusToProto(crowdfund.Status),
		Visibility:    convertVisibilityToProto(crowdfund.Visibility),
		DonorCount:    int32(crowdfund.DonorCount),
		CreatedAt:     timestamppb.New(crowdfund.CreatedAt),
		UpdatedAt:     timestamppb.New(crowdfund.UpdatedAt),
	}

	// Calculate progress percentage
	if crowdfund.TargetAmount > 0 {
		msg.ProgressPercentage = (float64(crowdfund.CurrentAmount) / float64(crowdfund.TargetAmount)) * 100
	}

	if crowdfund.Deadline != nil {
		msg.Deadline = timestamppb.New(*crowdfund.Deadline)
	}

	if crowdfund.ImageURL != nil {
		msg.ImageUrl = *crowdfund.ImageURL
	}

	if crowdfund.Metadata != nil {
		if metadataJSON, err := json.Marshal(crowdfund.Metadata); err == nil {
			msg.Metadata = string(metadataJSON)
		}
	}

	// Add creator details
	msg.Creator = s.creatorToProto(&crowdfund.Creator)

	// Add recent donations
	if len(crowdfund.Donations) > 0 {
		msg.RecentDonations = make([]*pb.CrowdfundDonationMessage, len(crowdfund.Donations))
		for i, donation := range crowdfund.Donations {
			msg.RecentDonations[i] = s.donationToProto(&donation, currentUserID, crowdfund.CreatorUserID)
		}
	}

	return msg
}

// creatorToProto converts a User to CrowdfundCreatorMessage
func (s *CrowdfundService) creatorToProto(user *models.User) *pb.CrowdfundCreatorMessage {
	msg := &pb.CrowdfundCreatorMessage{
		UserId:                   uint64(user.ID),
		FirstName:                user.FirstName,
		LastName:                 user.LastName,
		Verified:                 user.Verified,
		FacialRecognitionEnabled: user.FacialRecognitionEnabled,
	}

	if user.Username != nil {
		msg.Username = *user.Username
	}

	if user.ProfilePicture != nil {
		msg.ProfilePicture = *user.ProfilePicture
	}

	if user.VerifiedAt != nil {
		msg.VerifiedAt = timestamppb.New(*user.VerifiedAt)
	}

	return msg
}

// donationToProto converts a CrowdfundDonation to proto message
// Implements privacy abstraction based on viewer context
func (s *CrowdfundService) donationToProto(donation *models.CrowdfundDonation, currentUserID uint, creatorUserID uint) *pb.CrowdfundDonationMessage {
	msg := &pb.CrowdfundDonationMessage{
		Id:            donation.ID,
		CrowdfundId:   donation.CrowdfundID,
		DonorUserId:   uint64(donation.DonorUserID),
		Amount:        uint64(donation.Amount),
		Currency:      donation.Currency,
		DonationDate:  timestamppb.New(donation.DonationDate),
		Status:        convertDonationStatusToProto(donation.Status),
		IsAnonymous:   donation.IsAnonymous,
		PaymentMethod: donation.PaymentMethod,
	}

	if donation.TransactionID != nil {
		msg.TransactionId = *donation.TransactionID
	}

	if donation.ReceiptID != nil {
		msg.ReceiptId = *donation.ReceiptID
	}

	if donation.Message != nil {
		msg.Message = *donation.Message
	}

	if donation.Metadata != nil {
		if metadataJSON, err := json.Marshal(donation.Metadata); err == nil {
			msg.Metadata = string(metadataJSON)
		}
	}

	// Build donor message with privacy abstraction
	isCreatorViewing := currentUserID == creatorUserID
	msg.Donor = s.buildDonorMessage(&donation.Donor, donation.IsAnonymous, isCreatorViewing)

	return msg
}

// buildDonorMessage creates donor message with privacy abstraction
// Anonymous donors: Show "Anonymous Donor"
// Creator viewing: Show full details (name, username, verification)
// Public viewing: Show abstracted details (first name + last initial)
func (s *CrowdfundService) buildDonorMessage(donor *models.User, isAnonymous bool, isCreatorViewing bool) *pb.CrowdfundDonorMessage {
	if isAnonymous {
		return &pb.CrowdfundDonorMessage{
			UserId:      0,
			DisplayName: "Anonymous Donor",
			IsAnonymous: true,
			IsCreator:   false,
		}
	}

	donorMsg := &pb.CrowdfundDonorMessage{
		UserId:      uint64(donor.ID),
		IsAnonymous: false,
		IsCreator:   isCreatorViewing,
	}

	if isCreatorViewing {
		// Full details for creator
		donorMsg.DisplayName = donor.FirstName + " " + donor.LastName
		if donor.Username != nil && *donor.Username != "" {
			donorMsg.DisplayName = *donor.Username + " (" + donor.FirstName + " " + donor.LastName + ")"
		}
	} else {
		// Abstracted details for public
		lastInitial := ""
		if len(donor.LastName) > 0 {
			lastInitial = string(donor.LastName[0]) + "."
		}
		donorMsg.DisplayName = donor.FirstName + " " + lastInitial
	}

	if donor.ProfilePicture != nil {
		donorMsg.ProfilePicture = *donor.ProfilePicture
	}

	return donorMsg
}

// receiptToProto converts a CrowdfundReceipt to proto message
func (s *CrowdfundService) receiptToProto(receipt *models.CrowdfundReceipt) *pb.CrowdfundReceiptMessage {
	msg := &pb.CrowdfundReceiptMessage{
		Id:            receipt.ID,
		DonationId:    receipt.DonationID,
		CrowdfundId:   receipt.CrowdfundID,
		DonorUserId:   uint64(receipt.DonorUserID),
		Amount:        uint64(receipt.Amount),
		Currency:      receipt.Currency,
		DonationDate:  timestamppb.New(receipt.DonationDate),
		GeneratedAt:   timestamppb.New(receipt.GeneratedAt),
		ReceiptNumber: receipt.ReceiptNumber,
	}

	if receipt.Crowdfund.Title != "" {
		msg.CrowdfundTitle = receipt.Crowdfund.Title
	}

	if receipt.Donor.FirstName != "" {
		msg.DonorName = receipt.Donor.FirstName + " " + receipt.Donor.LastName
	}

	if receipt.ReceiptData != nil {
		if receiptDataJSON, err := json.Marshal(receipt.ReceiptData); err == nil {
			msg.ReceiptData = string(receiptDataJSON)
		}
	}

	return msg
}

// ============================================================================
// HELPER METHODS - STATUS CONVERSIONS
// ============================================================================

func convertStatusToProto(status models.CrowdfundStatus) pb.CrowdfundStatus {
	switch status {
	case models.CrowdfundStatusActive:
		return pb.CrowdfundStatus_CROWDFUND_STATUS_ACTIVE
	case models.CrowdfundStatusPaused:
		return pb.CrowdfundStatus_CROWDFUND_STATUS_PAUSED
	case models.CrowdfundStatusCompleted:
		return pb.CrowdfundStatus_CROWDFUND_STATUS_COMPLETED
	case models.CrowdfundStatusCancelled:
		return pb.CrowdfundStatus_CROWDFUND_STATUS_CANCELLED
	default:
		return pb.CrowdfundStatus_CROWDFUND_STATUS_UNSPECIFIED
	}
}

func convertStatusFromProto(status pb.CrowdfundStatus) models.CrowdfundStatus {
	switch status {
	case pb.CrowdfundStatus_CROWDFUND_STATUS_ACTIVE:
		return models.CrowdfundStatusActive
	case pb.CrowdfundStatus_CROWDFUND_STATUS_PAUSED:
		return models.CrowdfundStatusPaused
	case pb.CrowdfundStatus_CROWDFUND_STATUS_COMPLETED:
		return models.CrowdfundStatusCompleted
	case pb.CrowdfundStatus_CROWDFUND_STATUS_CANCELLED:
		return models.CrowdfundStatusCancelled
	default:
		return models.CrowdfundStatusActive
	}
}

func convertVisibilityToProto(visibility models.CrowdfundVisibility) pb.CrowdfundVisibility {
	switch visibility {
	case models.CrowdfundVisibilityPublic:
		return pb.CrowdfundVisibility_CROWDFUND_VISIBILITY_PUBLIC
	case models.CrowdfundVisibilityPrivate:
		return pb.CrowdfundVisibility_CROWDFUND_VISIBILITY_PRIVATE
	case models.CrowdfundVisibilityUnlisted:
		return pb.CrowdfundVisibility_CROWDFUND_VISIBILITY_UNLISTED
	default:
		return pb.CrowdfundVisibility_CROWDFUND_VISIBILITY_PUBLIC
	}
}

func convertVisibility(visibility pb.CrowdfundVisibility) models.CrowdfundVisibility {
	switch visibility {
	case pb.CrowdfundVisibility_CROWDFUND_VISIBILITY_PUBLIC:
		return models.CrowdfundVisibilityPublic
	case pb.CrowdfundVisibility_CROWDFUND_VISIBILITY_PRIVATE:
		return models.CrowdfundVisibilityPrivate
	case pb.CrowdfundVisibility_CROWDFUND_VISIBILITY_UNLISTED:
		return models.CrowdfundVisibilityUnlisted
	default:
		return models.CrowdfundVisibilityPublic
	}
}

func convertDonationStatusToProto(status models.DonationStatus) pb.DonationStatus {
	switch status {
	case models.DonationStatusPending:
		return pb.DonationStatus_DONATION_STATUS_PENDING
	case models.DonationStatusProcessing:
		return pb.DonationStatus_DONATION_STATUS_PROCESSING
	case models.DonationStatusCompleted:
		return pb.DonationStatus_DONATION_STATUS_COMPLETED
	case models.DonationStatusFailed:
		return pb.DonationStatus_DONATION_STATUS_FAILED
	case models.DonationStatusRefunded:
		return pb.DonationStatus_DONATION_STATUS_REFUNDED
	default:
		return pb.DonationStatus_DONATION_STATUS_UNSPECIFIED
	}
}

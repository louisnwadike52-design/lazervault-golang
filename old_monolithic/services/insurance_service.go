package services

import (
	"context"
	"encoding/json"
	"errors"
	"lazervaultGo/models"
	"lazervaultGo/tasks"
	"lazervaultGo/utils"
	"math"
	"time"

	"gorm.io/gorm"
)

// Using common errors from errors.go

// IInsuranceService defines the interface for insurance services
type IInsuranceService interface {
	// Insurance Policy Management
	GetUserInsurances(ctx context.Context, userID uint, page, limit int) ([]models.Insurance, *models.PaginationInfo, error)
	GetInsuranceById(ctx context.Context, id string, userID uint) (*models.Insurance, error)
	CreateInsurance(ctx context.Context, insurance *models.Insurance) error
	UpdateInsurance(ctx context.Context, insurance *models.Insurance, userID uint) error
	DeleteInsurance(ctx context.Context, id string, userID uint) error
	SearchInsurances(ctx context.Context, userID uint, query string, page, limit int) ([]models.Insurance, *models.PaginationInfo, error)

	// Payment Management
	GetInsurancePayments(ctx context.Context, insuranceID string, userID uint, page, limit int) ([]models.InsurancePayment, *models.PaginationInfo, error)
	GetUserPayments(ctx context.Context, userID uint, page, limit int) ([]models.InsurancePayment, *models.PaginationInfo, error)
	CreatePayment(ctx context.Context, payment *models.InsurancePayment) error
	ProcessPayment(ctx context.Context, paymentID string, paymentMethod string, paymentDetails map[string]string, userID uint) (*models.InsurancePayment, error)
	GetPaymentById(ctx context.Context, id string, userID uint) (*models.InsurancePayment, error)
	GetOverduePayments(ctx context.Context, userID uint) ([]models.InsurancePayment, error)

	// Claims Management
	GetInsuranceClaims(ctx context.Context, insuranceID string, userID uint, page, limit int) ([]models.InsuranceClaim, *models.PaginationInfo, error)
	GetUserClaims(ctx context.Context, userID uint, page, limit int) ([]models.InsuranceClaim, *models.PaginationInfo, error)
	CreateClaim(ctx context.Context, claim *models.InsuranceClaim) error
	UpdateClaim(ctx context.Context, claim *models.InsuranceClaim, userID uint) error
	GetClaimById(ctx context.Context, id string, userID uint) (*models.InsuranceClaim, error)

	// Statistics
	GetInsuranceStatistics(ctx context.Context, userID uint) (*models.InsuranceStatistics, error)
	GetPaymentStatistics(ctx context.Context, userID uint, startDate, endDate *time.Time) (*models.PaymentStatistics, error)
}

// InsuranceService implements IInsuranceService
type InsuranceService struct {
	db              *gorm.DB
	taskDistributor tasks.TaskDistributor
}

// NewInsuranceService creates a new insurance service
func NewInsuranceService(db *gorm.DB, taskDistributor tasks.TaskDistributor) IInsuranceService {
	return &InsuranceService{
		db:              db,
		taskDistributor: taskDistributor,
	}
}

// GetUserInsurances retrieves all insurances for a user with pagination
func (s *InsuranceService) GetUserInsurances(ctx context.Context, userID uint, page, limit int) ([]models.Insurance, *models.PaginationInfo, error) {
	var insurances []models.Insurance
	var total int64

	// Set default values
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	offset := (page - 1) * limit

	// Count total records
	if err := s.db.Model(&models.Insurance{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Get paginated records
	if err := s.db.Where("user_id = ?", userID).
		Preload("User").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&insurances).Error; err != nil {
		return nil, nil, err
	}

	// Calculate pagination info
	totalPages := int32(math.Ceil(float64(total) / float64(limit)))
	pagination := &models.PaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   totalPages,
		TotalItems:   int32(total),
		ItemsPerPage: int32(limit),
		HasNext:      page < int(totalPages),
		HasPrev:      page > 1,
	}

	return insurances, pagination, nil
}

// GetInsuranceById retrieves an insurance by ID for a specific user
func (s *InsuranceService) GetInsuranceById(ctx context.Context, id string, userID uint) (*models.Insurance, error) {
	var insurance models.Insurance

	if err := s.db.Where("id = ? AND user_id = ?", id, userID).
		Preload("User").
		Preload("Payments").
		Preload("Claims").
		First(&insurance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInsuranceNotFound
		}
		return nil, err
	}

	return &insurance, nil
}

// CreateInsurance creates a new insurance policy
func (s *InsuranceService) CreateInsurance(ctx context.Context, insurance *models.Insurance) error {
	// Validate insurance type
	if !isValidInsuranceType(insurance.Type) {
		return ErrInvalidInsuranceType
	}

	// Validate amount
	if insurance.PremiumAmount <= 0 || insurance.CoverageAmount <= 0 {
		return ErrInsuranceInvalidAmount
	}

	// Validate date range
	if insurance.StartDate.After(insurance.EndDate) {
		return ErrInvalidDateRange
	}

	// Set default values
	if insurance.Currency == "" {
		insurance.Currency = "USD"
	}
	if insurance.Status == "" {
		insurance.Status = "pending"
	}

	// Create insurance
	if err := s.db.Create(insurance).Error; err != nil {
		return err
	}

	return nil
}

// UpdateInsurance updates an existing insurance policy
func (s *InsuranceService) UpdateInsurance(ctx context.Context, insurance *models.Insurance, userID uint) error {
	// Check if insurance exists and belongs to user
	existingInsurance, err := s.GetInsuranceById(ctx, insurance.ID, userID)
	if err != nil {
		return err
	}

	// Validate insurance type if changed
	if insurance.Type != "" && !isValidInsuranceType(insurance.Type) {
		return ErrInvalidInsuranceType
	}

	// Validate amount if changed
	if insurance.PremiumAmount > 0 && insurance.PremiumAmount <= 0 {
		return ErrInsuranceInvalidAmount
	}
	if insurance.CoverageAmount > 0 && insurance.CoverageAmount <= 0 {
		return ErrInsuranceInvalidAmount
	}

	// Validate date range if changed
	if !insurance.StartDate.IsZero() && !insurance.EndDate.IsZero() && insurance.StartDate.After(insurance.EndDate) {
		return ErrInvalidDateRange
	}

	// Update only non-zero fields
	updates := make(map[string]interface{})
	if insurance.PolicyHolderName != "" {
		updates["policy_holder_name"] = insurance.PolicyHolderName
	}
	if insurance.PolicyHolderEmail != "" {
		updates["policy_holder_email"] = insurance.PolicyHolderEmail
	}
	if insurance.PolicyHolderPhone != "" {
		updates["policy_holder_phone"] = insurance.PolicyHolderPhone
	}
	if insurance.Type != "" {
		updates["type"] = insurance.Type
	}
	if insurance.Provider != "" {
		updates["provider"] = insurance.Provider
	}
	if insurance.ProviderLogo != "" {
		updates["provider_logo"] = insurance.ProviderLogo
	}
	if insurance.PremiumAmount > 0 {
		updates["premium_amount"] = insurance.PremiumAmount
	}
	if insurance.CoverageAmount > 0 {
		updates["coverage_amount"] = insurance.CoverageAmount
	}
	if insurance.Currency != "" {
		updates["currency"] = insurance.Currency
	}
	if !insurance.StartDate.IsZero() {
		updates["start_date"] = insurance.StartDate
	}
	if !insurance.EndDate.IsZero() {
		updates["end_date"] = insurance.EndDate
	}
	if !insurance.NextPaymentDate.IsZero() {
		updates["next_payment_date"] = insurance.NextPaymentDate
	}
	if insurance.Status != "" {
		updates["status"] = insurance.Status
	}
	if insurance.Description != nil {
		updates["description"] = insurance.Description
	}

	// Update insurance
	if err := s.db.Model(existingInsurance).Updates(updates).Error; err != nil {
		return err
	}

	return nil
}

// DeleteInsurance deletes an insurance policy
func (s *InsuranceService) DeleteInsurance(ctx context.Context, id string, userID uint) error {
	// Check if insurance exists and belongs to user
	_, err := s.GetInsuranceById(ctx, id, userID)
	if err != nil {
		return err
	}

	// Delete insurance (this will cascade to payments and claims)
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Insurance{}).Error; err != nil {
		return err
	}

	return nil
}

// SearchInsurances searches for insurances by query
func (s *InsuranceService) SearchInsurances(ctx context.Context, userID uint, query string, page, limit int) ([]models.Insurance, *models.PaginationInfo, error) {
	var insurances []models.Insurance
	var total int64

	// Set default values
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	offset := (page - 1) * limit

	// Build search query
	searchQuery := s.db.Where("user_id = ?", userID)
	if query != "" {
		searchQuery = searchQuery.Where(
			"policy_number ILIKE ? OR policy_holder_name ILIKE ? OR provider ILIKE ? OR type ILIKE ?",
			"%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%",
		)
	}

	// Count total records
	if err := searchQuery.Model(&models.Insurance{}).Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Get paginated records
	if err := searchQuery.
		Preload("User").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&insurances).Error; err != nil {
		return nil, nil, err
	}

	// Calculate pagination info
	totalPages := int32(math.Ceil(float64(total) / float64(limit)))
	pagination := &models.PaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   totalPages,
		TotalItems:   int32(total),
		ItemsPerPage: int32(limit),
		HasNext:      page < int(totalPages),
		HasPrev:      page > 1,
	}

	return insurances, pagination, nil
}

// GetInsurancePayments retrieves payments for a specific insurance
func (s *InsuranceService) GetInsurancePayments(ctx context.Context, insuranceID string, userID uint, page, limit int) ([]models.InsurancePayment, *models.PaginationInfo, error) {
	var payments []models.InsurancePayment
	var total int64

	// Set default values
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	offset := (page - 1) * limit

	// Count total records
	if err := s.db.Model(&models.InsurancePayment{}).
		Joins("JOIN insurances ON insurance_payments.insurance_id = insurances.id").
		Where("insurance_payments.insurance_id = ? AND insurances.user_id = ?", insuranceID, userID).
		Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Get paginated records
	if err := s.db.Joins("JOIN insurances ON insurance_payments.insurance_id = insurances.id").
		Where("insurance_payments.insurance_id = ? AND insurances.user_id = ?", insuranceID, userID).
		Preload("Insurance").
		Preload("User").
		Order("insurance_payments.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&payments).Error; err != nil {
		return nil, nil, err
	}

	// Calculate pagination info
	totalPages := int32(math.Ceil(float64(total) / float64(limit)))
	pagination := &models.PaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   totalPages,
		TotalItems:   int32(total),
		ItemsPerPage: int32(limit),
		HasNext:      page < int(totalPages),
		HasPrev:      page > 1,
	}

	return payments, pagination, nil
}

// GetUserPayments retrieves all payments for a user
func (s *InsuranceService) GetUserPayments(ctx context.Context, userID uint, page, limit int) ([]models.InsurancePayment, *models.PaginationInfo, error) {
	var payments []models.InsurancePayment
	var total int64

	// Set default values
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	offset := (page - 1) * limit

	// Count total records
	if err := s.db.Model(&models.InsurancePayment{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Get paginated records
	if err := s.db.Where("user_id = ?", userID).
		Preload("Insurance").
		Preload("User").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&payments).Error; err != nil {
		return nil, nil, err
	}

	// Calculate pagination info
	totalPages := int32(math.Ceil(float64(total) / float64(limit)))
	pagination := &models.PaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   totalPages,
		TotalItems:   int32(total),
		ItemsPerPage: int32(limit),
		HasNext:      page < int(totalPages),
		HasPrev:      page > 1,
	}

	return payments, pagination, nil
}

// CreatePayment creates a new payment
func (s *InsuranceService) CreatePayment(ctx context.Context, payment *models.InsurancePayment) error {
	// Validate payment method
	if !isValidPaymentMethod(payment.PaymentMethod) {
		return ErrInvalidPaymentMethod
	}

	// Validate amount
	if payment.Amount <= 0 {
		return ErrInsuranceInvalidAmount
	}

	// Set default values
	if payment.Currency == "" {
		payment.Currency = "USD"
	}
	if payment.Status == "" {
		payment.Status = "pending"
	}
	if payment.PaymentDate.IsZero() {
		payment.PaymentDate = time.Now()
	}

	// Create payment
	if err := s.db.Create(payment).Error; err != nil {
		return err
	}

	return nil
}

// ProcessPayment processes a payment
func (s *InsuranceService) ProcessPayment(ctx context.Context, paymentID string, paymentMethod string, paymentDetails map[string]string, userID uint) (*models.InsurancePayment, error) {
	// Get payment
	payment, err := s.GetPaymentById(ctx, paymentID, userID)
	if err != nil {
		return nil, err
	}

	// Check if payment is already processed
	if payment.Status == "completed" || payment.Status == "failed" {
		return nil, ErrInsurancePaymentAlreadyProcessed
	}

	// Validate payment method
	if !isValidPaymentMethod(paymentMethod) {
		return nil, ErrInvalidPaymentMethod
	}

	// Update payment status to processing
	payment.Status = "processing"
	payment.PaymentMethod = paymentMethod

	// Convert payment details to JSON
	paymentDetailsJSON, err := json.Marshal(paymentDetails)
	if err != nil {
		return nil, err
	}
	payment.PaymentDetails = models.JSON(paymentDetailsJSON)

	if err := s.db.Save(payment).Error; err != nil {
		return nil, err
	}

	// Simulate payment processing (in real implementation, integrate with payment gateway)
	// For now, we'll simulate a successful payment
	now := time.Now()
	payment.Status = "completed"
	payment.ProcessedAt = &now

	// Generate transaction ID and reference number
	randomStr1, err := utils.GenerateRandomString(8)
	if err != nil {
		return nil, err
	}
	randomStr2, err := utils.GenerateRandomString(8)
	if err != nil {
		return nil, err
	}

	transactionID := "TXN-" + time.Now().Format("20060102150405") + "-" + randomStr1
	referenceNumber := "REF-" + time.Now().Format("20060102150405") + "-" + randomStr2
	payment.TransactionID = &transactionID
	payment.ReferenceNumber = &referenceNumber

	// Generate receipt URL (in real implementation, generate actual receipt)
	receiptURL := "https://api.lazervault.com/receipts/" + payment.ID + ".pdf"
	payment.ReceiptURL = &receiptURL

	// Save updated payment
	if err := s.db.Save(payment).Error; err != nil {
		return nil, err
	}

	// Update insurance next payment date
	var insurance models.Insurance
	if err := s.db.Where("id = ?", payment.InsuranceID).First(&insurance).Error; err == nil {
		// Set next payment date to 1 month from now
		insurance.NextPaymentDate = time.Now().AddDate(0, 1, 0)
		s.db.Save(&insurance)
	}

	return payment, nil
}

// GetPaymentById retrieves a payment by ID
func (s *InsuranceService) GetPaymentById(ctx context.Context, id string, userID uint) (*models.InsurancePayment, error) {
	var payment models.InsurancePayment

	if err := s.db.Where("id = ? AND user_id = ?", id, userID).
		Preload("Insurance").
		Preload("User").
		First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}

	return &payment, nil
}

// GetOverduePayments retrieves overdue payments for a user
func (s *InsuranceService) GetOverduePayments(ctx context.Context, userID uint) ([]models.InsurancePayment, error) {
	var payments []models.InsurancePayment

	if err := s.db.Where("user_id = ? AND due_date < ? AND status IN (?, ?)",
		userID, time.Now(), "pending", "processing").
		Preload("Insurance").
		Preload("User").
		Order("due_date ASC").
		Find(&payments).Error; err != nil {
		return nil, err
	}

	return payments, nil
}

// GetInsuranceClaims retrieves claims for a specific insurance
func (s *InsuranceService) GetInsuranceClaims(ctx context.Context, insuranceID string, userID uint, page, limit int) ([]models.InsuranceClaim, *models.PaginationInfo, error) {
	var claims []models.InsuranceClaim
	var total int64

	// Set default values
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	offset := (page - 1) * limit

	// Count total records
	if err := s.db.Model(&models.InsuranceClaim{}).
		Joins("JOIN insurances ON insurance_claims.insurance_id = insurances.id").
		Where("insurance_claims.insurance_id = ? AND insurances.user_id = ?", insuranceID, userID).
		Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Get paginated records
	if err := s.db.Joins("JOIN insurances ON insurance_claims.insurance_id = insurances.id").
		Where("insurance_claims.insurance_id = ? AND insurances.user_id = ?", insuranceID, userID).
		Preload("Insurance").
		Preload("User").
		Order("insurance_claims.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&claims).Error; err != nil {
		return nil, nil, err
	}

	// Calculate pagination info
	totalPages := int32(math.Ceil(float64(total) / float64(limit)))
	pagination := &models.PaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   totalPages,
		TotalItems:   int32(total),
		ItemsPerPage: int32(limit),
		HasNext:      page < int(totalPages),
		HasPrev:      page > 1,
	}

	return claims, pagination, nil
}

// GetUserClaims retrieves all claims for a user
func (s *InsuranceService) GetUserClaims(ctx context.Context, userID uint, page, limit int) ([]models.InsuranceClaim, *models.PaginationInfo, error) {
	var claims []models.InsuranceClaim
	var total int64

	// Set default values
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	offset := (page - 1) * limit

	// Count total records
	if err := s.db.Model(&models.InsuranceClaim{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, nil, err
	}

	// Get paginated records
	if err := s.db.Where("user_id = ?", userID).
		Preload("Insurance").
		Preload("User").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&claims).Error; err != nil {
		return nil, nil, err
	}

	// Calculate pagination info
	totalPages := int32(math.Ceil(float64(total) / float64(limit)))
	pagination := &models.PaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   totalPages,
		TotalItems:   int32(total),
		ItemsPerPage: int32(limit),
		HasNext:      page < int(totalPages),
		HasPrev:      page > 1,
	}

	return claims, pagination, nil
}

// CreateClaim creates a new claim
func (s *InsuranceService) CreateClaim(ctx context.Context, claim *models.InsuranceClaim) error {
	// Validate claim amount
	if claim.ClaimAmount <= 0 {
		return ErrInsuranceInvalidAmount
	}

	// Set default values
	if claim.Currency == "" {
		claim.Currency = "USD"
	}
	if claim.Status == "" {
		claim.Status = "submitted"
	}

	// Create claim
	if err := s.db.Create(claim).Error; err != nil {
		return err
	}

	return nil
}

// UpdateClaim updates an existing claim
func (s *InsuranceService) UpdateClaim(ctx context.Context, claim *models.InsuranceClaim, userID uint) error {
	// Check if claim exists and belongs to user
	existingClaim, err := s.GetClaimById(ctx, claim.ID, userID)
	if err != nil {
		return err
	}

	// Check if claim is already processed
	if existingClaim.Status == "approved" || existingClaim.Status == "rejected" || existingClaim.Status == "settled" {
		return ErrClaimAlreadyProcessed
	}

	// Update only allowed fields
	updates := make(map[string]interface{})
	if claim.Title != "" {
		updates["title"] = claim.Title
	}
	if claim.Description != "" {
		updates["description"] = claim.Description
	}
	if claim.ClaimAmount > 0 {
		updates["claim_amount"] = claim.ClaimAmount
	}
	if !claim.IncidentDate.IsZero() {
		updates["incident_date"] = claim.IncidentDate
	}
	if claim.IncidentLocation != "" {
		updates["incident_location"] = claim.IncidentLocation
	}

	// Update claim
	if err := s.db.Model(existingClaim).Updates(updates).Error; err != nil {
		return err
	}

	return nil
}

// GetClaimById retrieves a claim by ID
func (s *InsuranceService) GetClaimById(ctx context.Context, id string, userID uint) (*models.InsuranceClaim, error) {
	var claim models.InsuranceClaim

	if err := s.db.Where("id = ? AND user_id = ?", id, userID).
		Preload("Insurance").
		Preload("User").
		First(&claim).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrClaimNotFound
		}
		return nil, err
	}

	return &claim, nil
}

// GetInsuranceStatistics retrieves insurance statistics for a user
func (s *InsuranceService) GetInsuranceStatistics(ctx context.Context, userID uint) (*models.InsuranceStatistics, error) {
	var stats models.InsuranceStatistics
	var policiesByType []struct {
		Type  string `json:"type"`
		Count int    `json:"count"`
	}

	// Use temporary variables for database queries
	var totalPolicies, activePolicies, expiredPolicies int64

	// Get total policies
	if err := s.db.Model(&models.Insurance{}).Where("user_id = ?", userID).Count(&totalPolicies).Error; err != nil {
		return nil, err
	}
	stats.TotalPolicies = int(totalPolicies)

	// Get active policies
	if err := s.db.Model(&models.Insurance{}).Where("user_id = ? AND status = ?", userID, "active").Count(&activePolicies).Error; err != nil {
		return nil, err
	}
	stats.ActivePolicies = int(activePolicies)

	// Get expired policies
	if err := s.db.Model(&models.Insurance{}).Where("user_id = ? AND end_date < ?", userID, time.Now()).Count(&expiredPolicies).Error; err != nil {
		return nil, err
	}
	stats.ExpiredPolicies = int(expiredPolicies)

	// Get total coverage amount
	if err := s.db.Model(&models.Insurance{}).Where("user_id = ?", userID).Select("COALESCE(SUM(coverage_amount), 0)").Scan(&stats.TotalCoverageAmount).Error; err != nil {
		return nil, err
	}

	// Get total premium amount
	if err := s.db.Model(&models.Insurance{}).Where("user_id = ?", userID).Select("COALESCE(SUM(premium_amount), 0)").Scan(&stats.TotalPremiumAmount).Error; err != nil {
		return nil, err
	}

	// Get policies by type
	if err := s.db.Model(&models.Insurance{}).
		Select("type, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("type").
		Scan(&policiesByType).Error; err != nil {
		return nil, err
	}

	// Convert to map
	stats.PoliciesByType = make(map[string]int)
	for _, pt := range policiesByType {
		stats.PoliciesByType[pt.Type] = pt.Count
	}

	return &stats, nil
}

// GetPaymentStatistics retrieves payment statistics for a user
func (s *InsuranceService) GetPaymentStatistics(ctx context.Context, userID uint, startDate, endDate *time.Time) (*models.PaymentStatistics, error) {
	var stats models.PaymentStatistics
	var paymentsByMethod []struct {
		PaymentMethod string `json:"payment_method"`
		Count         int    `json:"count"`
	}

	// Build date filter
	dateFilter := s.db.Where("user_id = ?", userID)
	if startDate != nil {
		dateFilter = dateFilter.Where("created_at >= ?", startDate)
	}
	if endDate != nil {
		dateFilter = dateFilter.Where("created_at <= ?", endDate)
	}

	// Use temporary variables for database queries
	var totalPayments, completedPayments, pendingPayments, failedPayments int64

	// Get total payments
	if err := dateFilter.Model(&models.InsurancePayment{}).Count(&totalPayments).Error; err != nil {
		return nil, err
	}
	stats.TotalPayments = int(totalPayments)

	// Get completed payments
	if err := dateFilter.Model(&models.InsurancePayment{}).Where("status = ?", "completed").Count(&completedPayments).Error; err != nil {
		return nil, err
	}
	stats.CompletedPayments = int(completedPayments)

	// Get pending payments
	if err := dateFilter.Model(&models.InsurancePayment{}).Where("status = ?", "pending").Count(&pendingPayments).Error; err != nil {
		return nil, err
	}
	stats.PendingPayments = int(pendingPayments)

	// Get failed payments
	if err := dateFilter.Model(&models.InsurancePayment{}).Where("status = ?", "failed").Count(&failedPayments).Error; err != nil {
		return nil, err
	}
	stats.FailedPayments = int(failedPayments)

	// Get total amount
	if err := dateFilter.Model(&models.InsurancePayment{}).Select("COALESCE(SUM(amount), 0)").Scan(&stats.TotalAmount).Error; err != nil {
		return nil, err
	}

	// Get completed amount
	if err := dateFilter.Model(&models.InsurancePayment{}).Where("status = ?", "completed").Select("COALESCE(SUM(amount), 0)").Scan(&stats.CompletedAmount).Error; err != nil {
		return nil, err
	}

	// Get payments by method
	if err := dateFilter.Model(&models.InsurancePayment{}).
		Select("payment_method, COUNT(*) as count").
		Group("payment_method").
		Scan(&paymentsByMethod).Error; err != nil {
		return nil, err
	}

	// Convert to map
	stats.PaymentsByMethod = make(map[string]int)
	for _, pm := range paymentsByMethod {
		stats.PaymentsByMethod[pm.PaymentMethod] = pm.Count
	}

	return &stats, nil
}

// Helper functions
func isValidInsuranceType(insuranceType string) bool {
	validTypes := []string{"health", "auto", "home", "life", "travel", "business"}
	for _, t := range validTypes {
		if t == insuranceType {
			return true
		}
	}
	return false
}

func isValidPaymentMethod(paymentMethod string) bool {
	validMethods := []string{"bank_transfer", "card", "mobile_money", "crypto", "wallet"}
	for _, m := range validMethods {
		if m == paymentMethod {
			return true
		}
	}
	return false
}

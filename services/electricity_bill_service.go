package services

import (
	"context"
	"encoding/json"
	"fmt"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/tasks"
	"lazervaultGo/token"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)


// ElectricityBillService implements the electricity bill payment service
type ElectricityBillService struct {
	pb.UnimplementedElectricityBillServiceServer
	db              *gorm.DB
	providerFactory *BillPaymentProviderFactory
	taskDistributor tasks.TaskDistributor
}

// NewElectricityBillService creates a new service instance
func NewElectricityBillService(
	db *gorm.DB,
	providerFactory *BillPaymentProviderFactory,
	distributor tasks.TaskDistributor,
) *ElectricityBillService {
	return &ElectricityBillService{
		db:              db,
		providerFactory: providerFactory,
		taskDistributor: distributor,
	}
}

// GetProviders retrieves available electricity providers
func (s *ElectricityBillService) GetProviders(ctx context.Context, req *pb.GetProvidersRequest) (*pb.GetProvidersResponse, error) {
	var providers []models.ElectricityProvider

	// Try raw SQL first for debugging
	rawSQL := "SELECT * FROM electricity_providers WHERE is_active = true"
	params := []interface{}{}

	if req.Country != "" {
		rawSQL += " AND country = ?"
		params = append(params, req.Country)
	}

	if req.PaymentGateway != "" {
		rawSQL += " AND payment_gateway = ?"
		params = append(params, req.PaymentGateway)
	}

	log.Printf("Executing SQL: %s with params: %v", rawSQL, params)

	if err := s.db.WithContext(ctx).Raw(rawSQL, params...).Scan(&providers).Error; err != nil {
		log.Printf("Error fetching providers: %v", err)
		return nil, status.Error(codes.Internal, "failed to fetch providers")
	}

	// Debug log
	log.Printf("Found %d providers (country=%s, gateway=%s)", len(providers), req.Country, req.PaymentGateway)
	if len(providers) > 0 {
		log.Printf("First provider: %+v", providers[0])
	}

	// Try direct query to verify table exists
	var count int64
	s.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM electricity_providers").Scan(&count)
	log.Printf("Total rows in electricity_providers table: %d", count)

	// Check search_path
	var searchPath string
	s.db.WithContext(ctx).Raw("SHOW search_path").Scan(&searchPath)
	log.Printf("Current search_path: %s", searchPath)

	// Check current database
	var currentDB string
	s.db.WithContext(ctx).Raw("SELECT current_database()").Scan(&currentDB)
	log.Printf("Current database: %s", currentDB)

	pbProviders := make([]*pb.ElectricityProvider, len(providers))
	for i, p := range providers {
		pbProviders[i] = &pb.ElectricityProvider{
			Id:             p.ID,
			ProviderCode:   p.ProviderCode,
			ProviderName:   p.ProviderName,
			Country:        p.Country,
			LogoUrl:        p.LogoURL,
			IsActive:       p.IsActive,
			PaymentGateway: p.PaymentGateway,
			MinAmount:      p.MinAmount,
			MaxAmount:      p.MaxAmount,
			ServiceFee:     p.ServiceFee,
			FeeType:        p.FeeType,
			CreatedAt:      timestamppb.New(p.CreatedAt),
		}
	}

	return &pb.GetProvidersResponse{
		Providers: pbProviders,
	}, nil
}

// ValidateMeterNumber validates a meter number with the provider
func (s *ElectricityBillService) ValidateMeterNumber(ctx context.Context, req *pb.ValidateMeterRequest) (*pb.ValidateMeterResponse, error) {
	var provider models.ElectricityProvider
	if err := s.db.Where("provider_code = ?", req.ProviderCode).First(&provider).Error; err != nil {
		return nil, status.Error(codes.NotFound, "provider not found")
	}

	client := s.providerFactory.GetProvider(provider.PaymentGateway)

	validationReq := MeterValidationRequest{
		ProviderCode: req.ProviderCode,
		MeterNumber:  req.MeterNumber,
		MeterType:    req.MeterType,
	}

	result, err := client.ValidateMeter(ctx, validationReq)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("validation failed: %v", err))
	}

	return &pb.ValidateMeterResponse{
		IsValid:            result.IsValid,
		CustomerName:       result.CustomerName,
		CustomerAddress:    result.CustomerAddress,
		MeterType:          result.MeterType,
		OutstandingBalance: result.OutstandingDebt,
		Message:            result.Message,
		MeterNumber:        req.MeterNumber,
	}, nil
}

// InitiatePayment starts a bill payment transaction
func (s *ElectricityBillService) InitiatePayment(ctx context.Context, req *pb.InitiatePaymentRequest) (*pb.InitiatePaymentResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	var provider models.ElectricityProvider
	if err := s.db.Where("provider_code = ?", req.ProviderCode).First(&provider).Error; err != nil {
		return nil, status.Error(codes.NotFound, "provider not found")
	}

	// Validate amount
	if req.Amount < provider.MinAmount || req.Amount > provider.MaxAmount {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("amount must be between %.2f and %.2f", provider.MinAmount, provider.MaxAmount))
	}

	// Calculate service fee
	serviceFee := provider.ServiceFee
	if provider.FeeType == "percentage" {
		serviceFee = req.Amount * (provider.ServiceFee / 100)
	}
	totalAmount := req.Amount + serviceFee

	// Generate reference number
	refNumber := fmt.Sprintf("ELEC-%s-%d", provider.ProviderCode, time.Now().Unix())

	// Create payment record
	payment := &models.BillPayment{
		UserID:          user.ID,
		ProviderID:      provider.ID,
		ProviderCode:    provider.ProviderCode,
		ProviderName:    provider.ProviderName,
		MeterNumber:     req.MeterNumber,
		Amount:          req.Amount,
		ServiceFee:      serviceFee,
		TotalAmount:     totalAmount,
		Currency:        req.Currency,
		Status:          models.BillPaymentStatusPending,
		PaymentGateway:  req.PaymentGateway,
		ReferenceNumber: refNumber,
		MeterType:       req.MeterType,
	}

	if err := s.db.Create(payment).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to create payment record")
	}

	// Enqueue async payment processing task
	paymentPayload := map[string]interface{}{
		"payment_id":       payment.ID,
		"provider_code":    req.ProviderCode,
		"meter_number":     req.MeterNumber,
		"amount":           req.Amount,
		"payment_gateway":  req.PaymentGateway,
		"reference_number": refNumber,
	}

	jsonPayload, _ := json.Marshal(paymentPayload)
	if err := s.taskDistributor.DistributeTask(
		ctx,
		tasks.TaskProcessBillPayment,
		jsonPayload,
		asynq.Queue(tasks.QueueCritical),
	); err != nil {
		// Log error but don't fail - user can retry
		fmt.Printf("Failed to enqueue payment task: %v\n", err)
	}

	return &pb.InitiatePaymentResponse{
		PaymentId:      payment.ID,
		ReferenceNumber: refNumber,
		Status:         string(payment.Status),
		TotalAmount:    totalAmount,
		ServiceFee:     serviceFee,
		Message:        "Payment initiated successfully",
	}, nil
}

// VerifyPayment verifies the status of a payment
func (s *ElectricityBillService) VerifyPayment(ctx context.Context, req *pb.VerifyPaymentRequest) (*pb.VerifyPaymentResponse, error) {
	var payment models.BillPayment
	query := s.db.Preload("Provider")

	if req.PaymentId != "" {
		query = query.Where("id = ?", req.PaymentId)
	} else if req.ReferenceNumber != "" {
		query = query.Where("reference_number = ?", req.ReferenceNumber)
	} else {
		return nil, status.Error(codes.InvalidArgument, "payment_id or reference_number required")
	}

	if err := query.First(&payment).Error; err != nil {
		return nil, status.Error(codes.NotFound, "payment not found")
	}

	return &pb.VerifyPaymentResponse{
		Payment: convertPaymentToProto(&payment),
		Message: "Payment verification successful",
	}, nil
}

// GetPaymentHistory retrieves user's payment history
func (s *ElectricityBillService) GetPaymentHistory(ctx context.Context, req *pb.GetPaymentHistoryRequest) (*pb.GetPaymentHistoryResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	query := s.db.Where("user_id = ?", user.ID)

	if req.ProviderCode != "" {
		query = query.Where("provider_code = ?", req.ProviderCode)
	}

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// Get total count
	var total int64
	query.Model(&models.BillPayment{}).Count(&total)

	// Apply pagination
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	var payments []models.BillPayment
	if err := query.Order("created_at DESC").Limit(int(limit)).Offset(int(req.Offset)).Find(&payments).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch payment history")
	}

	pbPayments := make([]*pb.BillPayment, len(payments))
	for i, p := range payments {
		pbPayments[i] = convertPaymentToProto(&p)
	}

	return &pb.GetPaymentHistoryResponse{
		Payments: pbPayments,
		Total:    int32(total),
	}, nil
}

// SaveBeneficiary saves a meter number for quick payments
func (s *ElectricityBillService) SaveBeneficiary(ctx context.Context, req *pb.SaveBeneficiaryRequest) (*pb.SaveBeneficiaryResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	// Use provided provider_id or look up by provider_code
	var providerID, providerName string
	if req.ProviderId != "" {
		providerID = req.ProviderId
		if req.ProviderName != "" {
			providerName = req.ProviderName
		} else {
			// Look up provider name if not provided
			var provider models.ElectricityProvider
			if err := s.db.Where("id = ?", req.ProviderId).First(&provider).Error; err == nil {
				providerName = provider.ProviderName
			}
		}
	} else {
		// Fall back to looking up by provider code
		var provider models.ElectricityProvider
		if err := s.db.Where("provider_code = ?", req.ProviderCode).First(&provider).Error; err != nil {
			return nil, status.Error(codes.NotFound, "provider not found")
		}
		providerID = provider.ID
		providerName = provider.ProviderName
	}

	// If this beneficiary should be default, unset others
	if req.IsDefault {
		s.db.Model(&models.BillBeneficiary{}).Where("user_id = ?", user.ID).Update("is_default", false)
	}

	beneficiary := &models.BillBeneficiary{
		UserID:          user.ID,
		ProviderID:      providerID,
		ProviderCode:    req.ProviderCode,
		ProviderName:    providerName,
		MeterNumber:     req.MeterNumber,
		CustomerName:    req.CustomerName,
		CustomerAddress: req.CustomerAddress,
		Nickname:        req.Nickname,
		MeterType:       req.MeterType,
		IsDefault:       req.IsDefault,
	}

	if err := s.db.Create(beneficiary).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to save beneficiary")
	}

	return &pb.SaveBeneficiaryResponse{
		Beneficiary: convertBeneficiaryToProto(beneficiary),
		Message:     "Beneficiary saved successfully",
	}, nil
}

// GetBeneficiaries retrieves user's saved beneficiaries
func (s *ElectricityBillService) GetBeneficiaries(ctx context.Context, req *pb.GetBeneficiariesRequest) (*pb.GetBeneficiariesResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	query := s.db.Where("user_id = ?", user.ID)

	if req.ProviderCode != "" {
		query = query.Where("provider_code = ?", req.ProviderCode)
	}

	var beneficiaries []models.BillBeneficiary
	if err := query.Order("is_default DESC, created_at DESC").Find(&beneficiaries).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch beneficiaries")
	}

	pbBeneficiaries := make([]*pb.BillBeneficiary, len(beneficiaries))
	for i, b := range beneficiaries {
		pbBeneficiaries[i] = convertBeneficiaryToProto(&b)
	}

	return &pb.GetBeneficiariesResponse{
		Beneficiaries: pbBeneficiaries,
	}, nil
}

// Helper functions to convert models to proto messages
func convertPaymentToProto(p *models.BillPayment) *pb.BillPayment {
	result := &pb.BillPayment{
		Id:               p.ID,
		UserId:           fmt.Sprintf("%d", p.UserID),
		ProviderCode:     p.ProviderCode,
		ProviderName:     p.ProviderName,
		MeterNumber:      p.MeterNumber,
		CustomerName:     p.CustomerName,
		CustomerAddress:  p.CustomerAddress,
		Amount:           p.Amount,
		ServiceFee:       p.ServiceFee,
		TotalAmount:      p.TotalAmount,
		Currency:         p.Currency,
		Status:           string(p.Status),
		PaymentGateway:   p.PaymentGateway,
		GatewayReference: p.GatewayReference,
		ReferenceNumber:  p.ReferenceNumber,
		Token:            p.Token,
		Units:            p.Units,
		MeterType:        p.MeterType,
		FailureReason:    p.FailureReason,
		CreatedAt:        timestamppb.New(p.CreatedAt),
		ProviderId:       p.ProviderID,
		UpdatedAt:        timestamppb.New(p.UpdatedAt),
		ErrorMessage:     p.FailureReason,
	}

	if p.CompletedAt != nil {
		result.CompletedAt = timestamppb.New(*p.CompletedAt)
	}
	if p.FailedAt != nil {
		result.FailedAt = timestamppb.New(*p.FailedAt)
	}

	return result
}

func convertBeneficiaryToProto(b *models.BillBeneficiary) *pb.BillBeneficiary {
	result := &pb.BillBeneficiary{
		Id:              b.ID,
		UserId:          fmt.Sprintf("%d", b.UserID),
		ProviderCode:    b.ProviderCode,
		ProviderName:    b.ProviderName,
		MeterNumber:     b.MeterNumber,
		CustomerName:    b.CustomerName,
		Nickname:        b.Nickname,
		MeterType:       b.MeterType,
		IsDefault:       b.IsDefault,
		CreatedAt:       timestamppb.New(b.CreatedAt),
		ProviderId:      b.ProviderID,
		CustomerAddress: b.CustomerAddress,
		UpdatedAt:       timestamppb.New(b.UpdatedAt),
	}

	if b.LastUsedAt != nil {
		result.LastUsedAt = timestamppb.New(*b.LastUsedAt)
	}

	return result
}

// GetBillDetails retrieves outstanding bill information
func (s *ElectricityBillService) GetBillDetails(ctx context.Context, req *pb.GetBillDetailsRequest) (*pb.GetBillDetailsResponse, error) {
	var provider models.ElectricityProvider
	if err := s.db.Where("provider_code = ?", req.ProviderCode).First(&provider).Error; err != nil {
		return nil, status.Error(codes.NotFound, "provider not found")
	}

	// For now, return empty bill details - this would need provider-specific API calls
	return &pb.GetBillDetailsResponse{
		CustomerName:      "",
		CustomerAddress:   "",
		OutstandingAmount: 0,
		DueDate:           "",
		MeterType:         "",
	}, nil
}

// GetPaymentReceipt retrieves payment receipt data
func (s *ElectricityBillService) GetPaymentReceipt(ctx context.Context, req *pb.GetPaymentReceiptRequest) (*pb.GetPaymentReceiptResponse, error) {
	var payment models.BillPayment
	if err := s.db.Where("id = ?", req.PaymentId).First(&payment).Error; err != nil {
		return nil, status.Error(codes.NotFound, "payment not found")
	}

	receiptData := &pb.ReceiptData{
		ReceiptNumber:  payment.ReferenceNumber,
		CustomerName:   payment.CustomerName,
		MeterNumber:    payment.MeterNumber,
		ProviderName:   payment.ProviderName,
		AmountPaid:     payment.Amount,
		ServiceFee:     payment.ServiceFee,
		TotalAmount:    payment.TotalAmount,
		Token:          payment.Token,
		Units:          payment.Units,
		PaymentDate:    payment.CreatedAt.Format("2006-01-02 15:04:05"),
		ReferenceNumber: payment.ReferenceNumber,
	}

	return &pb.GetPaymentReceiptResponse{
		Payment:     convertPaymentToProto(&payment),
		ReceiptData: receiptData,
	}, nil
}

// UpdateBeneficiary updates an existing beneficiary
func (s *ElectricityBillService) UpdateBeneficiary(ctx context.Context, req *pb.UpdateBeneficiaryRequest) (*pb.UpdateBeneficiaryResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	var beneficiary models.BillBeneficiary
	if err := s.db.Where("id = ? AND user_id = ?", req.BeneficiaryId, user.ID).First(&beneficiary).Error; err != nil {
		return nil, status.Error(codes.NotFound, "beneficiary not found")
	}

	// If setting as default, unset others
	if req.IsDefault {
		s.db.Model(&models.BillBeneficiary{}).Where("user_id = ?", user.ID).Update("is_default", false)
	}

	updates := map[string]interface{}{}
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
	}
	updates["is_default"] = req.IsDefault

	if err := s.db.Model(&beneficiary).Updates(updates).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to update beneficiary")
	}

	return &pb.UpdateBeneficiaryResponse{
		Beneficiary: convertBeneficiaryToProto(&beneficiary),
		Message:     "Beneficiary updated successfully",
	}, nil
}

// DeleteBeneficiary deletes a beneficiary
func (s *ElectricityBillService) DeleteBeneficiary(ctx context.Context, req *pb.DeleteBeneficiaryRequest) (*pb.DeleteBeneficiaryResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	result := s.db.Where("id = ? AND user_id = ?", req.BeneficiaryId, user.ID).Delete(&models.BillBeneficiary{})
	if result.Error != nil {
		return nil, status.Error(codes.Internal, "failed to delete beneficiary")
	}
	if result.RowsAffected == 0 {
		return nil, status.Error(codes.NotFound, "beneficiary not found")
	}

	return &pb.DeleteBeneficiaryResponse{
		Message: "Beneficiary deleted successfully",
	}, nil
}

// CreateAutoRecharge creates a new auto-recharge configuration
func (s *ElectricityBillService) CreateAutoRecharge(ctx context.Context, req *pb.CreateAutoRechargeRequest) (*pb.CreateAutoRechargeResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	var beneficiary models.BillBeneficiary
	if err := s.db.Where("id = ? AND user_id = ?", req.BeneficiaryId, user.ID).First(&beneficiary).Error; err != nil {
		return nil, status.Error(codes.NotFound, "beneficiary not found")
	}

	// Calculate next run date based on frequency
	nextRunDate := calculateNextRunDate(req.Frequency, req.DayOfWeek, req.DayOfMonth)

	maxRetries := req.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	autoRecharge := &models.AutoRecharge{
		UserID:        user.ID,
		BeneficiaryID: req.BeneficiaryId,
		ProviderID:    beneficiary.ProviderID,
		ProviderCode:  beneficiary.ProviderCode,
		ProviderName:  beneficiary.ProviderName,
		MeterNumber:   beneficiary.MeterNumber,
		CustomerName:  beneficiary.CustomerName,
		MeterType:     beneficiary.MeterType,
		Amount:        req.Amount,
		Currency:      req.Currency,
		Frequency:     models.RechargeFrequency(req.Frequency),
		NextRunDate:   nextRunDate,
		Status:        models.AutoRechargeStatusActive,
		MaxRetries:    int(maxRetries),
	}

	if req.DayOfWeek != 0 {
		day := int(req.DayOfWeek)
		autoRecharge.DayOfWeek = &day
	}
	if req.DayOfMonth != 0 {
		day := int(req.DayOfMonth)
		autoRecharge.DayOfMonth = &day
	}

	if err := s.db.Create(autoRecharge).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to create auto-recharge")
	}

	return &pb.CreateAutoRechargeResponse{
		AutoRecharge: convertAutoRechargeToProto(autoRecharge),
		Message:      "Auto-recharge created successfully",
	}, nil
}

// GetAutoRecharges retrieves user's auto-recharge configurations
func (s *ElectricityBillService) GetAutoRecharges(ctx context.Context, req *pb.GetAutoRechargesRequest) (*pb.GetAutoRechargesResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	query := s.db.Preload("Beneficiary").Where("user_id = ?", user.ID)

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	var autoRecharges []models.AutoRecharge
	if err := query.Order("created_at DESC").Find(&autoRecharges).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch auto-recharges")
	}

	pbAutoRecharges := make([]*pb.AutoRecharge, len(autoRecharges))
	for i, ar := range autoRecharges {
		pbAutoRecharges[i] = convertAutoRechargeToProto(&ar)
	}

	return &pb.GetAutoRechargesResponse{
		AutoRecharges: pbAutoRecharges,
	}, nil
}

// UpdateAutoRecharge updates an auto-recharge configuration
func (s *ElectricityBillService) UpdateAutoRecharge(ctx context.Context, req *pb.UpdateAutoRechargeRequest) (*pb.UpdateAutoRechargeResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	var autoRecharge models.AutoRecharge
	if err := s.db.Where("id = ? AND user_id = ?", req.AutoRechargeId, user.ID).First(&autoRecharge).Error; err != nil {
		return nil, status.Error(codes.NotFound, "auto-recharge not found")
	}

	updates := map[string]interface{}{}
	if req.Amount > 0 {
		updates["amount"] = req.Amount
	}
	if req.Frequency != "" {
		updates["frequency"] = req.Frequency
		nextRunDate := calculateNextRunDate(req.Frequency, req.DayOfWeek, req.DayOfMonth)
		updates["next_run_date"] = nextRunDate
	}
	if req.DayOfWeek != 0 {
		day := int(req.DayOfWeek)
		updates["day_of_week"] = &day
	}
	if req.DayOfMonth != 0 {
		day := int(req.DayOfMonth)
		updates["day_of_month"] = &day
	}
	if req.MaxRetries != 0 {
		updates["max_retries"] = req.MaxRetries
	}

	if err := s.db.Model(&autoRecharge).Updates(updates).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to update auto-recharge")
	}

	return &pb.UpdateAutoRechargeResponse{
		AutoRecharge: convertAutoRechargeToProto(&autoRecharge),
		Message:      "Auto-recharge updated successfully",
	}, nil
}

// DeleteAutoRecharge deletes an auto-recharge configuration
func (s *ElectricityBillService) DeleteAutoRecharge(ctx context.Context, req *pb.DeleteAutoRechargeRequest) (*pb.DeleteAutoRechargeResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	result := s.db.Where("id = ? AND user_id = ?", req.AutoRechargeId, user.ID).Delete(&models.AutoRecharge{})
	if result.Error != nil {
		return nil, status.Error(codes.Internal, "failed to delete auto-recharge")
	}
	if result.RowsAffected == 0 {
		return nil, status.Error(codes.NotFound, "auto-recharge not found")
	}

	return &pb.DeleteAutoRechargeResponse{
		Message: "Auto-recharge deleted successfully",
	}, nil
}

// PauseAutoRecharge pauses an auto-recharge configuration
func (s *ElectricityBillService) PauseAutoRecharge(ctx context.Context, req *pb.PauseAutoRechargeRequest) (*pb.PauseAutoRechargeResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	result := s.db.Model(&models.AutoRecharge{}).
		Where("id = ? AND user_id = ?", req.AutoRechargeId, user.ID).
		Update("status", models.AutoRechargeStatusPaused)

	if result.Error != nil {
		return nil, status.Error(codes.Internal, "failed to pause auto-recharge")
	}
	if result.RowsAffected == 0 {
		return nil, status.Error(codes.NotFound, "auto-recharge not found")
	}

	return &pb.PauseAutoRechargeResponse{
		Message: "Auto-recharge paused successfully",
	}, nil
}

// ResumeAutoRecharge resumes a paused auto-recharge configuration
func (s *ElectricityBillService) ResumeAutoRecharge(ctx context.Context, req *pb.ResumeAutoRechargeRequest) (*pb.ResumeAutoRechargeResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	result := s.db.Model(&models.AutoRecharge{}).
		Where("id = ? AND user_id = ?", req.AutoRechargeId, user.ID).
		Update("status", models.AutoRechargeStatusActive)

	if result.Error != nil {
		return nil, status.Error(codes.Internal, "failed to resume auto-recharge")
	}
	if result.RowsAffected == 0 {
		return nil, status.Error(codes.NotFound, "auto-recharge not found")
	}

	return &pb.ResumeAutoRechargeResponse{
		Message: "Auto-recharge resumed successfully",
	}, nil
}

// CreateReminder creates a new payment reminder
func (s *ElectricityBillService) CreateReminder(ctx context.Context, req *pb.CreateReminderRequest) (*pb.CreateReminderResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	reminder := &models.PaymentReminder{
		UserID:       user.ID,
		Title:        req.Title,
		Description:  req.Description,
		ReminderDate: req.ReminderDate.AsTime(),
		IsRecurring:  req.IsRecurring,
		Status:       models.ReminderStatusActive,
		Currency:     req.Currency,
	}

	if req.BeneficiaryId != "" {
		reminder.BeneficiaryID = &req.BeneficiaryId
	}
	if req.Amount > 0 {
		reminder.Amount = &req.Amount
	}
	if req.RecurrenceType != "" {
		reminder.RecurrenceType = &req.RecurrenceType
	}

	if err := s.db.Create(reminder).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to create reminder")
	}

	return &pb.CreateReminderResponse{
		Reminder: convertReminderToProto(reminder),
		Message:  "Reminder created successfully",
	}, nil
}

// GetReminders retrieves user's payment reminders
func (s *ElectricityBillService) GetReminders(ctx context.Context, req *pb.GetRemindersRequest) (*pb.GetRemindersResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	query := s.db.Where("user_id = ?", user.ID)

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	if !req.IncludePast {
		query = query.Where("reminder_date >= ?", time.Now())
	}

	var reminders []models.PaymentReminder
	if err := query.Order("reminder_date ASC").Find(&reminders).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to fetch reminders")
	}

	pbReminders := make([]*pb.BillPaymentReminder, len(reminders))
	for i, r := range reminders {
		pbReminders[i] = convertReminderToProto(&r)
	}

	return &pb.GetRemindersResponse{
		Reminders: pbReminders,
	}, nil
}

// UpdateReminder updates an existing reminder
func (s *ElectricityBillService) UpdateReminder(ctx context.Context, req *pb.UpdateReminderRequest) (*pb.UpdateReminderResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	var reminder models.PaymentReminder
	if err := s.db.Where("id = ? AND user_id = ?", req.ReminderId, user.ID).First(&reminder).Error; err != nil {
		return nil, status.Error(codes.NotFound, "reminder not found")
	}

	updates := map[string]interface{}{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.ReminderDate != nil {
		updates["reminder_date"] = req.ReminderDate.AsTime()
	}
	if req.Amount > 0 {
		updates["amount"] = req.Amount
	}
	if req.Currency != "" {
		updates["currency"] = req.Currency
	}
	updates["is_recurring"] = req.IsRecurring
	if req.RecurrenceType != "" {
		updates["recurrence_type"] = req.RecurrenceType
	}

	if err := s.db.Model(&reminder).Updates(updates).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to update reminder")
	}

	return &pb.UpdateReminderResponse{
		Reminder: convertReminderToProto(&reminder),
		Message:  "Reminder updated successfully",
	}, nil
}

// DeleteReminder deletes a reminder
func (s *ElectricityBillService) DeleteReminder(ctx context.Context, req *pb.DeleteReminderRequest) (*pb.DeleteReminderResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	result := s.db.Where("id = ? AND user_id = ?", req.ReminderId, user.ID).Delete(&models.PaymentReminder{})
	if result.Error != nil {
		return nil, status.Error(codes.Internal, "failed to delete reminder")
	}
	if result.RowsAffected == 0 {
		return nil, status.Error(codes.NotFound, "reminder not found")
	}

	return &pb.DeleteReminderResponse{
		Message: "Reminder deleted successfully",
	}, nil
}

// MarkReminderComplete marks a reminder as completed
func (s *ElectricityBillService) MarkReminderComplete(ctx context.Context, req *pb.MarkReminderCompleteRequest) (*pb.MarkReminderCompleteResponse, error) {
	payload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || payload == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	var user models.User
	if err := s.db.Where("email = ?", payload.Email).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	result := s.db.Model(&models.PaymentReminder{}).
		Where("id = ? AND user_id = ?", req.ReminderId, user.ID).
		Update("status", models.ReminderStatusCompleted)

	if result.Error != nil {
		return nil, status.Error(codes.Internal, "failed to mark reminder as complete")
	}
	if result.RowsAffected == 0 {
		return nil, status.Error(codes.NotFound, "reminder not found")
	}

	return &pb.MarkReminderCompleteResponse{
		Message: "Reminder marked as completed",
	}, nil
}

// SyncProviders syncs electricity providers from payment gateway
func (s *ElectricityBillService) SyncProviders(ctx context.Context, req *pb.SyncProvidersRequest) (*pb.SyncProvidersResponse, error) {
	gateway := req.PaymentGateway
	if gateway == "" {
		gateway = "flutterwave" // Default
	}

	client := s.providerFactory.GetProvider(gateway)
	providers, err := client.GetProviders(ctx, "NG") // Default to Nigeria

	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to sync providers: %v", err))
	}

	syncedCount := 0
	for _, p := range providers {
		provider := models.ElectricityProvider{
			ProviderCode:   p.Code,
			ProviderName:   p.Name,
			Country:        "NG",
			LogoURL:        p.LogoURL,
			IsActive:       true,
			PaymentGateway: gateway,
			MinAmount:      p.MinAmount,
			MaxAmount:      p.MaxAmount,
			ServiceFee:     p.ServiceFee,
			FeeType:        p.FeeType,
		}

		// Upsert: Update if exists, insert if new
		result := s.db.Where("provider_code = ? AND payment_gateway = ?", p.Code, gateway).
			Assign(provider).
			FirstOrCreate(&provider)

		if result.Error == nil {
			syncedCount++
		}
	}

	return &pb.SyncProvidersResponse{
		SyncedCount: int32(syncedCount),
		Message:     fmt.Sprintf("Successfully synced %d providers", syncedCount),
	}, nil
}

// Helper function to calculate next run date based on frequency
func calculateNextRunDate(frequency string, dayOfWeek, dayOfMonth int32) time.Time {
	now := time.Now()

	switch frequency {
	case "daily":
		return now.AddDate(0, 0, 1)
	case "weekly":
		daysUntil := (int(dayOfWeek) - int(now.Weekday()) + 7) % 7
		if daysUntil == 0 {
			daysUntil = 7
		}
		return now.AddDate(0, 0, daysUntil)
	case "monthly":
		nextMonth := now.AddDate(0, 1, 0)
		return time.Date(nextMonth.Year(), nextMonth.Month(), int(dayOfMonth), 0, 0, 0, 0, time.UTC)
	default:
		return now.AddDate(0, 0, 1)
	}
}

// Helper functions to convert models to proto
func convertAutoRechargeToProto(ar *models.AutoRecharge) *pb.AutoRecharge {
	result := &pb.AutoRecharge{
		Id:            ar.ID,
		UserId:        fmt.Sprintf("%d", ar.UserID),
		BeneficiaryId: ar.BeneficiaryID,
		MeterNumber:   ar.MeterNumber,
		Amount:        ar.Amount,
		Currency:      ar.Currency,
		Frequency:     string(ar.Frequency),
		NextRunDate:   timestamppb.New(ar.NextRunDate),
		Status:        string(ar.Status),
		FailureCount:  int32(ar.FailureCount),
		CreatedAt:     timestamppb.New(ar.CreatedAt),
		ProviderId:    ar.ProviderID,
		ProviderCode:  ar.ProviderCode,
		ProviderName:  ar.ProviderName,
		CustomerName:  ar.CustomerName,
		MeterType:     ar.MeterType,
		MaxRetries:    int32(ar.MaxRetries),
		UpdatedAt:     timestamppb.New(ar.UpdatedAt),
	}

	if ar.DayOfWeek != nil {
		result.DayOfWeek = int32(*ar.DayOfWeek)
	}
	if ar.DayOfMonth != nil {
		result.DayOfMonth = int32(*ar.DayOfMonth)
	}
	if ar.LastRunDate != nil {
		result.LastRunDate = timestamppb.New(*ar.LastRunDate)
	}
	if ar.Beneficiary != nil {
		result.Beneficiary = convertBeneficiaryToProto(ar.Beneficiary)
	}

	return result
}

func convertReminderToProto(r *models.PaymentReminder) *pb.BillPaymentReminder {
	result := &pb.BillPaymentReminder{
		Id:           r.ID,
		UserId:       fmt.Sprintf("%d", r.UserID),
		Title:        r.Title,
		Description:  r.Description,
		ReminderDate: timestamppb.New(r.ReminderDate),
		IsRecurring:  r.IsRecurring,
		Status:       string(r.Status),
		CreatedAt:    timestamppb.New(r.CreatedAt),
		Currency:     r.Currency,
		UpdatedAt:    timestamppb.New(r.UpdatedAt),
	}

	if r.BeneficiaryID != nil {
		result.BeneficiaryId = *r.BeneficiaryID
	}
	if r.Amount != nil {
		result.Amount = *r.Amount
	}
	if r.RecurrenceType != nil {
		result.RecurrenceType = *r.RecurrenceType
	}
	if r.NotifiedAt != nil {
		result.NotifiedAt = timestamppb.New(*r.NotifiedAt)
	}

	return result
}

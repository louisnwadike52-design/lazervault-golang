package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type InsuranceController struct {
	pb.UnimplementedInsuranceServiceServer
	insuranceService services.IInsuranceService
}

func NewInsuranceController(insuranceService services.IInsuranceService) *InsuranceController {
	return &InsuranceController{
		insuranceService: insuranceService,
	}
}

// GetUserInsurances retrieves all insurances for the authenticated user
func (c *InsuranceController) GetUserInsurances(ctx context.Context, req *pb.GetUserInsurancesRequest) (*pb.GetUserInsurancesResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Set default values
	page := int(req.Page)
	limit := int(req.Limit)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	// Get insurances from service
	insurances, pagination, err := c.insuranceService.GetUserInsurances(ctx, userID, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user insurances: %v", err)
	}

	// Convert to protobuf format
	pbInsurances := make([]*pb.Insurance, len(insurances))
	for i, insurance := range insurances {
		pbInsurances[i] = c.convertInsuranceToProto(&insurance)
	}

	return &pb.GetUserInsurancesResponse{
		Insurances: pbInsurances,
		Pagination: c.convertPaginationToProto(pagination),
		Success:    true,
		Msg:        "Insurances retrieved successfully",
	}, nil
}

// GetInsuranceById retrieves a specific insurance by ID
func (c *InsuranceController) GetInsuranceById(ctx context.Context, req *pb.GetInsuranceByIdRequest) (*pb.GetInsuranceByIdResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get insurance from service
	insurance, err := c.insuranceService.GetInsuranceById(ctx, req.Id, userID)
	if err != nil {
		if errors.Is(err, services.ErrInsuranceNotFound) {
			return nil, status.Errorf(codes.NotFound, "insurance not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get insurance: %v", err)
	}

	return &pb.GetInsuranceByIdResponse{
		Insurance: c.convertInsuranceToProto(insurance),
		Success:   true,
		Msg:       "Insurance retrieved successfully",
	}, nil
}

// CreateInsurance creates a new insurance policy
func (c *InsuranceController) CreateInsurance(ctx context.Context, req *pb.CreateInsuranceRequest) (*pb.CreateInsuranceResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Convert protobuf to model
	insurance := c.convertProtoToInsurance(req.Insurance)
	insurance.UserID = userID

	// Create insurance
	err = c.insuranceService.CreateInsurance(ctx, insurance)
	if err != nil {
		if errors.Is(err, services.ErrInvalidInsuranceType) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid insurance type")
		}
		if errors.Is(err, services.ErrInvalidAmount) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid amount")
		}
		if errors.Is(err, services.ErrInvalidDateRange) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid date range")
		}
		return nil, status.Errorf(codes.Internal, "failed to create insurance: %v", err)
	}

	return &pb.CreateInsuranceResponse{
		Insurance: c.convertInsuranceToProto(insurance),
		Success:   true,
		Msg:       "Insurance created successfully",
	}, nil
}

// UpdateInsurance updates an existing insurance policy
func (c *InsuranceController) UpdateInsurance(ctx context.Context, req *pb.UpdateInsuranceRequest) (*pb.UpdateInsuranceResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Convert protobuf to model
	insurance := c.convertProtoToInsurance(req.Insurance)

	// Update insurance
	err = c.insuranceService.UpdateInsurance(ctx, insurance, userID)
	if err != nil {
		if errors.Is(err, services.ErrInsuranceNotFound) {
			return nil, status.Errorf(codes.NotFound, "insurance not found")
		}
		if errors.Is(err, services.ErrInvalidInsuranceType) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid insurance type")
		}
		if errors.Is(err, services.ErrInvalidAmount) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid amount")
		}
		if errors.Is(err, services.ErrInvalidDateRange) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid date range")
		}
		return nil, status.Errorf(codes.Internal, "failed to update insurance: %v", err)
	}

	return &pb.UpdateInsuranceResponse{
		Insurance: c.convertInsuranceToProto(insurance),
		Success:   true,
		Msg:       "Insurance updated successfully",
	}, nil
}

// DeleteInsurance deletes an insurance policy
func (c *InsuranceController) DeleteInsurance(ctx context.Context, req *pb.DeleteInsuranceRequest) (*pb.DeleteInsuranceResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Delete insurance
	err = c.insuranceService.DeleteInsurance(ctx, req.Id, userID)
	if err != nil {
		if errors.Is(err, services.ErrInsuranceNotFound) {
			return nil, status.Errorf(codes.NotFound, "insurance not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete insurance: %v", err)
	}

	return &pb.DeleteInsuranceResponse{
		Success: true,
		Msg:     "Insurance deleted successfully",
	}, nil
}

// SearchInsurances searches for insurances
func (c *InsuranceController) SearchInsurances(ctx context.Context, req *pb.SearchInsurancesRequest) (*pb.SearchInsurancesResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Set default values
	page := int(req.Page)
	limit := int(req.Limit)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	// Search insurances
	insurances, pagination, err := c.insuranceService.SearchInsurances(ctx, userID, req.Query, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search insurances: %v", err)
	}

	// Convert to protobuf format
	pbInsurances := make([]*pb.Insurance, len(insurances))
	for i, insurance := range insurances {
		pbInsurances[i] = c.convertInsuranceToProto(&insurance)
	}

	return &pb.SearchInsurancesResponse{
		Insurances: pbInsurances,
		Pagination: c.convertPaginationToProto(pagination),
		Success:    true,
		Msg:        "Search completed successfully",
	}, nil
}

// GetInsurancePayments retrieves payments for a specific insurance
func (c *InsuranceController) GetInsurancePayments(ctx context.Context, req *pb.GetInsurancePaymentsRequest) (*pb.GetInsurancePaymentsResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Set default values
	page := int(req.Page)
	limit := int(req.Limit)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	// Get payments
	payments, pagination, err := c.insuranceService.GetInsurancePayments(ctx, req.InsuranceId, userID, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get insurance payments: %v", err)
	}

	// Convert to protobuf format
	pbPayments := make([]*pb.InsurancePayment, len(payments))
	for i, payment := range payments {
		pbPayments[i] = c.convertPaymentToProto(&payment)
	}

	return &pb.GetInsurancePaymentsResponse{
		Payments:   pbPayments,
		Pagination: c.convertPaginationToProto(pagination),
		Success:    true,
		Msg:        "Payments retrieved successfully",
	}, nil
}

// GetUserPayments retrieves all payments for the authenticated user
func (c *InsuranceController) GetUserPayments(ctx context.Context, req *pb.GetUserPaymentsRequest) (*pb.GetUserPaymentsResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Set default values
	page := int(req.Page)
	limit := int(req.Limit)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	// Get payments
	payments, pagination, err := c.insuranceService.GetUserPayments(ctx, userID, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user payments: %v", err)
	}

	// Convert to protobuf format
	pbPayments := make([]*pb.InsurancePayment, len(payments))
	for i, payment := range payments {
		pbPayments[i] = c.convertPaymentToProto(&payment)
	}

	return &pb.GetUserPaymentsResponse{
		Payments:   pbPayments,
		Pagination: c.convertPaginationToProto(pagination),
		Success:    true,
		Msg:        "Payments retrieved successfully",
	}, nil
}

// CreatePayment creates a new payment
func (c *InsuranceController) CreatePayment(ctx context.Context, req *pb.CreatePaymentRequest) (*pb.CreatePaymentResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Convert protobuf to model
	payment := c.convertProtoToPayment(req.Payment)
	payment.UserID = userID

	// Create payment
	err = c.insuranceService.CreatePayment(ctx, payment)
	if err != nil {
		if errors.Is(err, services.ErrInvalidPaymentMethod) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid payment method")
		}
		if errors.Is(err, services.ErrInvalidAmount) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid amount")
		}
		return nil, status.Errorf(codes.Internal, "failed to create payment: %v", err)
	}

	return &pb.CreatePaymentResponse{
		Payment: c.convertPaymentToProto(payment),
		Success: true,
		Msg:     "Payment created successfully",
	}, nil
}

// ProcessPayment processes a payment
func (c *InsuranceController) ProcessPayment(ctx context.Context, req *pb.ProcessPaymentRequest) (*pb.ProcessPaymentResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Convert payment details map
	paymentDetails := make(map[string]string)
	for k, v := range req.PaymentDetails {
		paymentDetails[k] = v
	}

	// Process payment
	payment, err := c.insuranceService.ProcessPayment(ctx, req.PaymentId, req.PaymentMethod, paymentDetails, userID)
	if err != nil {
		if errors.Is(err, services.ErrPaymentNotFound) {
			return nil, status.Errorf(codes.NotFound, "payment not found")
		}
		if errors.Is(err, services.ErrInvalidPaymentMethod) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid payment method")
		}
		if errors.Is(err, services.ErrPaymentAlreadyProcessed) {
			return nil, status.Errorf(codes.FailedPrecondition, "payment already processed")
		}
		return nil, status.Errorf(codes.Internal, "failed to process payment: %v", err)
	}

	// Get transaction details
	transactionID := ""
	referenceNumber := ""
	receiptURL := ""
	if payment.TransactionID != nil {
		transactionID = *payment.TransactionID
	}
	if payment.ReferenceNumber != nil {
		referenceNumber = *payment.ReferenceNumber
	}
	if payment.ReceiptURL != nil {
		receiptURL = *payment.ReceiptURL
	}

	return &pb.ProcessPaymentResponse{
		Payment:         c.convertPaymentToProto(payment),
		TransactionId:   transactionID,
		ReferenceNumber: referenceNumber,
		ReceiptUrl:      receiptURL,
		Success:         true,
		Msg:             "Payment processed successfully",
	}, nil
}

// GetPaymentById retrieves a payment by ID
func (c *InsuranceController) GetPaymentById(ctx context.Context, req *pb.GetPaymentByIdRequest) (*pb.GetPaymentByIdResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get payment
	payment, err := c.insuranceService.GetPaymentById(ctx, req.Id, userID)
	if err != nil {
		if errors.Is(err, services.ErrPaymentNotFound) {
			return nil, status.Errorf(codes.NotFound, "payment not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get payment: %v", err)
	}

	return &pb.GetPaymentByIdResponse{
		Payment: c.convertPaymentToProto(payment),
		Success: true,
		Msg:     "Payment retrieved successfully",
	}, nil
}

// GetOverduePayments retrieves overdue payments for the authenticated user
func (c *InsuranceController) GetOverduePayments(ctx context.Context, req *pb.GetOverduePaymentsRequest) (*pb.GetOverduePaymentsResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get overdue payments
	payments, err := c.insuranceService.GetOverduePayments(ctx, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get overdue payments: %v", err)
	}

	// Convert to protobuf format
	pbPayments := make([]*pb.InsurancePayment, len(payments))
	for i, payment := range payments {
		pbPayments[i] = c.convertPaymentToProto(&payment)
	}

	return &pb.GetOverduePaymentsResponse{
		Payments: pbPayments,
		Success:  true,
		Msg:      "Overdue payments retrieved successfully",
	}, nil
}

// GetInsuranceClaims retrieves claims for a specific insurance
func (c *InsuranceController) GetInsuranceClaims(ctx context.Context, req *pb.GetInsuranceClaimsRequest) (*pb.GetInsuranceClaimsResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Set default values
	page := int(req.Page)
	limit := int(req.Limit)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	// Get claims
	claims, pagination, err := c.insuranceService.GetInsuranceClaims(ctx, req.InsuranceId, userID, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get insurance claims: %v", err)
	}

	// Convert to protobuf format
	pbClaims := make([]*pb.InsuranceClaim, len(claims))
	for i, claim := range claims {
		pbClaims[i] = c.convertClaimToProto(&claim)
	}

	return &pb.GetInsuranceClaimsResponse{
		Claims:     pbClaims,
		Pagination: c.convertPaginationToProto(pagination),
		Success:    true,
		Msg:        "Claims retrieved successfully",
	}, nil
}

// GetUserClaims retrieves all claims for the authenticated user
func (c *InsuranceController) GetUserClaims(ctx context.Context, req *pb.GetUserClaimsRequest) (*pb.GetUserClaimsResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Set default values
	page := int(req.Page)
	limit := int(req.Limit)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	// Get claims
	claims, pagination, err := c.insuranceService.GetUserClaims(ctx, userID, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user claims: %v", err)
	}

	// Convert to protobuf format
	pbClaims := make([]*pb.InsuranceClaim, len(claims))
	for i, claim := range claims {
		pbClaims[i] = c.convertClaimToProto(&claim)
	}

	return &pb.GetUserClaimsResponse{
		Claims:     pbClaims,
		Pagination: c.convertPaginationToProto(pagination),
		Success:    true,
		Msg:        "Claims retrieved successfully",
	}, nil
}

// CreateClaim creates a new claim
func (c *InsuranceController) CreateClaim(ctx context.Context, req *pb.CreateClaimRequest) (*pb.CreateClaimResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Convert protobuf to model
	claim := c.convertProtoToClaim(req.Claim)
	claim.UserID = userID

	// Create claim
	err = c.insuranceService.CreateClaim(ctx, claim)
	if err != nil {
		if errors.Is(err, services.ErrInvalidAmount) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid amount")
		}
		return nil, status.Errorf(codes.Internal, "failed to create claim: %v", err)
	}

	return &pb.CreateClaimResponse{
		Claim:   c.convertClaimToProto(claim),
		Success: true,
		Msg:     "Claim created successfully",
	}, nil
}

// UpdateClaim updates an existing claim
func (c *InsuranceController) UpdateClaim(ctx context.Context, req *pb.UpdateClaimRequest) (*pb.UpdateClaimResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Convert protobuf to model
	claim := c.convertProtoToClaim(req.Claim)

	// Update claim
	err = c.insuranceService.UpdateClaim(ctx, claim, userID)
	if err != nil {
		if errors.Is(err, services.ErrClaimNotFound) {
			return nil, status.Errorf(codes.NotFound, "claim not found")
		}
		if errors.Is(err, services.ErrClaimAlreadyProcessed) {
			return nil, status.Errorf(codes.FailedPrecondition, "claim already processed")
		}
		if errors.Is(err, services.ErrInvalidAmount) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid amount")
		}
		return nil, status.Errorf(codes.Internal, "failed to update claim: %v", err)
	}

	return &pb.UpdateClaimResponse{
		Claim:   c.convertClaimToProto(claim),
		Success: true,
		Msg:     "Claim updated successfully",
	}, nil
}

// GetClaimById retrieves a claim by ID
func (c *InsuranceController) GetClaimById(ctx context.Context, req *pb.GetClaimByIdRequest) (*pb.GetClaimByIdResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get claim
	claim, err := c.insuranceService.GetClaimById(ctx, req.Id, userID)
	if err != nil {
		if errors.Is(err, services.ErrClaimNotFound) {
			return nil, status.Errorf(codes.NotFound, "claim not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get claim: %v", err)
	}

	return &pb.GetClaimByIdResponse{
		Claim:   c.convertClaimToProto(claim),
		Success: true,
		Msg:     "Claim retrieved successfully",
	}, nil
}

// GeneratePaymentReceipt generates a receipt for a payment
func (c *InsuranceController) GeneratePaymentReceipt(ctx context.Context, req *pb.GeneratePaymentReceiptRequest) (*pb.GeneratePaymentReceiptResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get payment to verify ownership
	payment, err := c.insuranceService.GetPaymentById(ctx, req.PaymentId, userID)
	if err != nil {
		if errors.Is(err, services.ErrPaymentNotFound) {
			return nil, status.Errorf(codes.NotFound, "payment not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get payment: %v", err)
	}

	// Generate receipt URL (in real implementation, generate actual receipt)
	receiptURL := "https://api.lazervault.com/receipts/" + payment.ID + ".pdf"
	receiptID := "REC-" + time.Now().Format("20060102150405") + "-" + payment.ID[:8]

	return &pb.GeneratePaymentReceiptResponse{
		ReceiptUrl: receiptURL,
		ReceiptId:  receiptID,
		Success:    true,
		Msg:        "Receipt generated successfully",
	}, nil
}

// GetUserReceipts retrieves all receipts for the authenticated user
func (c *InsuranceController) GetUserReceipts(ctx context.Context, req *pb.GetUserReceiptsRequest) (*pb.GetUserReceiptsResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Set default values
	page := int(req.Page)
	limit := int(req.Limit)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	// Get payments with receipts
	payments, pagination, err := c.insuranceService.GetUserPayments(ctx, userID, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user receipts: %v", err)
	}

	// Extract receipt URLs
	var receiptURLs []string
	for _, payment := range payments {
		if payment.ReceiptURL != nil && *payment.ReceiptURL != "" {
			receiptURLs = append(receiptURLs, *payment.ReceiptURL)
		}
	}

	return &pb.GetUserReceiptsResponse{
		ReceiptUrls: receiptURLs,
		Pagination:  c.convertPaginationToProto(pagination),
		Success:     true,
		Msg:         "Receipts retrieved successfully",
	}, nil
}

// GetInsuranceStatistics retrieves insurance statistics for the authenticated user
func (c *InsuranceController) GetInsuranceStatistics(ctx context.Context, req *pb.GetInsuranceStatisticsRequest) (*pb.GetInsuranceStatisticsResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get statistics
	stats, err := c.insuranceService.GetInsuranceStatistics(ctx, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get insurance statistics: %v", err)
	}

	// Convert policies by type map
	policiesByType := make(map[string]int32)
	for k, v := range stats.PoliciesByType {
		policiesByType[k] = int32(v)
	}

	return &pb.GetInsuranceStatisticsResponse{
		TotalPolicies:       int32(stats.TotalPolicies),
		ActivePolicies:      int32(stats.ActivePolicies),
		ExpiredPolicies:     int32(stats.ExpiredPolicies),
		TotalCoverageAmount: stats.TotalCoverageAmount,
		TotalPremiumAmount:  stats.TotalPremiumAmount,
		PoliciesByType:      policiesByType,
		Success:             true,
		Msg:                 "Statistics retrieved successfully",
	}, nil
}

// GetPaymentStatistics retrieves payment statistics for the authenticated user
func (c *InsuranceController) GetPaymentStatistics(ctx context.Context, req *pb.GetPaymentStatisticsRequest) (*pb.GetPaymentStatisticsResponse, error) {
	// Get user ID from context
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Parse date range
	var startDate, endDate *time.Time
	if req.StartDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.StartDate); err == nil {
			startDate = &parsed
		}
	}
	if req.EndDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.EndDate); err == nil {
			endDate = &parsed
		}
	}

	// Get statistics
	stats, err := c.insuranceService.GetPaymentStatistics(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get payment statistics: %v", err)
	}

	// Convert payments by method map
	paymentsByMethod := make(map[string]int32)
	for k, v := range stats.PaymentsByMethod {
		paymentsByMethod[k] = int32(v)
	}

	return &pb.GetPaymentStatisticsResponse{
		TotalPayments:     int32(stats.TotalPayments),
		CompletedPayments: int32(stats.CompletedPayments),
		PendingPayments:   int32(stats.PendingPayments),
		FailedPayments:    int32(stats.FailedPayments),
		TotalAmount:       stats.TotalAmount,
		CompletedAmount:   stats.CompletedAmount,
		PaymentsByMethod:  paymentsByMethod,
		Success:           true,
		Msg:               "Statistics retrieved successfully",
	}, nil
}

// Helper methods

// getUserIDFromContext extracts user ID from the context
func (c *InsuranceController) getUserIDFromContext(ctx context.Context) (uint, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return 0, status.Errorf(codes.Unauthenticated, "missing or invalid authorization payload")
	}

	// Extract user ID from the payload (assuming it's stored as a string)
	userID, err := strconv.ParseUint(authPayload.Email, 10, 32)
	if err != nil {
		// If email is not a user ID, we need to get the user ID from the database
		// For now, we'll use a placeholder approach
		// In a real implementation, you'd query the database to get the user ID from the email
		return 0, status.Errorf(codes.Internal, "failed to get user ID from context")
	}

	return uint(userID), nil
}

// convertInsuranceToProto converts Insurance model to protobuf
func (c *InsuranceController) convertInsuranceToProto(insurance *models.Insurance) *pb.Insurance {
	if insurance == nil {
		return nil
	}

	// Convert beneficiaries JSON to string slice
	var beneficiaries []string
	if !insurance.Beneficiaries.IsNull() {
		// Parse JSON array to string slice
		// This is a simplified implementation
		beneficiaries = []string{} // Parse from JSON
	}

	// Convert coverage details JSON to map
	coverageDetails := make(map[string]string)
	if !insurance.CoverageDetails.IsNull() {
		// Parse JSON object to map
		// This is a simplified implementation
		coverageDetails = make(map[string]string) // Parse from JSON
	}

	// Handle optional description field
	description := ""
	if insurance.Description != nil {
		description = *insurance.Description
	}

	return &pb.Insurance{
		Id:                insurance.ID,
		PolicyNumber:      insurance.PolicyNumber,
		PolicyHolderName:  insurance.PolicyHolderName,
		PolicyHolderEmail: insurance.PolicyHolderEmail,
		PolicyHolderPhone: insurance.PolicyHolderPhone,
		Type:              insurance.Type,
		Provider:          insurance.Provider,
		ProviderLogo:      insurance.ProviderLogo,
		PremiumAmount:     insurance.PremiumAmount,
		CoverageAmount:    insurance.CoverageAmount,
		Currency:          insurance.Currency,
		StartDate:         insurance.StartDate.Format("2006-01-02"),
		EndDate:           insurance.EndDate.Format("2006-01-02"),
		NextPaymentDate:   insurance.NextPaymentDate.Format("2006-01-02"),
		Status:            insurance.Status,
		Beneficiaries:     beneficiaries,
		CoverageDetails:   coverageDetails,
		Description:       description,
		UserId:            strconv.FormatUint(uint64(insurance.UserID), 10),
		CreatedAt:         insurance.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:         insurance.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// convertProtoToInsurance converts protobuf to Insurance model
func (c *InsuranceController) convertProtoToInsurance(pbInsurance *pb.Insurance) *models.Insurance {
	if pbInsurance == nil {
		return nil
	}

	// Parse dates
	startDate, _ := time.Parse("2006-01-02", pbInsurance.StartDate)
	endDate, _ := time.Parse("2006-01-02", pbInsurance.EndDate)
	nextPaymentDate, _ := time.Parse("2006-01-02", pbInsurance.NextPaymentDate)

	// Convert beneficiaries to JSON
	beneficiariesJSON := models.JSON("[]") // Convert string slice to JSON

	// Convert coverage details to JSON
	coverageDetailsJSON := models.JSON("{}") // Convert map to JSON

	return &models.Insurance{
		ID:                pbInsurance.Id,
		PolicyNumber:      pbInsurance.PolicyNumber,
		PolicyHolderName:  pbInsurance.PolicyHolderName,
		PolicyHolderEmail: pbInsurance.PolicyHolderEmail,
		PolicyHolderPhone: pbInsurance.PolicyHolderPhone,
		Type:              pbInsurance.Type,
		Provider:          pbInsurance.Provider,
		ProviderLogo:      pbInsurance.ProviderLogo,
		PremiumAmount:     pbInsurance.PremiumAmount,
		CoverageAmount:    pbInsurance.CoverageAmount,
		Currency:          pbInsurance.Currency,
		StartDate:         startDate,
		EndDate:           endDate,
		NextPaymentDate:   nextPaymentDate,
		Status:            pbInsurance.Status,
		Beneficiaries:     beneficiariesJSON,
		CoverageDetails:   coverageDetailsJSON,
		Description:       &pbInsurance.Description,
	}
}

// convertPaymentToProto converts InsurancePayment model to protobuf
func (c *InsuranceController) convertPaymentToProto(payment *models.InsurancePayment) *pb.InsurancePayment {
	if payment == nil {
		return nil
	}

	// Convert payment details JSON to map
	paymentDetails := make(map[string]string)
	if !payment.PaymentDetails.IsNull() {
		// Parse JSON object to map
		// This is a simplified implementation
		paymentDetails = make(map[string]string) // Parse from JSON
	}

	// Handle optional fields
	transactionID := ""
	if payment.TransactionID != nil {
		transactionID = *payment.TransactionID
	}

	referenceNumber := ""
	if payment.ReferenceNumber != nil {
		referenceNumber = *payment.ReferenceNumber
	}

	failureReason := ""
	if payment.FailureReason != nil {
		failureReason = *payment.FailureReason
	}

	receiptURL := ""
	if payment.ReceiptURL != nil {
		receiptURL = *payment.ReceiptURL
	}

	processedAt := ""
	if payment.ProcessedAt != nil {
		processedAt = payment.ProcessedAt.Format("2006-01-02T15:04:05Z")
	}

	return &pb.InsurancePayment{
		Id:              payment.ID,
		InsuranceId:     payment.InsuranceID,
		PolicyNumber:    payment.PolicyNumber,
		Amount:          payment.Amount,
		Currency:        payment.Currency,
		PaymentMethod:   payment.PaymentMethod,
		Status:          payment.Status,
		TransactionId:   transactionID,
		ReferenceNumber: referenceNumber,
		PaymentDate:     payment.PaymentDate.Format("2006-01-02T15:04:05Z"),
		DueDate:         payment.DueDate.Format("2006-01-02T15:04:05Z"),
		ProcessedAt:     processedAt,
		PaymentDetails:  paymentDetails,
		FailureReason:   failureReason,
		ReceiptUrl:      receiptURL,
		UserId:          strconv.FormatUint(uint64(payment.UserID), 10),
		CreatedAt:       payment.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       payment.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// convertProtoToPayment converts protobuf to InsurancePayment model
func (c *InsuranceController) convertProtoToPayment(pbPayment *pb.InsurancePayment) *models.InsurancePayment {
	if pbPayment == nil {
		return nil
	}

	// Parse dates
	paymentDate, _ := time.Parse("2006-01-02T15:04:05Z", pbPayment.PaymentDate)
	dueDate, _ := time.Parse("2006-01-02T15:04:05Z", pbPayment.DueDate)

	// Convert payment details to JSON
	paymentDetailsJSON := models.JSON("{}") // Convert map to JSON

	return &models.InsurancePayment{
		ID:             pbPayment.Id,
		InsuranceID:    pbPayment.InsuranceId,
		PolicyNumber:   pbPayment.PolicyNumber,
		Amount:         pbPayment.Amount,
		Currency:       pbPayment.Currency,
		PaymentMethod:  pbPayment.PaymentMethod,
		Status:         pbPayment.Status,
		PaymentDate:    paymentDate,
		DueDate:        dueDate,
		PaymentDetails: paymentDetailsJSON,
	}
}

// convertClaimToProto converts InsuranceClaim model to protobuf
func (c *InsuranceController) convertClaimToProto(claim *models.InsuranceClaim) *pb.InsuranceClaim {
	if claim == nil {
		return nil
	}

	// Convert attachments JSON to string slice
	var attachments []string
	if !claim.Attachments.IsNull() {
		// Parse JSON array to string slice
		// This is a simplified implementation
		attachments = []string{} // Parse from JSON
	}

	// Convert documents JSON to string slice
	var documents []string
	if !claim.Documents.IsNull() {
		// Parse JSON array to string slice
		// This is a simplified implementation
		documents = []string{} // Parse from JSON
	}

	// Convert additional info JSON to map
	additionalInfo := make(map[string]string)
	if !claim.AdditionalInfo.IsNull() {
		// Parse JSON object to map
		// This is a simplified implementation
		additionalInfo = make(map[string]string) // Parse from JSON
	}

	// Handle optional fields
	approvedAmount := 0.0
	if claim.ApprovedAmount != nil {
		approvedAmount = *claim.ApprovedAmount
	}

	rejectionReason := ""
	if claim.RejectionReason != nil {
		rejectionReason = *claim.RejectionReason
	}

	settlementDetails := ""
	if claim.SettlementDetails != nil {
		settlementDetails = *claim.SettlementDetails
	}

	settlementDate := ""
	if claim.SettlementDate != nil {
		settlementDate = claim.SettlementDate.Format("2006-01-02")
	}

	return &pb.InsuranceClaim{
		Id:                claim.ID,
		ClaimNumber:       claim.ClaimNumber,
		InsuranceId:       claim.InsuranceID,
		PolicyNumber:      claim.PolicyNumber,
		Type:              claim.Type,
		Status:            claim.Status,
		Title:             claim.Title,
		Description:       claim.Description,
		ClaimAmount:       claim.ClaimAmount,
		ApprovedAmount:    approvedAmount,
		Currency:          claim.Currency,
		IncidentDate:      claim.IncidentDate.Format("2006-01-02"),
		IncidentLocation:  claim.IncidentLocation,
		Attachments:       attachments,
		Documents:         documents,
		AdditionalInfo:    additionalInfo,
		RejectionReason:   rejectionReason,
		SettlementDate:    settlementDate,
		SettlementDetails: settlementDetails,
		UserId:            strconv.FormatUint(uint64(claim.UserID), 10),
		CreatedAt:         claim.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:         claim.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// convertProtoToClaim converts protobuf to InsuranceClaim model
func (c *InsuranceController) convertProtoToClaim(pbClaim *pb.InsuranceClaim) *models.InsuranceClaim {
	if pbClaim == nil {
		return nil
	}

	// Parse dates
	incidentDate, _ := time.Parse("2006-01-02", pbClaim.IncidentDate)

	// Convert attachments to JSON
	attachmentsJSON := models.JSON("[]") // Convert string slice to JSON

	// Convert documents to JSON
	documentsJSON := models.JSON("[]") // Convert string slice to JSON

	// Convert additional info to JSON
	additionalInfoJSON := models.JSON("{}") // Convert map to JSON

	return &models.InsuranceClaim{
		ID:                pbClaim.Id,
		ClaimNumber:       pbClaim.ClaimNumber,
		InsuranceID:       pbClaim.InsuranceId,
		PolicyNumber:      pbClaim.PolicyNumber,
		Type:              pbClaim.Type,
		Status:            pbClaim.Status,
		Title:             pbClaim.Title,
		Description:       pbClaim.Description,
		ClaimAmount:       pbClaim.ClaimAmount,
		ApprovedAmount:    &pbClaim.ApprovedAmount,
		Currency:          pbClaim.Currency,
		IncidentDate:      incidentDate,
		IncidentLocation:  pbClaim.IncidentLocation,
		Attachments:       attachmentsJSON,
		Documents:         documentsJSON,
		AdditionalInfo:    additionalInfoJSON,
		RejectionReason:   &pbClaim.RejectionReason,
		SettlementDetails: &pbClaim.SettlementDetails,
	}
}

// convertPaginationToProto converts PaginationInfo model to protobuf
func (c *InsuranceController) convertPaginationToProto(pagination *models.PaginationInfo) *pb.PaginationInfo {
	if pagination == nil {
		return nil
	}

	return &pb.PaginationInfo{
		CurrentPage:  pagination.CurrentPage,
		TotalPages:   pagination.TotalPages,
		TotalItems:   pagination.TotalItems,
		ItemsPerPage: pagination.ItemsPerPage,
		HasNext:      pagination.HasNext,
		HasPrev:      pagination.HasPrev,
	}
}

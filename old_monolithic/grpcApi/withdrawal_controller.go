package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	// Removed timestamppb
)

// WithdrawalController handles gRPC requests for the WithdrawService.
type WithdrawalController struct {
	pb.UnimplementedWithdrawServiceServer // Embed for forward compatibility
	withdrawalService                     services.IWithdrawalService
	userService                           services.IUserService // Needed to get UserID
}

// NewWithdrawalController creates a new WithdrawalController.
func NewWithdrawalController(withdrawService services.IWithdrawalService, userService services.IUserService) *WithdrawalController {
	return &WithdrawalController{
		withdrawalService: withdrawService,
		userService:       userService,
	}
}

// Removed convertWithdrawalStatus
// Removed convertWithdrawalTransaction

// Helper to convert Withdrawal model to GetWithdrawalDetailsResponse proto
func convertWithdrawalModelToProtoDetails(w *models.Withdrawal) *pb.GetWithdrawalDetailsResponse {
	if w == nil {
		return nil
	}
	resp := &pb.GetWithdrawalDetailsResponse{
		WithdrawalId:        w.ID,
		SourceAccountId:     uint64(w.SourceAccountID),
		Amount:              uint64(w.Amount), // Convert int64 minor units to uint64
		Currency:            w.Currency,
		TargetBankName:      w.TargetBankName,
		TargetAccountNumber: w.TargetAccountNumber, // Consider masking
		TargetSortCode:      w.TargetSortCode,
		Status:              convertWithdrawalStatusModelToProto(w.Status), // Use new helper name
		CreatedAt:           timestamppb.New(w.CreatedAt),
	}
	if w.ProcessingAt != nil {
		resp.ProcessingAt = timestamppb.New(*w.ProcessingAt)
	}
	if w.CompletedAt != nil {
		resp.CompletedAt = timestamppb.New(*w.CompletedAt)
	}
	if w.FailedAt != nil {
		resp.FailedAt = timestamppb.New(*w.FailedAt)
	}
	if w.FailureReason != "" {
		resp.FailureReason = w.FailureReason
	}
	if w.ExternalTransactionID != nil {
		resp.ExternalTransactionId = *w.ExternalTransactionID
	}
	return resp
}

// Helper to convert WithdrawalStatus model to proto enum
func convertWithdrawalStatusModelToProto(status models.WithdrawalStatus) pb.WithdrawalStatus {
	switch status {
	case models.WithdrawalStatusPending:
		return pb.WithdrawalStatus_WITHDRAWAL_STATUS_PENDING
	case models.WithdrawalStatusProcessing:
		return pb.WithdrawalStatus_WITHDRAWAL_STATUS_PROCESSING
	case models.WithdrawalStatusCompleted:
		return pb.WithdrawalStatus_WITHDRAWAL_STATUS_COMPLETED
	case models.WithdrawalStatusFailed:
		return pb.WithdrawalStatus_WITHDRAWAL_STATUS_FAILED
	default:
		return pb.WithdrawalStatus_WITHDRAWAL_STATUS_UNSPECIFIED
	}
}

// InitiateWithdrawal handles the gRPC request to start an asynchronous withdrawal.
func (c *WithdrawalController) InitiateWithdrawal(ctx context.Context, req *pb.InitiateWithdrawalRequest) (*pb.InitiateWithdrawalResponse, error) {
	// 1. Get User ID from context
	user, err := getUserFromContext(ctx, c.userService) // Use shared helper
	if err != nil {
		return nil, err
	}
	userID := user.ID

	// 2. Basic Input Validation
	if req.GetSourceAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "source_account_id is required")
	}
	if req.GetAmount() == 0 { // Amount is uint64
		return nil, status.Errorf(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetCurrency() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "currency is required")
	}
	if req.GetTargetBankName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "target_bank_name is required")
	}
	if req.GetTargetAccountNumber() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "target_account_number is required")
	}

	// 3. Prepare Service Request (Convert amount to int64)
	serviceReq := services.InitiateWithdrawalRequest{
		UserID:              userID,
		SourceAccountID:     uint(req.GetSourceAccountId()),
		Amount:              int64(req.GetAmount()), // Convert uint64 to int64
		Currency:            req.GetCurrency(),
		TargetBankName:      req.GetTargetBankName(),
		TargetAccountNumber: req.GetTargetAccountNumber(),
		TargetSortCode:      req.GetTargetSortCode(),
	}

	// 4. Call Service
	initiationResponse, err := c.withdrawalService.InitiateWithdrawal(ctx, userID, serviceReq)
	if err != nil {
		// Map service errors to gRPC status codes
		// Note: InsufficientFunds/AccountNotFound/CurrencyMismatch are checked in the worker now.
		if errors.Is(err, services.ErrWithdrawalInvalidAmount) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		} else if errors.Is(err, services.ErrWithdrawalInitiationFailed) {
			return nil, status.Errorf(codes.Internal, "failed to save withdrawal request")
		} else if errors.Is(err, services.ErrWithdrawalEnqueueTaskFailed) {
			return nil, status.Errorf(codes.Internal, "failed to schedule withdrawal processing")
		}
		// Handle other potential errors (e.g., DB connection issues)
		return nil, status.Errorf(codes.Internal, "failed to initiate withdrawal: %v", err)
	}

	// 5. Return successful acknowledgement response from service
	return initiationResponse, nil
}

// GetWithdrawalDetails handles the gRPC request to retrieve withdrawal details.
func (c *WithdrawalController) GetWithdrawalDetails(ctx context.Context, req *pb.GetWithdrawalDetailsRequest) (*pb.GetWithdrawalDetailsResponse, error) {
	// 1. Get User ID from context
	user, err := getUserFromContext(ctx, c.userService) // Use shared helper
	if err != nil {
		return nil, err
	}

	// 2. Validate Request
	withdrawalID := req.GetWithdrawalId()
	if withdrawalID == "" {
		return nil, status.Error(codes.InvalidArgument, "withdrawal_id is required")
	}

	// 3. Call Service
	withdrawalModel, err := c.withdrawalService.GetWithdrawalDetails(ctx, withdrawalID, user.ID)
	if err != nil {
		// Map service errors
		if errors.Is(err, services.ErrWithdrawalNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		} else if errors.Is(err, services.ErrWithdrawalAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to get withdrawal details: %v", err)
	}

	// 4. Convert model to proto response
	resp := convertWithdrawalModelToProtoDetails(withdrawalModel)

	return resp, nil
}

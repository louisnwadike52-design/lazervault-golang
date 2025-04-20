package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
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

// convertWithdrawalStatus converts models.WithdrawalStatus to pb.WithdrawalStatus
func convertWithdrawalStatus(modelStatus models.WithdrawalStatus) pb.WithdrawalStatus {
	switch modelStatus {
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

// convertWithdrawalTransaction converts models.Withdrawal to pb.WithdrawalTransaction
func convertWithdrawalTransaction(withdrawal *models.Withdrawal) *pb.WithdrawalTransaction {
	if withdrawal == nil {
		return nil
	}
	pbWithdrawal := &pb.WithdrawalTransaction{
		TransactionId:       withdrawal.ID,
		SourceAccountId:     uint64(withdrawal.SourceAccountID),
		Amount:              withdrawal.Amount,
		Currency:            withdrawal.Currency,
		TargetBankName:      withdrawal.TargetBankName,
		TargetAccountNumber: withdrawal.TargetAccountNumber, // Mask this in production?
		TargetSortCode:      withdrawal.TargetSortCode,
		Status:              convertWithdrawalStatus(withdrawal.Status),
		CreatedAt:           timestamppb.New(withdrawal.CreatedAt),
		FailureReason:       withdrawal.FailureReason,
	}
	if withdrawal.CompletedAt != nil {
		pbWithdrawal.CompletedAt = timestamppb.New(*withdrawal.CompletedAt)
	}
	if withdrawal.FailedAt != nil {
		pbWithdrawal.FailedAt = timestamppb.New(*withdrawal.FailedAt)
	}
	return pbWithdrawal
}

// InitiateWithdrawal handles the gRPC request to start a withdrawal.
func (c *WithdrawalController) InitiateWithdrawal(ctx context.Context, req *pb.InitiateWithdrawalRequest) (*pb.InitiateWithdrawalResponse, error) {
	// 1. Get User ID from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	userID := user.ID // uint user ID

	// 2. Basic Input Validation
	if req.GetSourceAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "source_account_id is required")
	}
	if req.GetAmount() <= 0 {
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

	// 3. Prepare Service Request
	serviceReq := services.InitiateWithdrawalRequest{
		UserID:              userID,
		SourceAccountID:     uint(req.GetSourceAccountId()), // Convert uint64 to uint
		Amount:              req.GetAmount(),
		Currency:            req.GetCurrency(),
		TargetBankName:      req.GetTargetBankName(),
		TargetAccountNumber: req.GetTargetAccountNumber(),
		TargetSortCode:      req.GetTargetSortCode(),
	}

	// 4. Call Service
	createdWithdrawal, err := c.withdrawalService.InitiateWithdrawal(ctx, userID, serviceReq)
	if err != nil {
		// Map service errors to gRPC status codes
		if errors.Is(err, services.ErrWithdrawalSourceAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		if errors.Is(err, services.ErrWithdrawalInvalidAmount) || errors.Is(err, services.ErrWithdrawalCurrencyMismatch) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}
		if errors.Is(err, services.ErrWithdrawalInsufficientFunds) {
			return nil, status.Errorf(codes.FailedPrecondition, err.Error()) // Use FailedPrecondition for insufficient funds
		}
		if errors.Is(err, services.ErrWithdrawalProcessingFailed) {
			// Log underlying error if possible
			return nil, status.Errorf(codes.Internal, "failed to process withdrawal")
		}
		// Generic internal error
		return nil, status.Errorf(codes.Internal, "failed to initiate withdrawal: %v", err)
	}

	// 5. Convert and Return Response
	pbResponse := &pb.InitiateWithdrawalResponse{
		Success:     true,
		Message:     "Withdrawal initiated and processed successfully.", // Adjust if using async processing
		Transaction: convertWithdrawalTransaction(createdWithdrawal),
	}

	return pbResponse, nil
}

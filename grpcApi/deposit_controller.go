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

// DepositController handles gRPC requests for the DepositService.
type DepositController struct {
	pb.UnimplementedDepositServiceServer // Embed for forward compatibility
	depositService                       services.IDepositService
	userService                          services.IUserService // Needed to get UserID
}

// NewDepositController creates a new DepositController.
func NewDepositController(depositService services.IDepositService, userService services.IUserService) *DepositController {
	return &DepositController{
		depositService: depositService,
		userService:    userService,
	}
}

// convertDepositStatus converts models.DepositStatus to pb.DepositStatus
func convertDepositStatus(modelStatus models.DepositStatus) pb.DepositStatus {
	switch modelStatus {
	case models.DepositStatusPending:
		return pb.DepositStatus_DEPOSIT_STATUS_PENDING
	case models.DepositStatusCompleted:
		return pb.DepositStatus_DEPOSIT_STATUS_COMPLETED
	case models.DepositStatusFailed:
		return pb.DepositStatus_DEPOSIT_STATUS_FAILED
	default:
		return pb.DepositStatus_DEPOSIT_STATUS_UNSPECIFIED
	}
}

// convertDepositTransaction converts models.Deposit to pb.DepositTransaction
func convertDepositTransaction(deposit *models.Deposit) *pb.DepositTransaction {
	if deposit == nil {
		return nil
	}
	pbDeposit := &pb.DepositTransaction{
		TransactionId:   deposit.ID,
		TargetAccountId: uint64(deposit.TargetAccountID),
		Amount:          deposit.Amount,
		Currency:        deposit.Currency,
		SourceBankName:  deposit.SourceBankName,
		Status:          convertDepositStatus(deposit.Status),
		CreatedAt:       timestamppb.New(deposit.CreatedAt),
		FailureReason:   deposit.FailureReason,
	}
	if deposit.CompletedAt != nil {
		pbDeposit.CompletedAt = timestamppb.New(*deposit.CompletedAt)
	}
	if deposit.FailedAt != nil {
		pbDeposit.FailedAt = timestamppb.New(*deposit.FailedAt)
	}
	return pbDeposit
}

// InitiateDeposit handles the gRPC request to start a deposit.
func (c *DepositController) InitiateDeposit(ctx context.Context, req *pb.InitiateDepositRequest) (*pb.InitiateDepositResponse, error) {
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

	// 2. Basic Input Validation (Service layer does more)
	if req.GetTargetAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "target_account_id is required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetCurrency() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "currency is required")
	}
	if req.GetSourceBankName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "source_bank_name is required")
	}

	// 3. Prepare Service Request
	serviceReq := services.InitiateDepositRequest{
		UserID:          userID,
		TargetAccountID: uint(req.GetTargetAccountId()), // Convert uint64 to uint
		Amount:          req.GetAmount(),
		Currency:        req.GetCurrency(),
		SourceBankName:  req.GetSourceBankName(),
	}

	// 4. Call Service
	createdDeposit, err := c.depositService.InitiateDeposit(ctx, userID, serviceReq)
	if err != nil {
		// Map service errors to gRPC status codes
		if errors.Is(err, services.ErrDepositTargetAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		if errors.Is(err, services.ErrDepositInvalidAmount) || errors.Is(err, services.ErrDepositCurrencyMismatch) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}
		if errors.Is(err, services.ErrDepositProcessingFailed) {
			// Log the underlying error from the service if possible (txErr)
			// log.Errorf("Deposit processing failed: %v", err)
			return nil, status.Errorf(codes.Internal, "failed to process deposit") // Don't expose internal details
		}
		// Generic internal error
		return nil, status.Errorf(codes.Internal, "failed to initiate deposit: %v", err)
	}

	// 5. Convert and Return Response
	pbResponse := &pb.InitiateDepositResponse{
		Success:     true,
		Message:     "Deposit initiated and processed successfully.", // Adjust message if using background tasks
		Transaction: convertDepositTransaction(createdDeposit),
	}

	return pbResponse, nil
}

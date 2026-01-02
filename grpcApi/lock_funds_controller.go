package grpcApi

import (
	"context"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LockFundsController handles gRPC requests for lock funds
type LockFundsController struct {
	pb.UnimplementedLockFundsServiceServer
	service     *services.LockFundsService
	userService services.IUserService
}

// NewLockFundsController creates a new lock funds controller
func NewLockFundsController(service *services.LockFundsService, userService services.IUserService) *LockFundsController {
	return &LockFundsController{
		service:     service,
		userService: userService,
	}
}

// CreateLockFund creates a new lock fund
func (c *LockFundsController) CreateLockFund(ctx context.Context, req *pb.CreateLockFundRequest) (*pb.CreateLockFundResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	lockFund, err := c.service.CreateLockFund(ctx, user.ID, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.CreateLockFundResponse{
		Success:  true,
		Message:  "Lock fund created successfully",
		LockFund: c.service.ConvertToProto(lockFund),
	}, nil
}

// GetLockFunds retrieves all lock funds for the user
func (c *LockFundsController) GetLockFunds(ctx context.Context, req *pb.GetLockFundsRequest) (*pb.GetLockFundsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	page := req.Page
	if page < 1 {
		page = 1
	}

	perPage := req.PerPage
	if perPage < 1 {
		perPage = 20
	}

	var statusFilter *pb.LockStatus
	if req.Status != pb.LockStatus_LOCK_STATUS_UNSPECIFIED {
		statusFilter = &req.Status
	}

	lockFunds, total, err := c.service.GetLockFunds(ctx, user.ID, statusFilter, page, perPage)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to proto
	protoLockFunds := make([]*pb.LockFund, len(lockFunds))
	for i, lf := range lockFunds {
		protoLockFunds[i] = c.service.ConvertToProto(lf)
	}

	// Calculate statistics
	var totalLockedAmount float64
	var totalAccruedInterest float64
	var activeLocksCount int32

	for _, lf := range lockFunds {
		if lf.Status == "ACTIVE" {
			activeLocksCount++
			totalLockedAmount += lf.Amount
			totalAccruedInterest += lf.AccruedInterest
		}
	}

	totalPages := int32((total + int64(perPage) - 1) / int64(perPage))

	return &pb.GetLockFundsResponse{
		LockFunds:            protoLockFunds,
		TotalCount:           int32(total),
		Page:                 page,
		TotalPages:           totalPages,
		TotalLockedAmount:    totalLockedAmount,
		TotalAccruedInterest: totalAccruedInterest,
		ActiveLocksCount:     activeLocksCount,
	}, nil
}

// GetLockFund retrieves a single lock fund
func (c *LockFundsController) GetLockFund(ctx context.Context, req *pb.GetLockFundRequest) (*pb.GetLockFundResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	lockFund, err := c.service.GetLockFund(ctx, user.ID, req.LockFundId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &pb.GetLockFundResponse{
		Success:  true,
		LockFund: c.service.ConvertToProto(lockFund),
	}, nil
}

// UnlockFund unlocks a fund
func (c *LockFundsController) UnlockFund(ctx context.Context, req *pb.UnlockFundRequest) (*pb.UnlockFundResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	amountReturned, penaltyAmount, interestEarned, lockFund, err := c.service.UnlockFund(
		ctx,
		user.ID,
		req.LockFundId,
		req.ForceEarlyUnlock,
	)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	message := "Fund unlocked successfully"
	if penaltyAmount > 0 {
		message = "Fund unlocked early with penalty applied"
	}

	return &pb.UnlockFundResponse{
		Success:          true,
		Message:          message,
		AmountReturned:   amountReturned,
		PenaltyAmount:    penaltyAmount,
		InterestEarned:   interestEarned,
		UpdatedLockFund:  c.service.ConvertToProto(lockFund),
	}, nil
}

// GetLockTransactions retrieves lock fund transactions
func (c *LockFundsController) GetLockTransactions(ctx context.Context, req *pb.GetLockTransactionsRequest) (*pb.GetLockTransactionsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	page := req.Page
	if page < 1 {
		page = 1
	}

	perPage := req.PerPage
	if perPage < 1 {
		perPage = 20
	}

	var lockFundIDFilter *string
	if req.LockFundId != "" {
		lockFundIDFilter = &req.LockFundId
	}

	transactions, total, err := c.service.GetLockTransactions(ctx, user.ID, lockFundIDFilter, page, perPage)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert to proto
	protoTransactions := make([]*pb.LockTransaction, len(transactions))
	for i, tx := range transactions {
		protoTransactions[i] = c.service.ConvertTransactionToProto(tx)
	}

	totalPages := int32((total + int64(perPage) - 1) / int64(perPage))

	return &pb.GetLockTransactionsResponse{
		Transactions: protoTransactions,
		TotalCount:   int32(total),
		Page:         page,
		TotalPages:   totalPages,
	}, nil
}

// CalculateInterest calculates estimated interest
func (c *LockFundsController) CalculateInterest(ctx context.Context, req *pb.CalculateInterestRequest) (*pb.CalculateInterestResponse, error) {
	interestRate, estimatedInterest, totalReturn, apy, err := c.service.CalculateInterest(
		req.LockType,
		req.Amount,
		req.LockDurationDays,
	)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &pb.CalculateInterestResponse{
		InterestRate:      interestRate,
		EstimatedInterest: estimatedInterest,
		TotalReturn:       totalReturn,
		Apy:               apy,
	}, nil
}

// RenewLockFund renews a matured lock fund
func (c *LockFundsController) RenewLockFund(ctx context.Context, req *pb.RenewLockFundRequest) (*pb.RenewLockFundResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	lockFund, err := c.service.RenewLockFund(ctx, user.ID, req.LockFundId, req.NewDurationDays)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &pb.RenewLockFundResponse{
		Success:          true,
		Message:          "Lock fund renewed successfully",
		RenewedLockFund:  c.service.ConvertToProto(lockFund),
	}, nil
}

// CancelLockFund cancels a lock fund
func (c *LockFundsController) CancelLockFund(ctx context.Context, req *pb.CancelLockFundRequest) (*pb.CancelLockFundResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	refundAmount, _, err := c.service.CancelLockFund(ctx, user.ID, req.LockFundId, req.Reason)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &pb.CancelLockFundResponse{
		Success:      true,
		Message:      "Lock fund cancelled successfully",
		RefundAmount: refundAmount,
	}, nil
}

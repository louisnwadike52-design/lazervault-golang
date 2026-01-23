package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CrowdfundController struct {
	pb.UnimplementedCrowdfundServiceServer
	crowdfundService services.ICrowdfundService
	userService      services.IUserService
}

func NewCrowdfundController(crowdfundService services.ICrowdfundService, userService services.IUserService) *CrowdfundController {
	return &CrowdfundController{
		crowdfundService: crowdfundService,
		userService:      userService,
	}
}

// ============================================================================
// CROWDFUND MANAGEMENT
// ============================================================================

func (c *CrowdfundController) CreateCrowdfund(ctx context.Context, req *pb.CreateCrowdfundRequest) (*pb.CreateCrowdfundResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	// Validate request
	if req.Title == "" || req.TargetAmount == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "title and target amount are required")
	}

	crowdfund, err := c.crowdfundService.CreateCrowdfund(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrUserNotVerified) {
			return nil, status.Errorf(codes.PermissionDenied, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to create crowdfund: %v", err)
	}

	return &pb.CreateCrowdfundResponse{Crowdfund: crowdfund}, nil
}

func (c *CrowdfundController) GetCrowdfund(ctx context.Context, req *pb.GetCrowdfundRequest) (*pb.GetCrowdfundResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	crowdfund, err := c.crowdfundService.GetCrowdfund(ctx, user.ID, req.CrowdfundId)
	if err != nil {
		if errors.Is(err, services.ErrCrowdfundNotFound) {
			return nil, status.Errorf(codes.NotFound, "crowdfund not found")
		}
		if errors.Is(err, services.ErrCrowdfundAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to get crowdfund: %v", err)
	}

	return &pb.GetCrowdfundResponse{Crowdfund: crowdfund}, nil
}

func (c *CrowdfundController) ListCrowdfunds(ctx context.Context, req *pb.ListCrowdfundsRequest) (*pb.ListCrowdfundsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	crowdfunds, total, err := c.crowdfundService.ListCrowdfunds(ctx, user.ID, page, pageSize, req.Status, req.Category, req.MyCrowdfundsOnly)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list crowdfunds: %v", err)
	}

	pagination := &pb.CrowdfundPaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   int32((total + int64(pageSize) - 1) / int64(pageSize)),
		TotalItems:   int32(total),
		ItemsPerPage: int32(pageSize),
		HasNext:      int64(page*pageSize) < total,
		HasPrev:      page > 1,
	}

	return &pb.ListCrowdfundsResponse{
		Crowdfunds: crowdfunds,
		Pagination: pagination,
	}, nil
}

func (c *CrowdfundController) SearchCrowdfunds(ctx context.Context, req *pb.SearchCrowdfundsRequest) (*pb.SearchCrowdfundsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.Query == "" {
		return nil, status.Errorf(codes.InvalidArgument, "search query is required")
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 10
	}

	crowdfunds, err := c.crowdfundService.SearchCrowdfunds(ctx, user.ID, req.Query, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search failed: %v", err)
	}

	return &pb.SearchCrowdfundsResponse{Crowdfunds: crowdfunds}, nil
}

func (c *CrowdfundController) UpdateCrowdfund(ctx context.Context, req *pb.UpdateCrowdfundRequest) (*pb.UpdateCrowdfundResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	crowdfund, err := c.crowdfundService.UpdateCrowdfund(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrCrowdfundNotFound) {
			return nil, status.Errorf(codes.NotFound, "crowdfund not found")
		}
		if errors.Is(err, services.ErrCrowdfundAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "only crowdfund creator can update")
		}
		return nil, status.Errorf(codes.Internal, "failed to update crowdfund: %v", err)
	}

	return &pb.UpdateCrowdfundResponse{Crowdfund: crowdfund}, nil
}

func (c *CrowdfundController) DeleteCrowdfund(ctx context.Context, req *pb.DeleteCrowdfundRequest) (*pb.DeleteCrowdfundResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	err = c.crowdfundService.DeleteCrowdfund(ctx, user.ID, req.CrowdfundId)
	if err != nil {
		if errors.Is(err, services.ErrCrowdfundNotFound) {
			return nil, status.Errorf(codes.NotFound, "crowdfund not found")
		}
		if errors.Is(err, services.ErrCrowdfundAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "only crowdfund creator can delete")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete crowdfund: %v", err)
	}

	return &pb.DeleteCrowdfundResponse{
		Success: true,
		Message: "Crowdfund deleted successfully",
	}, nil
}

// ============================================================================
// DONATION OPERATIONS
// ============================================================================

func (c *CrowdfundController) MakeDonation(ctx context.Context, req *pb.MakeDonationRequest) (*pb.MakeDonationResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.Amount == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "donation amount must be greater than 0")
	}

	donation, err := c.crowdfundService.MakeDonation(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidAmount) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}
		if errors.Is(err, services.ErrCrowdfundNotActive) {
			return nil, status.Errorf(codes.FailedPrecondition, "crowdfund is not active")
		}
		if errors.Is(err, services.ErrCrowdfundDeadlinePassed) {
			return nil, status.Errorf(codes.FailedPrecondition, "crowdfund deadline has passed")
		}
		return nil, status.Errorf(codes.Internal, "failed to make donation: %v", err)
	}

	return &pb.MakeDonationResponse{Donation: donation}, nil
}

func (c *CrowdfundController) GetCrowdfundDonations(ctx context.Context, req *pb.GetCrowdfundDonationsRequest) (*pb.GetCrowdfundDonationsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	donations, total, err := c.crowdfundService.GetCrowdfundDonations(ctx, user.ID, req.CrowdfundId, page, pageSize)
	if err != nil {
		if errors.Is(err, services.ErrCrowdfundNotFound) {
			return nil, status.Errorf(codes.NotFound, "crowdfund not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get donations: %v", err)
	}

	pagination := &pb.DonationPaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   int32((total + int64(pageSize) - 1) / int64(pageSize)),
		TotalItems:   int32(total),
		ItemsPerPage: int32(pageSize),
		HasNext:      int64(page*pageSize) < total,
		HasPrev:      page > 1,
	}

	return &pb.GetCrowdfundDonationsResponse{
		Donations:  donations,
		Pagination: pagination,
	}, nil
}

func (c *CrowdfundController) GetUserDonations(ctx context.Context, req *pb.GetUserDonationsRequest) (*pb.GetUserDonationsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	donations, total, err := c.crowdfundService.GetUserDonations(ctx, user.ID, page, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user donations: %v", err)
	}

	pagination := &pb.DonationPaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   int32((total + int64(pageSize) - 1) / int64(pageSize)),
		TotalItems:   int32(total),
		ItemsPerPage: int32(pageSize),
		HasNext:      int64(page*pageSize) < total,
		HasPrev:      page > 1,
	}

	return &pb.GetUserDonationsResponse{
		Donations:  donations,
		Pagination: pagination,
	}, nil
}

// ============================================================================
// RECEIPT OPERATIONS
// ============================================================================

func (c *CrowdfundController) GenerateDonationReceipt(ctx context.Context, req *pb.GenerateDonationReceiptRequest) (*pb.GenerateDonationReceiptResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	receipt, err := c.crowdfundService.GenerateDonationReceipt(ctx, user.ID, req.DonationId)
	if err != nil {
		if errors.Is(err, services.ErrDonationNotFound) {
			return nil, status.Errorf(codes.NotFound, "donation not found")
		}
		if errors.Is(err, services.ErrUnauthorized) {
			return nil, status.Errorf(codes.PermissionDenied, "unauthorized to generate receipt")
		}
		return nil, status.Errorf(codes.Internal, "failed to generate receipt: %v", err)
	}

	return &pb.GenerateDonationReceiptResponse{Receipt: receipt}, nil
}

func (c *CrowdfundController) GetUserCrowdfundReceipts(ctx context.Context, req *pb.GetUserCrowdfundReceiptsRequest) (*pb.GetUserCrowdfundReceiptsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	receipts, total, err := c.crowdfundService.GetUserReceipts(ctx, user.ID, page, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get receipts: %v", err)
	}

	pagination := &pb.CrowdfundReceiptPaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   int32((total + int64(pageSize) - 1) / int64(pageSize)),
		TotalItems:   int32(total),
		ItemsPerPage: int32(pageSize),
		HasNext:      int64(page*pageSize) < total,
		HasPrev:      page > 1,
	}

	return &pb.GetUserCrowdfundReceiptsResponse{
		Receipts:   receipts,
		Pagination: pagination,
	}, nil
}

// ============================================================================
// STATISTICS
// ============================================================================

func (c *CrowdfundController) GetCrowdfundStatistics(ctx context.Context, req *pb.GetCrowdfundStatisticsRequest) (*pb.GetCrowdfundStatisticsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	stats, err := c.crowdfundService.GetCrowdfundStatistics(ctx, user.ID, req.CrowdfundId)
	if err != nil {
		if errors.Is(err, services.ErrCrowdfundNotFound) {
			return nil, status.Errorf(codes.NotFound, "crowdfund not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get statistics: %v", err)
	}

	return stats, nil
}

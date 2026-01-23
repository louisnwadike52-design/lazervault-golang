package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GroupAccountController struct {
	pb.UnimplementedGroupAccountServiceServer
	groupAccountService services.IGroupAccountService
	userService         services.IUserService
}

func NewGroupAccountController(groupAccountService services.IGroupAccountService, userService services.IUserService) *GroupAccountController {
	return &GroupAccountController{
		groupAccountService: groupAccountService,
		userService:         userService,
	}
}

// ============================================================================
// GROUP MANAGEMENT
// ============================================================================

func (c *GroupAccountController) CreateGroup(ctx context.Context, req *pb.CreateGroupRequest) (*pb.CreateGroupResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	group, err := c.groupAccountService.CreateGroup(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create group: %v", err)
	}

	return &pb.CreateGroupResponse{Group: group}, nil
}

func (c *GroupAccountController) GetGroup(ctx context.Context, req *pb.GetGroupRequest) (*pb.GetGroupResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	group, err := c.groupAccountService.GetGroup(ctx, user.ID, req.GroupId)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrGroupAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to get group: %v", err)
	}

	return &pb.GetGroupResponse{Group: group}, nil
}

func (c *GroupAccountController) ListUserGroups(ctx context.Context, req *pb.ListUserGroupsRequest) (*pb.ListUserGroupsResponse, error) {
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

	groups, total, err := c.groupAccountService.ListUserGroups(ctx, user.ID, page, pageSize, req.Status)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list groups: %v", err)
	}

	pagination := &pb.GroupPaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   int32((total + int64(pageSize) - 1) / int64(pageSize)),
		TotalItems:   int32(total),
		ItemsPerPage: int32(pageSize),
		HasNext:      int64(page*pageSize) < total,
		HasPrev:      page > 1,
	}

	return &pb.ListUserGroupsResponse{
		Groups:     groups,
		Pagination: pagination,
	}, nil
}

func (c *GroupAccountController) UpdateGroup(ctx context.Context, req *pb.UpdateGroupRequest) (*pb.UpdateGroupResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	group, err := c.groupAccountService.UpdateGroup(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrNotGroupAdmin) {
			return nil, status.Errorf(codes.PermissionDenied, "only group admin can update")
		}
		return nil, status.Errorf(codes.Internal, "failed to update group: %v", err)
	}

	return &pb.UpdateGroupResponse{Group: group}, nil
}

func (c *GroupAccountController) DeleteGroup(ctx context.Context, req *pb.DeleteGroupRequest) (*pb.DeleteGroupResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	err = c.groupAccountService.DeleteGroup(ctx, user.ID, req.GroupId)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrNotGroupAdmin) {
			return nil, status.Errorf(codes.PermissionDenied, "only group admin can delete")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete group: %v", err)
	}

	return &pb.DeleteGroupResponse{Success: true}, nil
}

// ============================================================================
// MEMBER MANAGEMENT
// ============================================================================

func (c *GroupAccountController) GetGroupMembers(ctx context.Context, req *pb.GetGroupMembersRequest) (*pb.GetGroupMembersResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	members, err := c.groupAccountService.GetGroupMembers(ctx, user.ID, req.GroupId)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrGroupAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to get members: %v", err)
	}

	return &pb.GetGroupMembersResponse{Members: members}, nil
}

func (c *GroupAccountController) AddMember(ctx context.Context, req *pb.AddMemberRequest) (*pb.AddMemberResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	member, err := c.groupAccountService.AddMember(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrNotGroupAdmin) {
			return nil, status.Errorf(codes.PermissionDenied, "only group admin can add members")
		}
		if errors.Is(err, services.ErrGroupMemberAlreadyExists) {
			return nil, status.Errorf(codes.AlreadyExists, "user is already a member")
		}
		return nil, status.Errorf(codes.Internal, "failed to add member: %v", err)
	}

	return &pb.AddMemberResponse{Member: member}, nil
}

func (c *GroupAccountController) UpdateMemberRole(ctx context.Context, req *pb.UpdateMemberRoleRequest) (*pb.UpdateMemberRoleResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	member, err := c.groupAccountService.UpdateMemberRole(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrNotGroupAdmin) {
			return nil, status.Errorf(codes.PermissionDenied, "only group admin can update roles")
		}
		if errors.Is(err, services.ErrGroupMemberNotFound) {
			return nil, status.Errorf(codes.NotFound, "member not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to update member role: %v", err)
	}

	return &pb.UpdateMemberRoleResponse{Member: member}, nil
}

func (c *GroupAccountController) RemoveMember(ctx context.Context, req *pb.RemoveMemberRequest) (*pb.RemoveMemberResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	err = c.groupAccountService.RemoveMember(ctx, user.ID, req.GroupId, req.MemberId)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrNotGroupAdmin) {
			return nil, status.Errorf(codes.PermissionDenied, "only group admin can remove members")
		}
		return nil, status.Errorf(codes.Internal, "failed to remove member: %v", err)
	}

	return &pb.RemoveMemberResponse{Success: true}, nil
}

func (c *GroupAccountController) SearchUsers(ctx context.Context, req *pb.SearchUsersRequest) (*pb.SearchUsersResponse, error) {
	limit := int(req.Limit)
	if limit == 0 {
		limit = 10
	}

	users, err := c.groupAccountService.SearchUsers(ctx, req.Query, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search users: %v", err)
	}

	return &pb.SearchUsersResponse{Users: users}, nil
}

// ============================================================================
// CONTRIBUTION MANAGEMENT
// ============================================================================

func (c *GroupAccountController) CreateContribution(ctx context.Context, req *pb.CreateContributionRequest) (*pb.CreateContributionResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	contribution, err := c.groupAccountService.CreateContribution(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrGroupAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to create contribution: %v", err)
	}

	return &pb.CreateContributionResponse{Contribution: contribution}, nil
}

func (c *GroupAccountController) GetContribution(ctx context.Context, req *pb.GetContributionRequest) (*pb.GetContributionResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	contribution, err := c.groupAccountService.GetContribution(ctx, user.ID, req.ContributionId)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrContributionAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to get contribution: %v", err)
	}

	return &pb.GetContributionResponse{Contribution: contribution}, nil
}

func (c *GroupAccountController) ListGroupContributions(ctx context.Context, req *pb.ListGroupContributionsRequest) (*pb.ListGroupContributionsResponse, error) {
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

	contributions, total, err := c.groupAccountService.ListGroupContributions(ctx, user.ID, req.GroupId, page, pageSize, req.Status)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrGroupAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to list contributions: %v", err)
	}

	pagination := &pb.ContributionPaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   int32((total + int64(pageSize) - 1) / int64(pageSize)),
		TotalItems:   int32(total),
		ItemsPerPage: int32(pageSize),
		HasNext:      int64(page*pageSize) < total,
		HasPrev:      page > 1,
	}

	return &pb.ListGroupContributionsResponse{
		Contributions: contributions,
		Pagination:    pagination,
	}, nil
}

func (c *GroupAccountController) UpdateContribution(ctx context.Context, req *pb.UpdateContributionRequest) (*pb.UpdateContributionResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	contribution, err := c.groupAccountService.UpdateContribution(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrContributionAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to update contribution: %v", err)
	}

	return &pb.UpdateContributionResponse{Contribution: contribution}, nil
}

func (c *GroupAccountController) DeleteContribution(ctx context.Context, req *pb.DeleteContributionRequest) (*pb.DeleteContributionResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	err = c.groupAccountService.DeleteContribution(ctx, user.ID, req.ContributionId)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrContributionAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete contribution: %v", err)
	}

	return &pb.DeleteContributionResponse{Success: true}, nil
}

// ============================================================================
// PAYMENT OPERATIONS
// ============================================================================

func (c *GroupAccountController) MakePayment(ctx context.Context, req *pb.MakePaymentRequest) (*pb.MakePaymentResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	payment, err := c.groupAccountService.MakePayment(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrGroupAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to make payment: %v", err)
	}

	return &pb.MakePaymentResponse{Payment: payment}, nil
}

func (c *GroupAccountController) GetContributionPayments(ctx context.Context, req *pb.GetContributionPaymentsRequest) (*pb.GetContributionPaymentsResponse, error) {
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

	payments, total, err := c.groupAccountService.GetContributionPayments(ctx, user.ID, req.ContributionId, page, pageSize)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrContributionAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to get payments: %v", err)
	}

	pagination := &pb.PaymentPaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   int32((total + int64(pageSize) - 1) / int64(pageSize)),
		TotalItems:   int32(total),
		ItemsPerPage: int32(pageSize),
		HasNext:      int64(page*pageSize) < total,
		HasPrev:      page > 1,
	}

	return &pb.GetContributionPaymentsResponse{
		Payments:   payments,
		Pagination: pagination,
	}, nil
}

// Placeholder implementations for remaining methods
func (c *GroupAccountController) UpdatePaymentStatus(ctx context.Context, req *pb.UpdatePaymentStatusRequest) (*pb.UpdatePaymentStatusResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	payment, err := c.groupAccountService.UpdatePaymentStatus(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrGroupPaymentNotFound) {
			return nil, status.Errorf(codes.NotFound, "payment not found")
		}
		if errors.Is(err, services.ErrUnauthorized) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to update payment status: %v", err)
	}

	return &pb.UpdatePaymentStatusResponse{Payment: payment}, nil
}

func (c *GroupAccountController) ProcessScheduledPayments(ctx context.Context, req *pb.ProcessScheduledPaymentsRequest) (*pb.ProcessScheduledPaymentsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	contribution, payments, err := c.groupAccountService.ProcessScheduledPayments(ctx, user.ID, req.ContributionId)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrNotGroupAdmin) {
			return nil, status.Errorf(codes.PermissionDenied, "only admin can process scheduled payments")
		}
		return nil, status.Errorf(codes.Internal, "failed to process scheduled payments: %v", err)
	}

	return &pb.ProcessScheduledPaymentsResponse{
		Contribution:      contribution,
		PaymentsProcessed: payments,
	}, nil
}

func (c *GroupAccountController) GetOverdueContributions(ctx context.Context, req *pb.GetOverdueContributionsRequest) (*pb.GetOverdueContributionsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	contributions, err := c.groupAccountService.GetOverdueContributions(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get overdue contributions: %v", err)
	}

	return &pb.GetOverdueContributionsResponse{Contributions: contributions}, nil
}

func (c *GroupAccountController) GetPayoutSchedule(ctx context.Context, req *pb.GetPayoutScheduleRequest) (*pb.GetPayoutScheduleResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	schedule, err := c.groupAccountService.GetPayoutSchedule(ctx, user.ID, req.ContributionId)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrContributionAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to get payout schedule: %v", err)
	}

	return &pb.GetPayoutScheduleResponse{Schedule: schedule}, nil
}

func (c *GroupAccountController) ProcessPayout(ctx context.Context, req *pb.ProcessPayoutRequest) (*pb.ProcessPayoutResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	transaction, err := c.groupAccountService.ProcessPayout(ctx, user.ID, req.ContributionId)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrNotGroupAdmin) {
			return nil, status.Errorf(codes.PermissionDenied, "only admin can process payouts")
		}
		if errors.Is(err, services.ErrGroupInsufficientFunds) {
			return nil, status.Errorf(codes.FailedPrecondition, "insufficient funds")
		}
		return nil, status.Errorf(codes.Internal, "failed to process payout: %v", err)
	}

	return &pb.ProcessPayoutResponse{Transaction: transaction}, nil
}

func (c *GroupAccountController) UpdatePayoutStatus(ctx context.Context, req *pb.UpdatePayoutStatusRequest) (*pb.UpdatePayoutStatusResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	payout, err := c.groupAccountService.UpdatePayoutStatus(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrPayoutNotFound) {
			return nil, status.Errorf(codes.NotFound, "payout not found")
		}
		if errors.Is(err, services.ErrNotGroupAdmin) {
			return nil, status.Errorf(codes.PermissionDenied, "only admin can update payout status")
		}
		return nil, status.Errorf(codes.Internal, "failed to update payout status: %v", err)
	}

	return &pb.UpdatePayoutStatusResponse{Payout: payout}, nil
}

func (c *GroupAccountController) AdvancePayoutRotation(ctx context.Context, req *pb.AdvancePayoutRotationRequest) (*pb.AdvancePayoutRotationResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	contribution, err := c.groupAccountService.AdvancePayoutRotation(ctx, user.ID, req.ContributionId)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrNotGroupAdmin) {
			return nil, status.Errorf(codes.PermissionDenied, "only admin can advance rotation")
		}
		return nil, status.Errorf(codes.Internal, "failed to advance rotation: %v", err)
	}

	return &pb.AdvancePayoutRotationResponse{Contribution: contribution}, nil
}

func (c *GroupAccountController) GenerateReceipt(ctx context.Context, req *pb.GenerateReceiptRequest) (*pb.GenerateReceiptResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	receipt, err := c.groupAccountService.GenerateReceipt(ctx, user.ID, req.PaymentId)
	if err != nil {
		if errors.Is(err, services.ErrGroupPaymentNotFound) {
			return nil, status.Errorf(codes.NotFound, "payment not found")
		}
		if errors.Is(err, services.ErrUnauthorized) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to generate receipt: %v", err)
	}

	return &pb.GenerateReceiptResponse{Receipt: receipt}, nil
}

func (c *GroupAccountController) GetUserReceipts(ctx context.Context, req *pb.GetUserContributionReceiptsRequest) (*pb.GetUserContributionReceiptsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	page := int(req.Page)
	if page < 1 {
		page = 1
	}

	pageSize := int(req.PageSize)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	receipts, total, err := c.groupAccountService.GetUserReceipts(ctx, user.ID, page, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get receipts: %v", err)
	}

	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))

	pagination := &pb.ReceiptPaginationInfo{
		CurrentPage:  int32(page),
		TotalPages:   totalPages,
		TotalItems:   int32(total),
		ItemsPerPage: int32(pageSize),
		HasNext:      page < int(totalPages),
		HasPrev:      page > 1,
	}

	return &pb.GetUserContributionReceiptsResponse{
		Receipts:   receipts,
		Pagination: pagination,
	}, nil
}

func (c *GroupAccountController) GenerateTranscript(ctx context.Context, req *pb.GenerateTranscriptRequest) (*pb.GenerateTranscriptResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	transcript, err := c.groupAccountService.GenerateTranscript(ctx, user.ID, req.ContributionId)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrContributionAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to generate transcript: %v", err)
	}

	return &pb.GenerateTranscriptResponse{Transcript: transcript}, nil
}

// ============================================================================
// STATISTICS
// ============================================================================

func (c *GroupAccountController) GetGroupStatistics(ctx context.Context, req *pb.GetGroupStatisticsRequest) (*pb.GetGroupStatisticsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	stats, err := c.groupAccountService.GetGroupStatistics(ctx, user.ID, req.GroupId)
	if err != nil {
		if errors.Is(err, services.ErrGroupNotFound) {
			return nil, status.Errorf(codes.NotFound, "group not found")
		}
		if errors.Is(err, services.ErrGroupAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to get statistics: %v", err)
	}

	return stats, nil
}

func (c *GroupAccountController) GetUserContributionStats(ctx context.Context, req *pb.GetUserContributionStatsRequest) (*pb.GetUserContributionStatsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	stats, err := c.groupAccountService.GetUserContributionStats(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user stats: %v", err)
	}

	return stats, nil
}

func (c *GroupAccountController) GetContributionAnalytics(ctx context.Context, req *pb.GetContributionAnalyticsRequest) (*pb.GetContributionAnalyticsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	analytics, err := c.groupAccountService.GetContributionAnalytics(ctx, user.ID, req.ContributionId)
	if err != nil {
		if errors.Is(err, services.ErrContributionNotFound) {
			return nil, status.Errorf(codes.NotFound, "contribution not found")
		}
		if errors.Is(err, services.ErrContributionAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "access denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to get analytics: %v", err)
	}

	return analytics, nil
}

package grpcApi

import (
	"context"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// ReferralController handles gRPC requests for the ReferralService
type ReferralController struct {
	pb.UnimplementedReferralServiceServer
	referralService services.IReferralService
	userService     services.IUserService
	db              *gorm.DB
}

// NewReferralController creates a new ReferralController
func NewReferralController(referralService services.IReferralService, userService services.IUserService, db *gorm.DB) *ReferralController {
	return &ReferralController{
		referralService: referralService,
		userService:     userService,
		db:              db,
	}
}

// ValidateReferralCode validates if a referral code exists and is active
func (c *ReferralController) ValidateReferralCode(ctx context.Context, req *pb.ValidateReferralCodeRequest) (*pb.ValidateReferralCodeResponse, error) {
	// Input validation
	if req.GetCode() == "" {
		return &pb.ValidateReferralCodeResponse{
			IsValid: false,
			Message: "Referral code is required",
		}, nil
	}

	// Call service
	isValid, message, err := c.referralService.ValidateReferralCode(ctx, req.GetCode())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to validate referral code: %v", err)
	}

	return &pb.ValidateReferralCodeResponse{
		IsValid: isValid,
		Message: message,
	}, nil
}

// GetMyReferralCode gets the authenticated user's referral code
func (c *ReferralController) GetMyReferralCode(ctx context.Context, req *pb.GetMyReferralCodeRequest) (*pb.GetMyReferralCodeResponse, error) {
	// Get user ID from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// Get referral code
	referralCode, err := c.referralService.GetMyReferralCode(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "referral code not found: %v", err)
	}

	// Convert to proto
	return &pb.GetMyReferralCodeResponse{
		ReferralCode: &pb.ReferralCode{
			Id:        uint64(referralCode.ID),
			UserId:    uint64(referralCode.UserID),
			Code:      referralCode.Code,
			IsActive:  referralCode.IsActive,
			CreatedAt: timestamppb.New(referralCode.CreatedAt),
			UpdatedAt: timestamppb.New(referralCode.UpdatedAt),
		},
	}, nil
}

// GetMyReferralStats gets aggregated referral statistics for the authenticated user
func (c *ReferralController) GetMyReferralStats(ctx context.Context, req *pb.GetMyReferralStatsRequest) (*pb.GetMyReferralStatsResponse, error) {
	// Get user ID from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// Get stats
	totalReferrals, totalRewardsEarned, pendingRewards, currency, err := c.referralService.GetMyReferralStats(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get referral stats: %v", err)
	}

	return &pb.GetMyReferralStatsResponse{
		Stats: &pb.ReferralStats{
			TotalReferrals:     totalReferrals,
			TotalRewardsEarned: int32(totalRewardsEarned),
			PendingRewards:     int32(pendingRewards),
			Currency:           currency,
		},
	}, nil
}

// GetMyReferrals gets a paginated list of referrals made by the authenticated user
func (c *ReferralController) GetMyReferrals(ctx context.Context, req *pb.GetMyReferralsRequest) (*pb.GetMyReferralsResponse, error) {
	// Get user ID from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// Default pagination
	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	// Get referrals
	transactions, err := c.referralService.GetMyReferrals(ctx, user.ID, page, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get referrals: %v", err)
	}

	// Convert to proto
	var protoTransactions []*pb.ReferralTransaction
	for _, tx := range transactions {
		protoTx := &pb.ReferralTransaction{
			Id:                   uint64(tx.ID),
			ReferrerUserId:       uint64(tx.ReferrerUserID),
			RefereeUserId:        uint64(tx.RefereeUserID),
			ReferralCodeUsed:     tx.ReferralCodeUsed,
			Status:               convertReferralStatusToProto(tx.Status),
			ReferrerRewardAmount: int32(tx.ReferrerRewardAmount),
			RefereeRewardAmount:  int32(tx.RefereeRewardAmount),
			Currency:             tx.Currency,
			CreatedAt:            timestamppb.New(tx.CreatedAt),
		}
		if tx.CompletedAt != nil {
			protoTx.CompletedAt = timestamppb.New(*tx.CompletedAt)
		}
		if tx.FailureReason != nil {
			protoTx.FailureReason = *tx.FailureReason
		}
		protoTransactions = append(protoTransactions, protoTx)
	}

	return &pb.GetMyReferralsResponse{
		Referrals: protoTransactions,
		Page:      int32(page),
		PageSize:  int32(pageSize),
	}, nil
}

// GetReferralLeaderboard gets the top referrers
func (c *ReferralController) GetReferralLeaderboard(ctx context.Context, req *pb.GetReferralLeaderboardRequest) (*pb.GetReferralLeaderboardResponse, error) {
	// Default limit
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	// Get leaderboard
	results, err := c.referralService.GetReferralLeaderboard(ctx, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get leaderboard: %v", err)
	}

	// Convert to proto
	var entries []*pb.LeaderboardEntry
	for _, result := range results {
		entry := &pb.LeaderboardEntry{
			UserId:        uint64(result["user_id"].(uint)),
			FirstName:     result["first_name"].(string),
			LastName:      result["last_name"].(string),
			Username:      result["username"].(string),
			TotalReferrals: result["total_referrals"].(int64),
			Rank:          int32(result["rank"].(int)),
		}
		entries = append(entries, entry)
	}

	return &pb.GetReferralLeaderboardResponse{
		Entries: entries,
	}, nil
}

// GetCountryRewardConfig gets reward configuration for a specific country
func (c *ReferralController) GetCountryRewardConfig(ctx context.Context, req *pb.GetCountryRewardConfigRequest) (*pb.GetCountryRewardConfigResponse, error) {
	// Input validation
	if req.GetCountryCode() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "country code is required")
	}

	// Get config
	config, err := c.referralService.GetCountryRewardConfig(ctx, req.GetCountryCode())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get country reward config: %v", err)
	}

	// Convert to proto
	protoConfig := &pb.CountryRewardConfig{
		Id:             uint64(config.ID),
		CountryCode:    config.CountryCode,
		Currency:       config.Currency,
		ReferrerReward: int32(config.ReferrerReward),
		RefereeReward:  int32(config.RefereeReward),
		IsActive:       config.IsActive,
	}
	if !config.CreatedAt.IsZero() {
		protoConfig.CreatedAt = timestamppb.New(config.CreatedAt)
	}
	if !config.UpdatedAt.IsZero() {
		protoConfig.UpdatedAt = timestamppb.New(config.UpdatedAt)
	}

	return &pb.GetCountryRewardConfigResponse{
		Config: protoConfig,
	}, nil
}

// RecordReferral records a referral transaction (admin only)
func (c *ReferralController) RecordReferral(ctx context.Context, req *pb.RecordReferralRequest) (*pb.RecordReferralResponse, error) {
	// This method is typically called internally during user signup
	// Not exposed to clients directly
	return &pb.RecordReferralResponse{
		Success: false,
		Message: "This endpoint is for internal use only",
	}, status.Errorf(codes.Unimplemented, "method not implemented for external use")
}

// CreditReferralRewards credits rewards to both referrer and referee (admin only)
func (c *ReferralController) CreditReferralRewards(ctx context.Context, req *pb.CreditReferralRewardsRequest) (*pb.CreditReferralRewardsResponse, error) {
	// This method is typically called internally during user signup
	// Not exposed to clients directly
	return &pb.CreditReferralRewardsResponse{
		Success: false,
		Message: "This endpoint is for internal use only",
	}, status.Errorf(codes.Unimplemented, "method not implemented for external use")
}

// Helper function to convert model ReferralStatus to proto
func convertReferralStatusToProto(status models.ReferralStatus) pb.ReferralStatus {
	switch status {
	case models.ReferralStatusPending:
		return pb.ReferralStatus_REFERRAL_STATUS_PENDING
	case models.ReferralStatusCompleted:
		return pb.ReferralStatus_REFERRAL_STATUS_COMPLETED
	case models.ReferralStatusFailed:
		return pb.ReferralStatus_REFERRAL_STATUS_FAILED
	case models.ReferralStatusCancelled:
		return pb.ReferralStatus_REFERRAL_STATUS_CANCELLED
	default:
		return pb.ReferralStatus_REFERRAL_STATUS_PENDING
	}
}

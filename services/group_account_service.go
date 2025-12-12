package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrGroupNotFound             = errors.New("group account not found")
	ErrGroupAccessDenied         = errors.New("access denied to group account")
	ErrGroupMemberNotFound       = errors.New("group member not found")
	ErrGroupMemberAlreadyExists  = errors.New("user is already a member of this group")
	ErrContributionNotFound      = errors.New("contribution not found")
	ErrContributionAccessDenied  = errors.New("access denied to contribution")
	ErrGroupPaymentNotFound      = errors.New("group payment not found")
	ErrPayoutNotFound            = errors.New("payout not found")
	ErrGroupInsufficientFunds    = errors.New("insufficient funds in contribution")
	ErrInvalidPayoutRecipient    = errors.New("invalid payout recipient")
	ErrNotGroupAdmin             = errors.New("only group admin can perform this action")
	ErrUnauthorized              = errors.New("unauthorized access")
)

// ============================================================================
// INTERFACE
// ============================================================================

type IGroupAccountService interface {
	// Group Management
	CreateGroup(ctx context.Context, userID uint, req *pb.CreateGroupRequest) (*pb.GroupAccountMessage, error)
	GetGroup(ctx context.Context, userID uint, groupID string) (*pb.GroupAccountMessage, error)
	ListUserGroups(ctx context.Context, userID uint, page, pageSize int, statusFilter string) ([]*pb.GroupAccountMessage, int64, error)
	UpdateGroup(ctx context.Context, userID uint, req *pb.UpdateGroupRequest) (*pb.GroupAccountMessage, error)
	DeleteGroup(ctx context.Context, userID uint, groupID string) error

	// Member Management
	GetGroupMembers(ctx context.Context, userID uint, groupID string) ([]*pb.GroupMemberMessage, error)
	AddMember(ctx context.Context, userID uint, req *pb.AddMemberRequest) (*pb.GroupMemberMessage, error)
	UpdateMemberRole(ctx context.Context, userID uint, req *pb.UpdateMemberRoleRequest) (*pb.GroupMemberMessage, error)
	RemoveMember(ctx context.Context, userID uint, groupID, memberID string) error
	SearchUsers(ctx context.Context, query string, limit int) ([]*pb.GroupMemberMessage, error)

	// Contribution Management
	CreateContribution(ctx context.Context, userID uint, req *pb.CreateContributionRequest) (*pb.ContributionMessage, error)
	GetContribution(ctx context.Context, userID uint, contributionID string) (*pb.ContributionMessage, error)
	ListGroupContributions(ctx context.Context, userID uint, groupID string, page, pageSize int, statusFilter string) ([]*pb.ContributionMessage, int64, error)
	UpdateContribution(ctx context.Context, userID uint, req *pb.UpdateContributionRequest) (*pb.ContributionMessage, error)
	DeleteContribution(ctx context.Context, userID uint, contributionID string) error

	// Payment Operations
	MakePayment(ctx context.Context, userID uint, req *pb.MakePaymentRequest) (*pb.ContributionPaymentMessage, error)
	GetContributionPayments(ctx context.Context, userID uint, contributionID string, page, pageSize int) ([]*pb.ContributionPaymentMessage, int64, error)
	UpdatePaymentStatus(ctx context.Context, userID uint, req *pb.UpdatePaymentStatusRequest) (*pb.ContributionPaymentMessage, error)

	// Scheduled Payments
	ProcessScheduledPayments(ctx context.Context, userID uint, contributionID string) (*pb.ContributionMessage, []*pb.ContributionPaymentMessage, error)
	GetOverdueContributions(ctx context.Context, userID uint) ([]*pb.ContributionMessage, error)

	// Payout Operations
	GetPayoutSchedule(ctx context.Context, userID uint, contributionID string) ([]*pb.PayoutScheduleMessage, error)
	ProcessPayout(ctx context.Context, userID uint, contributionID string) (*pb.PayoutTransactionMessage, error)
	UpdatePayoutStatus(ctx context.Context, userID uint, req *pb.UpdatePayoutStatusRequest) (*pb.PayoutTransactionMessage, error)
	AdvancePayoutRotation(ctx context.Context, userID uint, contributionID string) (*pb.ContributionMessage, error)

	// Receipt Operations
	GenerateReceipt(ctx context.Context, userID uint, paymentID string) (*pb.ContributionReceiptMessage, error)
	GetUserReceipts(ctx context.Context, userID uint, page, pageSize int) ([]*pb.ContributionReceiptMessage, int64, error)

	// Transcript Operations
	GenerateTranscript(ctx context.Context, userID uint, contributionID string) (*pb.ContributionTranscriptMessage, error)

	// Statistics
	GetGroupStatistics(ctx context.Context, userID uint, groupID string) (*pb.GetGroupStatisticsResponse, error)
	GetUserContributionStats(ctx context.Context, userID uint) (*pb.GetUserContributionStatsResponse, error)
	GetContributionAnalytics(ctx context.Context, userID uint, contributionID string) (*pb.GetContributionAnalyticsResponse, error)
}

// ============================================================================
// SERVICE STRUCT
// ============================================================================

type GroupAccountService struct {
	db *gorm.DB
}

func NewGroupAccountService(db *gorm.DB) IGroupAccountService {
	return &GroupAccountService{db: db}
}

// ============================================================================
// GROUP MANAGEMENT
// ============================================================================

func (s *GroupAccountService) CreateGroup(ctx context.Context, userID uint, req *pb.CreateGroupRequest) (*pb.GroupAccountMessage, error) {
	group := &models.GroupAccount{
		Name:        req.Name,
		Description: req.Description,
		AdminID:     userID,
		Status:      models.GroupAccountStatusActive,
	}

	if err := s.db.WithContext(ctx).Create(group).Error; err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	// Add creator as admin member
	member := &models.GroupMember{
		GroupID: group.ID,
		UserID:  userID,
		Role:    models.GroupMemberRoleAdmin,
		Status:  models.GroupMemberStatusActive,
	}

	if err := s.db.WithContext(ctx).Create(member).Error; err != nil {
		return nil, fmt.Errorf("failed to add admin member: %w", err)
	}

	// Reload with members
	if err := s.db.WithContext(ctx).Preload("Members.User").Preload("Contributions").First(group, group.ID).Error; err != nil {
		return nil, err
	}

	return s.groupToProto(group), nil
}

func (s *GroupAccountService) GetGroup(ctx context.Context, userID uint, groupID string) (*pb.GroupAccountMessage, error) {
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).
		Preload("Members.User").
		Preload("Contributions").
		First(&group, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}

	// Check access
	if !s.userHasGroupAccess(userID, &group) {
		return nil, ErrGroupAccessDenied
	}

	return s.groupToProto(&group), nil
}

func (s *GroupAccountService) ListUserGroups(ctx context.Context, userID uint, page, pageSize int, statusFilter string) ([]*pb.GroupAccountMessage, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// Get groups where user is admin or member
	// Use subquery to get distinct group IDs
	subQuery := s.db.Model(&models.GroupAccount{}).
		Select("group_accounts.id").
		Joins("LEFT JOIN group_members ON group_members.group_id = group_accounts.id").
		Where("group_accounts.admin_id = ? OR group_members.user_id = ?", userID, userID)

	if statusFilter != "" {
		subQuery = subQuery.Where("group_accounts.status = ?", statusFilter)
	}

	// Count total matching groups
	var total int64
	if err := subQuery.Distinct().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get the actual group data with preloads
	var groupIDs []uint
	if err := subQuery.Distinct().Limit(pageSize).Offset(offset).Pluck("group_accounts.id", &groupIDs).Error; err != nil {
		return nil, 0, err
	}

	var groups []models.GroupAccount
	if len(groupIDs) > 0 {
		if err := s.db.WithContext(ctx).
			Preload("Members.User").
			Preload("Contributions").
			Where("id IN ?", groupIDs).
			Find(&groups).Error; err != nil {
			return nil, 0, err
		}
	}

	protoGroups := make([]*pb.GroupAccountMessage, len(groups))
	for i, group := range groups {
		protoGroups[i] = s.groupToProto(&group)
	}

	return protoGroups, total, nil
}

func (s *GroupAccountService) UpdateGroup(ctx context.Context, userID uint, req *pb.UpdateGroupRequest) (*pb.GroupAccountMessage, error) {
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", req.GroupId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}

	// Only admin can update
	if group.AdminID != userID {
		return nil, ErrNotGroupAdmin
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Status != pb.GroupAccountStatus_GROUP_ACCOUNT_STATUS_UNSPECIFIED {
		updates["status"] = protoToGroupStatus(req.Status)
	}
	if req.Metadata != "" {
		var metadata models.JSONB
		if err := json.Unmarshal([]byte(req.Metadata), &metadata); err == nil {
			updates["metadata"] = metadata
		}
	}

	if err := s.db.WithContext(ctx).Model(&group).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Reload
	if err := s.db.WithContext(ctx).Preload("Members.User").Preload("Contributions").First(&group, group.ID).Error; err != nil {
		return nil, err
	}

	return s.groupToProto(&group), nil
}

func (s *GroupAccountService) DeleteGroup(ctx context.Context, userID uint, groupID string) error {
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrGroupNotFound
		}
		return err
	}

	// Only admin can delete
	if group.AdminID != userID {
		return ErrNotGroupAdmin
	}

	return s.db.WithContext(ctx).Delete(&group).Error
}

// ============================================================================
// MEMBER MANAGEMENT
// ============================================================================

func (s *GroupAccountService) GetGroupMembers(ctx context.Context, userID uint, groupID string) ([]*pb.GroupMemberMessage, error) {
	// Check access first
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, ErrGroupAccessDenied
	}

	var members []models.GroupMember
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("group_id = ?", groupID).
		Find(&members).Error; err != nil {
		return nil, err
	}

	protoMembers := make([]*pb.GroupMemberMessage, len(members))
	for i, member := range members {
		protoMembers[i] = s.memberToProto(&member)
	}

	return protoMembers, nil
}

func (s *GroupAccountService) AddMember(ctx context.Context, userID uint, req *pb.AddMemberRequest) (*pb.GroupMemberMessage, error) {
	// Check if user is admin
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", req.GroupId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}

	if group.AdminID != userID {
		return nil, ErrNotGroupAdmin
	}

	// Convert GroupId from string to uint
	groupID, err := strconv.ParseUint(req.GroupId, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid group ID: %w", err)
	}

	// Find or create user based on provided identifier
	var targetUser models.User
	var isNewPartialUser bool

	if req.UserId > 0 {
		// User ID provided - validate exists
		if err := s.db.WithContext(ctx).First(&targetUser, "id = ?", req.UserId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("user with ID %d not found", req.UserId)
			}
			return nil, fmt.Errorf("failed to verify user: %w", err)
		}
	} else if req.Email != "" {
		// Email provided - find or create partial user
		err := s.db.WithContext(ctx).Where("email = ?", req.Email).First(&targetUser).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Create partial user
				targetUser = models.User{
					Email:     req.Email,
					IsPartial: true,
					InvitedBy: &userID,
				}
				if err := s.db.WithContext(ctx).Create(&targetUser).Error; err != nil {
					return nil, fmt.Errorf("failed to create partial user: %w", err)
				}
				isNewPartialUser = true
			} else {
				return nil, fmt.Errorf("failed to lookup user by email: %w", err)
			}
		}
	} else if req.PhoneNumber != "" {
		// Phone provided - find or create partial user
		err := s.db.WithContext(ctx).Where("phone_number = ?", req.PhoneNumber).First(&targetUser).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Create partial user
				targetUser = models.User{
					PhoneNumber: req.PhoneNumber,
					Email:       "", // Will be set later
					IsPartial:   true,
					InvitedBy:   &userID,
				}
				if err := s.db.WithContext(ctx).Create(&targetUser).Error; err != nil {
					return nil, fmt.Errorf("failed to create partial user: %w", err)
				}
				isNewPartialUser = true
			} else {
				return nil, fmt.Errorf("failed to lookup user by phone: %w", err)
			}
		}
	} else if req.LookupUsername != "" {
		// Username provided - find existing FULLY REGISTERED user only
		// (can't create partial user with just username, and partial users don't have usernames)
		err := s.db.WithContext(ctx).Where("username = ? AND is_partial = ?", req.LookupUsername, false).First(&targetUser).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("user with username '%s' not found. Use email or contact to invite new users", req.LookupUsername)
			}
			return nil, fmt.Errorf("failed to lookup user by username: %w", err)
		}
	} else {
		return nil, fmt.Errorf("must provide user_id, email, phone_number, or username")
	}

	// Check if user already a member
	var existing models.GroupMember
	if err := s.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, targetUser.ID).
		First(&existing).Error; err == nil {
		return nil, ErrGroupMemberAlreadyExists
	}

	member := &models.GroupMember{
		GroupID: uint(groupID),
		UserID:  targetUser.ID,
		Role:    protoToMemberRole(req.Role),
		Status:  models.GroupMemberStatusActive,
	}

	if err := s.db.WithContext(ctx).Create(member).Error; err != nil {
		return nil, err
	}

	// If we created a new partial user, create an invitation record
	if isNewPartialUser {
		invitationType := models.InvitationTypeEmail
		if req.PhoneNumber != "" {
			invitationType = models.InvitationTypeSMS
		}

		invitation := &models.Invitation{
			InviterID:   userID,
			InviteeID:   &targetUser.ID,
			Type:        invitationType,
			Status:      models.InvitationStatusPending,
			Email:       &req.Email,
			PhoneNumber: &req.PhoneNumber,
			GroupID:     &group.ID,
			ExpiresAt:   time.Now().Add(30 * 24 * time.Hour), // 30 days
		}
		if err := s.db.WithContext(ctx).Create(invitation).Error; err != nil {
			// Log error but don't fail the member creation
			fmt.Printf("Failed to create invitation record: %v\n", err)
		}

		// TODO: Enqueue background job to send invitation via email/SMS
		// This will be implemented with the worker system
	}

	// Reload with user
	if err := s.db.WithContext(ctx).Preload("User").First(member, member.ID).Error; err != nil {
		return nil, err
	}

	return s.memberToProto(member), nil
}

func (s *GroupAccountService) UpdateMemberRole(ctx context.Context, userID uint, req *pb.UpdateMemberRoleRequest) (*pb.GroupMemberMessage, error) {
	// Check if user is admin
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", req.GroupId).Error; err != nil {
		return nil, ErrGroupNotFound
	}

	if group.AdminID != userID {
		return nil, ErrNotGroupAdmin
	}

	var member models.GroupMember
	if err := s.db.WithContext(ctx).First(&member, "id = ?", req.MemberId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupMemberNotFound
		}
		return nil, err
	}

	member.Role = protoToMemberRole(req.NewRole)
	if err := s.db.WithContext(ctx).Save(&member).Error; err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Preload("User").First(&member, member.ID).Error; err != nil {
		return nil, err
	}

	return s.memberToProto(&member), nil
}

func (s *GroupAccountService) RemoveMember(ctx context.Context, userID uint, groupID, memberID string) error {
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", groupID).Error; err != nil {
		return ErrGroupNotFound
	}

	if group.AdminID != userID {
		return ErrNotGroupAdmin
	}

	return s.db.WithContext(ctx).Delete(&models.GroupMember{}, "id = ?", memberID).Error
}

func (s *GroupAccountService) SearchUsers(ctx context.Context, query string, limit int) ([]*pb.GroupMemberMessage, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	var users []models.User
	// Search by email, username, phone, first name, or last name
	// Only return non-partial users (fully registered)
	if err := s.db.WithContext(ctx).
		Where("is_partial = ? AND (email ILIKE ? OR username ILIKE ? OR phone_number ILIKE ? OR first_name ILIKE ? OR last_name ILIKE ?)",
			false, "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%").
		Limit(limit).
		Find(&users).Error; err != nil {
		return nil, err
	}

	protoMembers := make([]*pb.GroupMemberMessage, len(users))
	for i, user := range users {
		userName := user.FirstName + " " + user.LastName
		if user.Username != nil && *user.Username != "" {
			userName = *user.Username + " (" + user.FirstName + " " + user.LastName + ")"
		}

		protoMembers[i] = &pb.GroupMemberMessage{
			UserId:      uint64(user.ID),
			UserName:    userName,
			Email:       user.Email,
			PhoneNumber: user.PhoneNumber,
			Role:        pb.GroupMemberRole_GROUP_MEMBER_ROLE_MEMBER,
			Status:      pb.GroupMemberStatus_GROUP_MEMBER_STATUS_ACTIVE,
		}
	}

	return protoMembers, nil
}

// ============================================================================
// CONTRIBUTION MANAGEMENT
// ============================================================================

func (s *GroupAccountService) CreateContribution(ctx context.Context, userID uint, req *pb.CreateContributionRequest) (*pb.ContributionMessage, error) {
	// Verify user has access to group
	// Convert GroupId from string to uint
	groupID, err := strconv.ParseUint(req.GroupId, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid group ID: %w", err)
	}

	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", groupID).Error; err != nil {
		return nil, ErrGroupNotFound
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, ErrGroupAccessDenied
	}

	contribution := &models.Contribution{
		GroupID:       uint(groupID),
		Title:         req.Title,
		Description:   req.Description,
		TargetAmount:  int64(req.TargetAmount),
		CurrentAmount: 0,
		Currency:      req.Currency,
		Deadline:      req.Deadline.AsTime(),
		Status:        models.ContributionStatusActive,
		CreatedBy:     userID,
		Type:          protoToContributionType(req.Type),
	}

	// Set type-specific fields
	if req.Frequency != pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_UNSPECIFIED {
		freq := protoToContributionFrequency(req.Frequency)
		contribution.Frequency = &freq
	}
	if req.RegularAmount > 0 {
		amt := int64(req.RegularAmount)
		contribution.RegularAmount = &amt
	}
	if req.StartDate != nil {
		startDate := req.StartDate.AsTime()
		contribution.StartDate = &startDate
	}
	if req.TotalCycles > 0 {
		cycles := int(req.TotalCycles)
		contribution.TotalCycles = &cycles
	}
	if req.PenaltyAmount > 0 {
		penalty := int64(req.PenaltyAmount)
		contribution.PenaltyAmount = &penalty
	}
	if req.GracePeriodDays > 0 {
		grace := int(req.GracePeriodDays)
		contribution.GracePeriodDays = &grace
	}
	if req.MinimumBalance > 0 {
		minBal := int64(req.MinimumBalance)
		contribution.MinimumBalance = &minBal
	}

	contribution.AutoPayEnabled = req.AutoPayEnabled
	contribution.AllowPartialPayments = req.AllowPartialPayments

	if err := s.db.WithContext(ctx).Create(contribution).Error; err != nil {
		return nil, err
	}

	// Create payout schedule if rotating savings
	if req.Type == pb.ContributionType_CONTRIBUTION_TYPE_ROTATING_SAVINGS && len(req.MemberRotationOrder) > 0 {
		if err := s.createPayoutSchedule(ctx, contribution.ID, req.MemberRotationOrder, req); err != nil {
			return nil, err
		}
	}

	// Reload
	if err := s.db.WithContext(ctx).
		Preload("Payments").
		Preload("PayoutSchedules").
		First(contribution, contribution.ID).Error; err != nil {
		return nil, err
	}

	return s.contributionToProto(contribution), nil
}

func (s *GroupAccountService) GetContribution(ctx context.Context, userID uint, contributionID string) (*pb.ContributionMessage, error) {
	var contribution models.Contribution
	if err := s.db.WithContext(ctx).
		Preload("Payments").
		Preload("PayoutSchedules").
		Preload("PayoutHistory").
		First(&contribution, "id = ?", contributionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrContributionNotFound
		}
		return nil, err
	}

	// Check access
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, ErrContributionAccessDenied
	}

	return s.contributionToProto(&contribution), nil
}

func (s *GroupAccountService) ListGroupContributions(ctx context.Context, userID uint, groupID string, page, pageSize int, statusFilter string) ([]*pb.ContributionMessage, int64, error) {
	// Check access
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", groupID).Error; err != nil {
		return nil, 0, ErrGroupNotFound
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, 0, ErrGroupAccessDenied
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	query := s.db.WithContext(ctx).Where("group_id = ?", groupID)
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}

	var total int64
	if err := query.Model(&models.Contribution{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var contributions []models.Contribution
	if err := query.
		Preload("Payments").
		Preload("PayoutSchedules").
		Limit(pageSize).
		Offset(offset).
		Order("created_at DESC").
		Find(&contributions).Error; err != nil {
		return nil, 0, err
	}

	protoContributions := make([]*pb.ContributionMessage, len(contributions))
	for i, contrib := range contributions {
		protoContributions[i] = s.contributionToProto(&contrib)
	}

	return protoContributions, total, nil
}

func (s *GroupAccountService) UpdateContribution(ctx context.Context, userID uint, req *pb.UpdateContributionRequest) (*pb.ContributionMessage, error) {
	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", req.ContributionId).Error; err != nil {
		return nil, ErrContributionNotFound
	}

	// Check access - must be creator or group admin
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if contribution.CreatedBy != userID && group.AdminID != userID {
		return nil, ErrContributionAccessDenied
	}

	updates := map[string]interface{}{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.TargetAmount > 0 {
		updates["target_amount"] = int64(req.TargetAmount)
	}
	if req.Deadline != nil {
		updates["deadline"] = req.Deadline.AsTime()
	}
	if req.Status != pb.ContributionStatus_CONTRIBUTION_STATUS_UNSPECIFIED {
		updates["status"] = protoToContributionStatus(req.Status)
	}

	if err := s.db.WithContext(ctx).Model(&contribution).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Reload
	if err := s.db.WithContext(ctx).Preload("Payments").Preload("PayoutSchedules").First(&contribution, contribution.ID).Error; err != nil {
		return nil, err
	}

	return s.contributionToProto(&contribution), nil
}

func (s *GroupAccountService) DeleteContribution(ctx context.Context, userID uint, contributionID string) error {
	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", contributionID).Error; err != nil {
		return ErrContributionNotFound
	}

	// Only creator or group admin can delete
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return err
	}

	if contribution.CreatedBy != userID && group.AdminID != userID {
		return ErrContributionAccessDenied
	}

	return s.db.WithContext(ctx).Delete(&contribution).Error
}

// ============================================================================
// PAYMENT OPERATIONS
// ============================================================================

func (s *GroupAccountService) MakePayment(ctx context.Context, userID uint, req *pb.MakePaymentRequest) (*pb.ContributionPaymentMessage, error) {
	// Convert ContributionId from string to uint
	contributionID, err := strconv.ParseUint(req.ContributionId, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid contribution ID: %w", err)
	}

	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", contributionID).Error; err != nil {
		return nil, ErrContributionNotFound
	}

	// Verify user is a member
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, ErrGroupAccessDenied
	}

	// Create payment
	payment := &models.ContributionPayment{
		ContributionID: uint(contributionID),
		GroupID:        contribution.GroupID,
		UserID:         userID,
		Amount:         int64(req.Amount),
		Currency:       contribution.Currency,
		PaymentDate:    time.Now(),
		Status:         models.PaymentStatusCompleted, // In production, would be pending until confirmed
	}

	if req.Notes != "" {
		payment.Notes = &req.Notes
	}

	// Use transaction to update both payment and contribution
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(payment).Error; err != nil {
			return err
		}

		// Update contribution current amount
		if err := tx.Model(&contribution).
			Update("current_amount", gorm.Expr("current_amount + ?", payment.Amount)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Reload payment with user info
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}

	return s.paymentToProto(payment, &user), nil
}

func (s *GroupAccountService) GetContributionPayments(ctx context.Context, userID uint, contributionID string, page, pageSize int) ([]*pb.ContributionPaymentMessage, int64, error) {
	// Check access
	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", contributionID).Error; err != nil {
		return nil, 0, ErrContributionNotFound
	}

	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, 0, err
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, 0, ErrContributionAccessDenied
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	var total int64
	query := s.db.WithContext(ctx).Model(&models.ContributionPayment{}).Where("contribution_id = ?", contributionID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var payments []models.ContributionPayment
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("contribution_id = ?", contributionID).
		Limit(pageSize).
		Offset(offset).
		Order("payment_date DESC").
		Find(&payments).Error; err != nil {
		return nil, 0, err
	}

	protoPayments := make([]*pb.ContributionPaymentMessage, len(payments))
	for i, payment := range payments {
		protoPayments[i] = s.paymentToProto(&payment, &payment.User)
	}

	return protoPayments, total, nil
}

// ============================================================================
// STATISTICS
// ============================================================================

func (s *GroupAccountService) GetGroupStatistics(ctx context.Context, userID uint, groupID string) (*pb.GetGroupStatisticsResponse, error) {
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).
		Preload("Members").
		Preload("Contributions").
		First(&group, "id = ?", groupID).Error; err != nil {
		return nil, ErrGroupNotFound
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, ErrGroupAccessDenied
	}

	var totalTarget, totalCurrent uint64
	completedCount := 0
	activeCount := 0

	for _, contrib := range group.Contributions {
		totalTarget += uint64(contrib.TargetAmount)
		totalCurrent += uint64(contrib.CurrentAmount)
		if contrib.Status == models.ContributionStatusCompleted {
			completedCount++
		} else if contrib.Status == models.ContributionStatusActive {
			activeCount++
		}
	}

	completionRate := float64(0)
	if len(group.Contributions) > 0 {
		completionRate = float64(completedCount) / float64(len(group.Contributions)) * 100
	}

	return &pb.GetGroupStatisticsResponse{
		MemberCount:            int32(len(group.Members)),
		TotalContributions:     int32(len(group.Contributions)),
		CompletedContributions: int32(completedCount),
		ActiveContributions:    int32(activeCount),
		TotalTargetAmount:      totalTarget,
		TotalCurrentAmount:     totalCurrent,
		CompletionRate:         completionRate,
	}, nil
}

func (s *GroupAccountService) GetUserContributionStats(ctx context.Context, userID uint) (*pb.GetUserContributionStatsResponse, error) {
	var totalPayments int64
	var totalAmount uint64
	var groupsCount int64

	// Count total payments
	if err := s.db.WithContext(ctx).
		Model(&models.ContributionPayment{}).
		Where("user_id = ?", userID).
		Count(&totalPayments).Error; err != nil {
		return nil, err
	}

	// Sum total amount
	var sumResult struct {
		Total int64
	}
	if err := s.db.WithContext(ctx).
		Model(&models.ContributionPayment{}).
		Select("COALESCE(SUM(amount), 0) as total").
		Where("user_id = ?", userID).
		Scan(&sumResult).Error; err != nil {
		return nil, err
	}
	totalAmount = uint64(sumResult.Total)

	// Count groups
	if err := s.db.WithContext(ctx).
		Model(&models.GroupAccount{}).
		Where("admin_id = ?", userID).
		Or("id IN (SELECT group_id FROM group_members WHERE user_id = ?)", userID).
		Count(&groupsCount).Error; err != nil {
		return nil, err
	}

	averagePayment := float64(0)
	if totalPayments > 0 {
		averagePayment = float64(totalAmount) / float64(totalPayments)
	}

	return &pb.GetUserContributionStatsResponse{
		TotalPayments:  int32(totalPayments),
		TotalAmount:    totalAmount,
		GroupsCount:    int32(groupsCount),
		AveragePayment: averagePayment,
	}, nil
}

// ============================================================================
// ADVANCED PAYMENT OPERATIONS
// ============================================================================

func (s *GroupAccountService) UpdatePaymentStatus(ctx context.Context, userID uint, req *pb.UpdatePaymentStatusRequest) (*pb.ContributionPaymentMessage, error) {
	var payment models.ContributionPayment
	if err := s.db.WithContext(ctx).Preload("User").First(&payment, "id = ?", req.PaymentId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupPaymentNotFound
		}
		return nil, err
	}

	// Check access - must be admin or creator
	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", payment.ContributionID).Error; err != nil {
		return nil, err
	}

	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if group.AdminID != userID && contribution.CreatedBy != userID {
		return nil, ErrUnauthorized
	}

	// Update status
	updates := map[string]interface{}{
		"status": protoToPaymentStatus(req.Status),
	}

	if req.TransactionId != "" {
		updates["transaction_id"] = req.TransactionId
	}

	if err := s.db.WithContext(ctx).Model(&payment).Updates(updates).Error; err != nil {
		return nil, err
	}

	return s.paymentToProto(&payment, &payment.User), nil
}

// ============================================================================
// SCHEDULED PAYMENTS
// ============================================================================

func (s *GroupAccountService) ProcessScheduledPayments(ctx context.Context, userID uint, contributionID string) (*pb.ContributionMessage, []*pb.ContributionPaymentMessage, error) {
	// Convert contributionID from string to uint
	contribID, err := strconv.ParseUint(contributionID, 10, 64)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid contribution ID: %w", err)
	}

	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", contribID).Error; err != nil {
		return nil, nil, ErrContributionNotFound
	}

	// Check access
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, nil, err
	}

	if group.AdminID != userID {
		return nil, nil, ErrNotGroupAdmin
	}

	// Only process recurring or rotating savings contributions
	if contribution.Type != models.ContributionTypeRecurring && contribution.Type != models.ContributionTypeRotatingSavings {
		return nil, nil, errors.New("contribution is not recurring or rotating savings")
	}

	if !contribution.AutoPayEnabled {
		return nil, nil, errors.New("auto-pay is not enabled for this contribution")
	}

	// Get all active members
	var members []models.GroupMember
	if err := s.db.WithContext(ctx).
		Where("group_id = ? AND status = ?", contribution.GroupID, models.GroupMemberStatusActive).
		Find(&members).Error; err != nil {
		return nil, nil, err
	}

	var processedPayments []*pb.ContributionPaymentMessage

	// Process scheduled payments for each member
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, member := range members {
			// Check if payment is due
			if contribution.NextPaymentDate != nil && contribution.NextPaymentDate.After(time.Now()) {
				continue
			}

			amount := int64(0)
			if contribution.RegularAmount != nil {
				amount = *contribution.RegularAmount
			}

			payment := &models.ContributionPayment{
				ContributionID: uint(contribID),
				GroupID:        contribution.GroupID,
				UserID:         member.UserID,
				Amount:         amount,
				Currency:       contribution.Currency,
				PaymentDate:    time.Now(),
				Status:         models.PaymentStatusCompleted,
			}

			if err := tx.Create(payment).Error; err != nil {
				return err
			}

			// Update contribution amount
			if err := tx.Model(&contribution).
				Update("current_amount", gorm.Expr("current_amount + ?", amount)).Error; err != nil {
				return err
			}

			// Load user info for proto conversion
			var user models.User
			if err := tx.First(&user, member.UserID).Error; err != nil {
				return err
			}

			processedPayments = append(processedPayments, s.paymentToProto(payment, &user))
		}

		// Update next payment date
		if contribution.Frequency != nil {
			nextDate := s.calculateNextPaymentDate(time.Now(), *contribution.Frequency)
			if err := tx.Model(&contribution).
				Update("next_payment_date", nextDate).Error; err != nil {
				return err
			}
		}

		// Increment cycle
		if err := tx.Model(&contribution).
			Update("current_cycle", gorm.Expr("current_cycle + 1")).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	// Reload contribution
	if err := s.db.WithContext(ctx).
		Preload("Payments").
		Preload("PayoutSchedules").
		First(&contribution, contribution.ID).Error; err != nil {
		return nil, nil, err
	}

	return s.contributionToProto(&contribution), processedPayments, nil
}

func (s *GroupAccountService) GetOverdueContributions(ctx context.Context, userID uint) ([]*pb.ContributionMessage, error) {
	var contributions []models.Contribution

	// Find contributions where user is a member and payment is overdue
	if err := s.db.WithContext(ctx).
		Joins("JOIN group_members ON group_members.group_id = contributions.group_id").
		Where("group_members.user_id = ? AND contributions.status = ? AND contributions.next_payment_date < ? AND contributions.type IN (?)",
			userID, models.ContributionStatusActive, time.Now(), []models.ContributionType{models.ContributionTypeRecurring, models.ContributionTypeRotatingSavings}).
		Preload("Payments").
		Preload("PayoutSchedules").
		Find(&contributions).Error; err != nil {
		return nil, err
	}

	protoContributions := make([]*pb.ContributionMessage, len(contributions))
	for i, contrib := range contributions {
		protoContributions[i] = s.contributionToProto(&contrib)
	}

	return protoContributions, nil
}

// ============================================================================
// PAYOUT OPERATIONS
// ============================================================================

func (s *GroupAccountService) GetPayoutSchedule(ctx context.Context, userID uint, contributionID string) ([]*pb.PayoutScheduleMessage, error) {
	// Check access
	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", contributionID).Error; err != nil {
		return nil, ErrContributionNotFound
	}

	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, ErrContributionAccessDenied
	}

	var schedules []models.PayoutSchedule
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("contribution_id = ?", contributionID).
		Order("position ASC").
		Find(&schedules).Error; err != nil {
		return nil, err
	}

	protoSchedules := make([]*pb.PayoutScheduleMessage, len(schedules))
	for i, schedule := range schedules {
		protoSchedules[i] = s.payoutScheduleToProto(&schedule)
	}

	return protoSchedules, nil
}

func (s *GroupAccountService) ProcessPayout(ctx context.Context, userID uint, contributionID string) (*pb.PayoutTransactionMessage, error) {
	// Convert contributionID from string to uint
	contribID, err := strconv.ParseUint(contributionID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid contribution ID: %w", err)
	}

	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", contribID).Error; err != nil {
		return nil, ErrContributionNotFound
	}

	// Check access
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if group.AdminID != userID {
		return nil, ErrNotGroupAdmin
	}

	// Verify it's a rotating savings contribution
	if contribution.Type != models.ContributionTypeRotatingSavings {
		return nil, errors.New("payout is only available for rotating savings contributions")
	}

	// Verify there's a current recipient
	if contribution.CurrentPayoutRecipient == nil {
		return nil, errors.New("no current payout recipient set")
	}

	// Check if enough funds
	if contribution.CurrentAmount <= 0 {
		return nil, ErrGroupInsufficientFunds
	}

	var transaction *models.PayoutTransaction

	// Create payout transaction
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		transaction = &models.PayoutTransaction{
			ContributionID:    uint(contribID),
			GroupID:           contribution.GroupID,
			RecipientUserID:   *contribution.CurrentPayoutRecipient,
			Amount:            contribution.CurrentAmount,
			Currency:          contribution.Currency,
			PayoutDate:        time.Now(),
			Status:            models.PayoutTransactionStatusCompleted, // In production, would be pending
		}

		if err := tx.Create(transaction).Error; err != nil {
			return err
		}

		// Update payout schedule status
		if err := tx.Model(&models.PayoutSchedule{}).
			Where("contribution_id = ? AND user_id = ? AND status = ?",
				contribID, *contribution.CurrentPayoutRecipient, models.PayoutStatusPending).
			Updates(map[string]interface{}{
				"status":        models.PayoutStatusCompleted,
				"received_date": time.Now(),
				"actual_amount": contribution.CurrentAmount,
			}).Error; err != nil {
			return err
		}

		// Reset contribution current amount
		if err := tx.Model(&contribution).
			Update("current_amount", 0).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Load recipient info
	var recipient models.User
	if err := s.db.WithContext(ctx).First(&recipient, transaction.RecipientUserID).Error; err != nil {
		return nil, err
	}

	return s.payoutTransactionToProto(transaction, &recipient), nil
}

func (s *GroupAccountService) UpdatePayoutStatus(ctx context.Context, userID uint, req *pb.UpdatePayoutStatusRequest) (*pb.PayoutTransactionMessage, error) {
	var payout models.PayoutTransaction
	if err := s.db.WithContext(ctx).First(&payout, "id = ?", req.PayoutId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPayoutNotFound
		}
		return nil, err
	}

	// Check access - must be group admin
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", payout.GroupID).Error; err != nil {
		return nil, err
	}

	if group.AdminID != userID {
		return nil, ErrNotGroupAdmin
	}

	// Update status
	updates := map[string]interface{}{
		"status": protoToPayoutTransactionStatus(req.Status),
	}

	if req.TransactionId != "" {
		updates["transaction_id"] = req.TransactionId
	}

	if req.FailureReason != "" {
		updates["failure_reason"] = req.FailureReason
	}

	if err := s.db.WithContext(ctx).Model(&payout).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Reload with user info
	var recipient models.User
	if err := s.db.WithContext(ctx).First(&recipient, payout.RecipientUserID).Error; err != nil {
		return nil, err
	}

	return s.payoutTransactionToProto(&payout, &recipient), nil
}

func (s *GroupAccountService) AdvancePayoutRotation(ctx context.Context, userID uint, contributionID string) (*pb.ContributionMessage, error) {
	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", contributionID).Error; err != nil {
		return nil, ErrContributionNotFound
	}

	// Check access
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if group.AdminID != userID {
		return nil, ErrNotGroupAdmin
	}

	// Verify it's a rotating savings contribution
	if contribution.Type != models.ContributionTypeRotatingSavings {
		return nil, errors.New("rotation is only available for rotating savings contributions")
	}

	// Get next pending payout schedule
	var nextSchedule models.PayoutSchedule
	if err := s.db.WithContext(ctx).
		Where("contribution_id = ? AND status = ?", contributionID, models.PayoutStatusPending).
		Order("position ASC").
		First(&nextSchedule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no pending payout schedules found")
		}
		return nil, err
	}

	// Update contribution with next recipient
	nextRecipient := nextSchedule.UserID
	nextDate := nextSchedule.ScheduledDate

	if err := s.db.WithContext(ctx).Model(&contribution).
		Updates(map[string]interface{}{
			"current_payout_recipient": nextRecipient,
			"next_payout_date":         nextDate,
		}).Error; err != nil {
		return nil, err
	}

	// Reload contribution
	if err := s.db.WithContext(ctx).
		Preload("Payments").
		Preload("PayoutSchedules").
		First(&contribution, contribution.ID).Error; err != nil {
		return nil, err
	}

	return s.contributionToProto(&contribution), nil
}

// ============================================================================
// RECEIPT OPERATIONS
// ============================================================================

func (s *GroupAccountService) GenerateReceipt(ctx context.Context, userID uint, paymentID string) (*pb.ContributionReceiptMessage, error) {
	// Convert paymentID from string to uint
	pmtID, err := strconv.ParseUint(paymentID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid payment ID: %w", err)
	}

	var payment models.ContributionPayment
	if err := s.db.WithContext(ctx).Preload("User").First(&payment, "id = ?", pmtID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupPaymentNotFound
		}
		return nil, err
	}

	// Check if user has access
	var contribution models.Contribution
	if err := s.db.WithContext(ctx).First(&contribution, "id = ?", payment.ContributionID).Error; err != nil {
		return nil, err
	}

	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if !s.userHasGroupAccess(userID, &group) && payment.UserID != userID {
		return nil, ErrUnauthorized
	}

	// Check if receipt already exists
	var existingReceipt models.ContributionReceipt
	if err := s.db.WithContext(ctx).Where("payment_id = ?", pmtID).First(&existingReceipt).Error; err == nil {
		return s.receiptToProto(&existingReceipt, &payment.User), nil
	}

	// Generate receipt number
	receiptNumber := fmt.Sprintf("RCP-%s-%d", time.Now().Format("20060102"), payment.ID)

	receipt := &models.ContributionReceipt{
		PaymentID:      uint(pmtID),
		ContributionID: payment.ContributionID,
		GroupID:        payment.GroupID,
		UserID:         payment.UserID,
		Amount:         payment.Amount,
		Currency:       payment.Currency,
		PaymentDate:    payment.PaymentDate,
		GeneratedAt:    time.Now(),
		ReceiptNumber:  receiptNumber,
		ReceiptData: models.JSONB{
			"payment_id":      pmtID,
			"contribution_id": payment.ContributionID,
			"group_id":        payment.GroupID,
			"amount":          payment.Amount,
			"currency":        payment.Currency,
			"payment_date":    payment.PaymentDate.Format(time.RFC3339),
		},
	}

	if err := s.db.WithContext(ctx).Create(receipt).Error; err != nil {
		return nil, err
	}

	return s.receiptToProto(receipt, &payment.User), nil
}

func (s *GroupAccountService) GetUserReceipts(ctx context.Context, userID uint, page, pageSize int) ([]*pb.ContributionReceiptMessage, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	var total int64
	query := s.db.WithContext(ctx).Model(&models.ContributionReceipt{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var receipts []models.ContributionReceipt
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Limit(pageSize).
		Offset(offset).
		Order("generated_at DESC").
		Find(&receipts).Error; err != nil {
		return nil, 0, err
	}

	protoReceipts := make([]*pb.ContributionReceiptMessage, len(receipts))
	for i, receipt := range receipts {
		var user models.User
		s.db.WithContext(ctx).First(&user, userID)
		protoReceipts[i] = s.receiptToProto(&receipt, &user)
	}

	return protoReceipts, total, nil
}

// ============================================================================
// TRANSCRIPT OPERATIONS
// ============================================================================

func (s *GroupAccountService) GenerateTranscript(ctx context.Context, userID uint, contributionID string) (*pb.ContributionTranscriptMessage, error) {
	// Convert contributionID from string to uint
	contribID, err := strconv.ParseUint(contributionID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid contribution ID: %w", err)
	}

	var contribution models.Contribution
	if err := s.db.WithContext(ctx).
		Preload("Payments.User").
		First(&contribution, "id = ?", contribID).Error; err != nil {
		return nil, ErrContributionNotFound
	}

	// Check access
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, ErrContributionAccessDenied
	}

	// Get all payments for this contribution
	var payments []models.ContributionPayment
	if err := s.db.WithContext(ctx).
		Preload("User").
		Where("contribution_id = ?", contribID).
		Order("payment_date ASC").
		Find(&payments).Error; err != nil {
		return nil, err
	}

	// Calculate member contributions
	memberContributions := make(map[string]int64)
	var totalAmount int64

	for _, payment := range payments {
		memberKey := fmt.Sprintf("%d", payment.UserID)
		memberContributions[memberKey] += payment.Amount
		totalAmount += payment.Amount
	}

	// Convert to JSON string
	memberContribData, _ := json.Marshal(memberContributions)

	transcript := &pb.ContributionTranscriptMessage{
		Id:             fmt.Sprintf("TXS-%s-%d", time.Now().Format("20060102"), contribution.ID),
		ContributionId: contributionID,
		GroupId:        strconv.FormatUint(uint64(contribution.GroupID), 10),
		GeneratedAt:    timestamppb.New(time.Now()),
		TotalAmount:    uint64(totalAmount),
		Currency:       contribution.Currency,
		MemberContributions: string(memberContribData),
	}

	for _, payment := range payments {
		transcript.Payments = append(transcript.Payments, s.paymentToProto(&payment, &payment.User))
	}

	return transcript, nil
}

// ============================================================================
// ADVANCED ANALYTICS
// ============================================================================

func (s *GroupAccountService) GetContributionAnalytics(ctx context.Context, userID uint, contributionID string) (*pb.GetContributionAnalyticsResponse, error) {
	// Convert contributionID from string to uint
	contribID, err := strconv.ParseUint(contributionID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid contribution ID: %w", err)
	}

	var contribution models.Contribution
	if err := s.db.WithContext(ctx).
		Preload("Payments").
		Preload("PayoutSchedules").
		First(&contribution, "id = ?", contribID).Error; err != nil {
		return nil, ErrContributionNotFound
	}

	// Check access
	var group models.GroupAccount
	if err := s.db.WithContext(ctx).Preload("Members").First(&group, "id = ?", contribution.GroupID).Error; err != nil {
		return nil, err
	}

	if !s.userHasGroupAccess(userID, &group) {
		return nil, ErrContributionAccessDenied
	}

	// Calculate progress
	progressPercentage := float64(0)
	if contribution.TargetAmount > 0 {
		progressPercentage = float64(contribution.CurrentAmount) / float64(contribution.TargetAmount) * 100
	}

	// Calculate average payment
	totalPayments := int32(len(contribution.Payments))
	averagePayment := float64(0)
	if totalPayments > 0 {
		averagePayment = float64(contribution.CurrentAmount) / float64(totalPayments)
	}

	// Member participation
	participatingMembers := make(map[uint]bool)
	for _, payment := range contribution.Payments {
		participatingMembers[payment.UserID] = true
	}

	participationRate := float64(0)
	if len(group.Members) > 0 {
		participationRate = float64(len(participatingMembers)) / float64(len(group.Members)) * 100
	}

	memberParticipation := &pb.GetContributionAnalyticsResponse_MemberParticipation{
		TotalMembers:         int32(len(group.Members)),
		ParticipatingMembers: int32(len(participatingMembers)),
		ParticipationRate:    participationRate,
	}

	// Schedule info
	schedule := &pb.GetContributionAnalyticsResponse_Schedule{
		IsOnSchedule:       true,
		DaysBehindSchedule: 0,
		CurrentCycle:       0,
	}

	if contribution.CurrentCycle != nil {
		schedule.CurrentCycle = int32(*contribution.CurrentCycle)
	}

	if contribution.NextPaymentDate != nil {
		schedule.NextPaymentDate = timestamppb.New(*contribution.NextPaymentDate)
		if contribution.NextPaymentDate.Before(time.Now()) {
			schedule.IsOnSchedule = false
			schedule.DaysBehindSchedule = int32(time.Since(*contribution.NextPaymentDate).Hours() / 24)
		}
	}

	if contribution.TotalCycles != nil {
		schedule.TotalCycles = int32(*contribution.TotalCycles)
	}

	// Payout info (for rotating savings)
	var payout *pb.GetContributionAnalyticsResponse_Payout
	if contribution.Type == models.ContributionTypeRotatingSavings {
		completedPayouts := 0
		pendingPayouts := 0

		for _, ps := range contribution.PayoutSchedules {
			if ps.Status == models.PayoutStatusCompleted {
				completedPayouts++
			} else if ps.Status == models.PayoutStatusPending {
				pendingPayouts++
			}
		}

		payout = &pb.GetContributionAnalyticsResponse_Payout{
			CompletedPayouts: int32(completedPayouts),
			PendingPayouts:   int32(pendingPayouts),
		}

		if contribution.CurrentPayoutRecipient != nil {
			payout.CurrentRecipient = uint64(*contribution.CurrentPayoutRecipient)
		}

		if contribution.NextPayoutDate != nil {
			payout.NextPayoutDate = timestamppb.New(*contribution.NextPayoutDate)
		}
	}

	// Member stats (payment amounts per member)
	memberStats := make(map[string]int64)
	for _, payment := range contribution.Payments {
		memberKey := fmt.Sprintf("%d", payment.UserID)
		memberStats[memberKey] += payment.Amount
	}

	memberStatsJSON, _ := json.Marshal(memberStats)

	// Convert contribution type to string
	contributionTypeStr := string(contribution.Type)

	return &pb.GetContributionAnalyticsResponse{
		ContributionId:      contributionID,
		Type:                contributionTypeStr,
		ProgressPercentage:  progressPercentage,
		TotalPayments:       totalPayments,
		AveragePayment:      averagePayment,
		CurrentAmount:       uint64(contribution.CurrentAmount),
		TargetAmount:        uint64(contribution.TargetAmount),
		MemberParticipation: memberParticipation,
		Schedule:            schedule,
		Payout:              payout,
		MemberStats:         string(memberStatsJSON),
	}, nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func (s *GroupAccountService) userHasGroupAccess(userID uint, group *models.GroupAccount) bool {
	if group.AdminID == userID {
		return true
	}

	var count int64
	s.db.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", group.ID, userID).
		Count(&count)

	return count > 0
}

func (s *GroupAccountService) createPayoutSchedule(ctx context.Context, contributionID uint, memberOrder []uint64, req *pb.CreateContributionRequest) error {
	startDate := time.Now()
	if req.StartDate != nil {
		startDate = req.StartDate.AsTime()
	}

	_ = req.Frequency // frequency used for future enhancement
	expectedAmount := req.TargetAmount / uint64(len(memberOrder))

	for i, userID := range memberOrder {
		schedule := &models.PayoutSchedule{
			ContributionID: contributionID,
			UserID:         uint(userID),
			Position:       i + 1,
			ScheduledDate:  startDate.AddDate(0, i, 0), // Monthly by default
			ExpectedAmount: int64(expectedAmount * uint64(len(memberOrder))),
			Status:         models.PayoutStatusPending,
		}

		if err := s.db.WithContext(ctx).Create(schedule).Error; err != nil {
			return err
		}
	}

	// Set first payout recipient
	if len(memberOrder) > 0 {
		firstRecipient := uint(memberOrder[0])
		nextPayoutDate := startDate
		s.db.WithContext(ctx).Model(&models.Contribution{}).
			Where("id = ?", contributionID).
			Updates(map[string]interface{}{
				"current_payout_recipient": firstRecipient,
				"next_payout_date":         nextPayoutDate,
			})
	}

	return nil
}

// ============================================================================
// PROTO CONVERSION HELPERS
// ============================================================================

func (s *GroupAccountService) groupToProto(group *models.GroupAccount) *pb.GroupAccountMessage {
	proto := &pb.GroupAccountMessage{
		Id:          strconv.FormatUint(uint64(group.ID), 10),
		Name:        group.Name,
		Description: group.Description,
		AdminId:     uint64(group.AdminID),
		Status:      groupStatusToProto(group.Status),
		CreatedAt:   timestamppb.New(group.CreatedAt),
		UpdatedAt:   timestamppb.New(group.UpdatedAt),
	}

	if group.Metadata != nil {
		if data, err := json.Marshal(group.Metadata); err == nil {
			proto.Metadata = string(data)
		}
	}

	for _, member := range group.Members {
		proto.Members = append(proto.Members, s.memberToProto(&member))
	}

	for _, contrib := range group.Contributions {
		proto.Contributions = append(proto.Contributions, s.contributionToProto(&contrib))
	}

	return proto
}

func (s *GroupAccountService) memberToProto(member *models.GroupMember) *pb.GroupMemberMessage {
	proto := &pb.GroupMemberMessage{
		Id:       strconv.FormatUint(uint64(member.ID), 10),
		GroupId:  strconv.FormatUint(uint64(member.GroupID), 10),
		UserId:   uint64(member.UserID),
		Role:     memberRoleToProto(member.Role),
		Status:   memberStatusToProto(member.Status),
		JoinedAt: timestamppb.New(member.CreatedAt),
	}

	if member.User.ID != 0 {
		proto.IsPartial = member.User.IsPartial
		proto.PhoneNumber = member.User.PhoneNumber
		proto.Email = member.User.Email

		if member.User.IsPartial {
			// For partial users, show email or phone as the name
			if member.User.Email != "" {
				proto.UserName = member.User.Email + " (Invited)"
			} else {
				proto.UserName = member.User.PhoneNumber + " (Invited)"
			}
		} else {
			proto.UserName = member.User.FirstName + " " + member.User.LastName
			if member.User.Username != nil && *member.User.Username != "" {
				proto.UserUsername = *member.User.Username
			}
		}
		// Profile image URL will be added when available from User model
	}

	if member.Permissions != nil {
		if data, err := json.Marshal(member.Permissions); err == nil {
			proto.Permissions = string(data)
		}
	}

	return proto
}

func (s *GroupAccountService) contributionToProto(contrib *models.Contribution) *pb.ContributionMessage {
	proto := &pb.ContributionMessage{
		Id:             strconv.FormatUint(uint64(contrib.ID), 10),
		GroupId:        strconv.FormatUint(uint64(contrib.GroupID), 10),
		Title:          contrib.Title,
		Description:    contrib.Description,
		TargetAmount:   uint64(contrib.TargetAmount),
		CurrentAmount:  uint64(contrib.CurrentAmount),
		Currency:       contrib.Currency,
		Deadline:       timestamppb.New(contrib.Deadline),
		Status:         contributionStatusToProto(contrib.Status),
		CreatedBy:      uint64(contrib.CreatedBy),
		CreatedAt:      timestamppb.New(contrib.CreatedAt),
		UpdatedAt:      timestamppb.New(contrib.UpdatedAt),
		Type:           contributionTypeToProto(contrib.Type),
		AutoPayEnabled: contrib.AutoPayEnabled,
		AllowPartialPayments: contrib.AllowPartialPayments,
	}

	if contrib.Frequency != nil {
		proto.Frequency = contributionFrequencyToProto(*contrib.Frequency)
	}
	if contrib.RegularAmount != nil {
		proto.RegularAmount = uint64(*contrib.RegularAmount)
	}
	if contrib.NextPaymentDate != nil {
		proto.NextPaymentDate = timestamppb.New(*contrib.NextPaymentDate)
	}
	if contrib.StartDate != nil {
		proto.StartDate = timestamppb.New(*contrib.StartDate)
	}
	if contrib.TotalCycles != nil {
		proto.TotalCycles = int32(*contrib.TotalCycles)
	}
	if contrib.CurrentCycle != nil {
		proto.CurrentCycle = int32(*contrib.CurrentCycle)
	}
	if contrib.CurrentPayoutRecipient != nil {
		proto.CurrentPayoutRecipient = uint64(*contrib.CurrentPayoutRecipient)
	}
	if contrib.NextPayoutDate != nil {
		proto.NextPayoutDate = timestamppb.New(*contrib.NextPayoutDate)
	}
	if contrib.PenaltyAmount != nil {
		proto.PenaltyAmount = uint64(*contrib.PenaltyAmount)
	}
	if contrib.GracePeriodDays != nil {
		proto.GracePeriodDays = int32(*contrib.GracePeriodDays)
	}
	if contrib.MinimumBalance != nil {
		proto.MinimumBalance = uint64(*contrib.MinimumBalance)
	}

	for _, payment := range contrib.Payments {
		proto.Payments = append(proto.Payments, s.paymentToProto(&payment, nil))
	}

	for _, schedule := range contrib.PayoutSchedules {
		proto.PayoutSchedule = append(proto.PayoutSchedule, s.payoutScheduleToProto(&schedule))
	}

	return proto
}

func (s *GroupAccountService) paymentToProto(payment *models.ContributionPayment, user *models.User) *pb.ContributionPaymentMessage {
	proto := &pb.ContributionPaymentMessage{
		Id:             strconv.FormatUint(uint64(payment.ID), 10),
		ContributionId: strconv.FormatUint(uint64(payment.ContributionID), 10),
		GroupId:        strconv.FormatUint(uint64(payment.GroupID), 10),
		UserId:         uint64(payment.UserID),
		Amount:         uint64(payment.Amount),
		Currency:       payment.Currency,
		PaymentDate:    timestamppb.New(payment.PaymentDate),
		Status:         paymentStatusToProto(payment.Status),
	}

	if user != nil {
		proto.UserName = user.FirstName + " " + user.LastName
	}
	if payment.TransactionID != nil {
		proto.TransactionId = *payment.TransactionID
	}
	if payment.ReceiptID != nil {
		proto.ReceiptId = *payment.ReceiptID
	}
	if payment.Notes != nil {
		proto.Notes = *payment.Notes
	}

	return proto
}

func (s *GroupAccountService) payoutScheduleToProto(schedule *models.PayoutSchedule) *pb.PayoutScheduleMessage {
	proto := &pb.PayoutScheduleMessage{
		Id:             strconv.FormatUint(uint64(schedule.ID), 10),
		UserId:         uint64(schedule.UserID),
		Position:       int32(schedule.Position),
		ScheduledDate:  timestamppb.New(schedule.ScheduledDate),
		ExpectedAmount: uint64(schedule.ExpectedAmount),
		Status:         payoutStatusToProto(schedule.Status),
	}

	if schedule.User.ID != 0 {
		proto.UserName = schedule.User.FirstName + " " + schedule.User.LastName
	}
	if schedule.ReceivedDate != nil {
		proto.ReceivedDate = timestamppb.New(*schedule.ReceivedDate)
	}
	if schedule.ActualAmount != nil {
		proto.ActualAmount = uint64(*schedule.ActualAmount)
	}
	if schedule.Notes != nil {
		proto.Notes = *schedule.Notes
	}

	return proto
}

// Enum conversion helpers
func groupStatusToProto(status models.GroupAccountStatus) pb.GroupAccountStatus {
	switch status {
	case models.GroupAccountStatusActive:
		return pb.GroupAccountStatus_GROUP_ACCOUNT_STATUS_ACTIVE
	case models.GroupAccountStatusSuspended:
		return pb.GroupAccountStatus_GROUP_ACCOUNT_STATUS_SUSPENDED
	case models.GroupAccountStatusClosed:
		return pb.GroupAccountStatus_GROUP_ACCOUNT_STATUS_CLOSED
	default:
		return pb.GroupAccountStatus_GROUP_ACCOUNT_STATUS_UNSPECIFIED
	}
}

func protoToGroupStatus(status pb.GroupAccountStatus) models.GroupAccountStatus {
	switch status {
	case pb.GroupAccountStatus_GROUP_ACCOUNT_STATUS_ACTIVE:
		return models.GroupAccountStatusActive
	case pb.GroupAccountStatus_GROUP_ACCOUNT_STATUS_SUSPENDED:
		return models.GroupAccountStatusSuspended
	case pb.GroupAccountStatus_GROUP_ACCOUNT_STATUS_CLOSED:
		return models.GroupAccountStatusClosed
	default:
		return models.GroupAccountStatusActive
	}
}

func memberRoleToProto(role models.GroupMemberRole) pb.GroupMemberRole {
	switch role {
	case models.GroupMemberRoleAdmin:
		return pb.GroupMemberRole_GROUP_MEMBER_ROLE_ADMIN
	case models.GroupMemberRoleMember:
		return pb.GroupMemberRole_GROUP_MEMBER_ROLE_MEMBER
	case models.GroupMemberRoleViewer:
		return pb.GroupMemberRole_GROUP_MEMBER_ROLE_VIEWER
	default:
		return pb.GroupMemberRole_GROUP_MEMBER_ROLE_UNSPECIFIED
	}
}

func protoToMemberRole(role pb.GroupMemberRole) models.GroupMemberRole {
	switch role {
	case pb.GroupMemberRole_GROUP_MEMBER_ROLE_ADMIN:
		return models.GroupMemberRoleAdmin
	case pb.GroupMemberRole_GROUP_MEMBER_ROLE_MEMBER:
		return models.GroupMemberRoleMember
	case pb.GroupMemberRole_GROUP_MEMBER_ROLE_VIEWER:
		return models.GroupMemberRoleViewer
	default:
		return models.GroupMemberRoleMember
	}
}

func memberStatusToProto(status models.GroupMemberStatus) pb.GroupMemberStatus {
	switch status {
	case models.GroupMemberStatusActive:
		return pb.GroupMemberStatus_GROUP_MEMBER_STATUS_ACTIVE
	case models.GroupMemberStatusInactive:
		return pb.GroupMemberStatus_GROUP_MEMBER_STATUS_INACTIVE
	case models.GroupMemberStatusSuspended:
		return pb.GroupMemberStatus_GROUP_MEMBER_STATUS_SUSPENDED
	case models.GroupMemberStatusRemoved:
		return pb.GroupMemberStatus_GROUP_MEMBER_STATUS_REMOVED
	default:
		return pb.GroupMemberStatus_GROUP_MEMBER_STATUS_UNSPECIFIED
	}
}

func contributionTypeToProto(t models.ContributionType) pb.ContributionType {
	switch t {
	case models.ContributionTypeOneTime:
		return pb.ContributionType_CONTRIBUTION_TYPE_ONE_TIME
	case models.ContributionTypeRecurring:
		return pb.ContributionType_CONTRIBUTION_TYPE_RECURRING
	case models.ContributionTypeRotatingSavings:
		return pb.ContributionType_CONTRIBUTION_TYPE_ROTATING_SAVINGS
	default:
		return pb.ContributionType_CONTRIBUTION_TYPE_UNSPECIFIED
	}
}

func protoToContributionType(t pb.ContributionType) models.ContributionType {
	switch t {
	case pb.ContributionType_CONTRIBUTION_TYPE_ONE_TIME:
		return models.ContributionTypeOneTime
	case pb.ContributionType_CONTRIBUTION_TYPE_RECURRING:
		return models.ContributionTypeRecurring
	case pb.ContributionType_CONTRIBUTION_TYPE_ROTATING_SAVINGS:
		return models.ContributionTypeRotatingSavings
	default:
		return models.ContributionTypeOneTime
	}
}

func contributionFrequencyToProto(f models.ContributionFrequency) pb.ContributionFrequency {
	switch f {
	case models.ContributionFrequencyDaily:
		return pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_DAILY
	case models.ContributionFrequencyWeekly:
		return pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_WEEKLY
	case models.ContributionFrequencyBiweekly:
		return pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_BIWEEKLY
	case models.ContributionFrequencyMonthly:
		return pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_MONTHLY
	case models.ContributionFrequencyQuarterly:
		return pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_QUARTERLY
	case models.ContributionFrequencyYearly:
		return pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_YEARLY
	default:
		return pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_UNSPECIFIED
	}
}

func protoToContributionFrequency(f pb.ContributionFrequency) models.ContributionFrequency {
	switch f {
	case pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_DAILY:
		return models.ContributionFrequencyDaily
	case pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_WEEKLY:
		return models.ContributionFrequencyWeekly
	case pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_BIWEEKLY:
		return models.ContributionFrequencyBiweekly
	case pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_MONTHLY:
		return models.ContributionFrequencyMonthly
	case pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_QUARTERLY:
		return models.ContributionFrequencyQuarterly
	case pb.ContributionFrequency_CONTRIBUTION_FREQUENCY_YEARLY:
		return models.ContributionFrequencyYearly
	default:
		return models.ContributionFrequencyMonthly
	}
}

func contributionStatusToProto(s models.ContributionStatus) pb.ContributionStatus {
	switch s {
	case models.ContributionStatusActive:
		return pb.ContributionStatus_CONTRIBUTION_STATUS_ACTIVE
	case models.ContributionStatusPaused:
		return pb.ContributionStatus_CONTRIBUTION_STATUS_PAUSED
	case models.ContributionStatusCompleted:
		return pb.ContributionStatus_CONTRIBUTION_STATUS_COMPLETED
	case models.ContributionStatusCancelled:
		return pb.ContributionStatus_CONTRIBUTION_STATUS_CANCELLED
	default:
		return pb.ContributionStatus_CONTRIBUTION_STATUS_UNSPECIFIED
	}
}

func protoToContributionStatus(s pb.ContributionStatus) models.ContributionStatus {
	switch s {
	case pb.ContributionStatus_CONTRIBUTION_STATUS_ACTIVE:
		return models.ContributionStatusActive
	case pb.ContributionStatus_CONTRIBUTION_STATUS_PAUSED:
		return models.ContributionStatusPaused
	case pb.ContributionStatus_CONTRIBUTION_STATUS_COMPLETED:
		return models.ContributionStatusCompleted
	case pb.ContributionStatus_CONTRIBUTION_STATUS_CANCELLED:
		return models.ContributionStatusCancelled
	default:
		return models.ContributionStatusActive
	}
}

func paymentStatusToProto(s models.PaymentStatus) pb.PaymentStatus {
	switch s {
	case models.PaymentStatusPending:
		return pb.PaymentStatus_PAYMENT_STATUS_PENDING
	case models.PaymentStatusProcessing:
		return pb.PaymentStatus_PAYMENT_STATUS_PROCESSING
	case models.PaymentStatusCompleted:
		return pb.PaymentStatus_PAYMENT_STATUS_COMPLETED
	case models.PaymentStatusFailed:
		return pb.PaymentStatus_PAYMENT_STATUS_FAILED
	case models.PaymentStatusRefunded:
		return pb.PaymentStatus_PAYMENT_STATUS_REFUNDED
	default:
		return pb.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	}
}

func payoutStatusToProto(s models.PayoutStatus) pb.PayoutStatus {
	switch s {
	case models.PayoutStatusPending:
		return pb.PayoutStatus_PAYOUT_STATUS_PENDING
	case models.PayoutStatusCompleted:
		return pb.PayoutStatus_PAYOUT_STATUS_COMPLETED
	case models.PayoutStatusCancelled:
		return pb.PayoutStatus_PAYOUT_STATUS_CANCELLED
	default:
		return pb.PayoutStatus_PAYOUT_STATUS_UNSPECIFIED
	}
}

func protoToPaymentStatus(s pb.PaymentStatus) models.PaymentStatus {
	switch s {
	case pb.PaymentStatus_PAYMENT_STATUS_PENDING:
		return models.PaymentStatusPending
	case pb.PaymentStatus_PAYMENT_STATUS_PROCESSING:
		return models.PaymentStatusProcessing
	case pb.PaymentStatus_PAYMENT_STATUS_COMPLETED:
		return models.PaymentStatusCompleted
	case pb.PaymentStatus_PAYMENT_STATUS_FAILED:
		return models.PaymentStatusFailed
	case pb.PaymentStatus_PAYMENT_STATUS_REFUNDED:
		return models.PaymentStatusRefunded
	default:
		return models.PaymentStatusPending
	}
}

func payoutTransactionStatusToProto(s models.PayoutTransactionStatus) pb.PayoutTransactionStatus {
	switch s {
	case models.PayoutTransactionStatusPending:
		return pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_PENDING
	case models.PayoutTransactionStatusProcessing:
		return pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_PROCESSING
	case models.PayoutTransactionStatusCompleted:
		return pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_COMPLETED
	case models.PayoutTransactionStatusFailed:
		return pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_FAILED
	case models.PayoutTransactionStatusRefunded:
		return pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_REFUNDED
	default:
		return pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_UNSPECIFIED
	}
}

func protoToPayoutTransactionStatus(s pb.PayoutTransactionStatus) models.PayoutTransactionStatus {
	switch s {
	case pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_PENDING:
		return models.PayoutTransactionStatusPending
	case pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_PROCESSING:
		return models.PayoutTransactionStatusProcessing
	case pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_COMPLETED:
		return models.PayoutTransactionStatusCompleted
	case pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_FAILED:
		return models.PayoutTransactionStatusFailed
	case pb.PayoutTransactionStatus_PAYOUT_TRANSACTION_STATUS_REFUNDED:
		return models.PayoutTransactionStatusRefunded
	default:
		return models.PayoutTransactionStatusPending
	}
}

func (s *GroupAccountService) calculateNextPaymentDate(currentDate time.Time, frequency models.ContributionFrequency) time.Time {
	switch frequency {
	case models.ContributionFrequencyDaily:
		return currentDate.AddDate(0, 0, 1)
	case models.ContributionFrequencyWeekly:
		return currentDate.AddDate(0, 0, 7)
	case models.ContributionFrequencyBiweekly:
		return currentDate.AddDate(0, 0, 14)
	case models.ContributionFrequencyMonthly:
		return currentDate.AddDate(0, 1, 0)
	case models.ContributionFrequencyQuarterly:
		return currentDate.AddDate(0, 3, 0)
	case models.ContributionFrequencyYearly:
		return currentDate.AddDate(1, 0, 0)
	default:
		return currentDate.AddDate(0, 1, 0) // Default to monthly
	}
}

func (s *GroupAccountService) payoutTransactionToProto(payout *models.PayoutTransaction, recipient *models.User) *pb.PayoutTransactionMessage {
	proto := &pb.PayoutTransactionMessage{
		Id:                strconv.FormatUint(uint64(payout.ID), 10),
		ContributionId:    strconv.FormatUint(uint64(payout.ContributionID), 10),
		GroupId:           strconv.FormatUint(uint64(payout.GroupID), 10),
		RecipientUserId:   uint64(payout.RecipientUserID),
		Amount:            uint64(payout.Amount),
		Currency:          payout.Currency,
		PayoutDate:        timestamppb.New(payout.PayoutDate),
		Status:            payoutTransactionStatusToProto(payout.Status),
	}

	if recipient != nil {
		proto.RecipientUserName = recipient.FirstName + " " + recipient.LastName
	}

	if payout.TransactionID != nil {
		proto.TransactionId = *payout.TransactionID
	}

	if payout.PaymentMethod != nil {
		proto.PaymentMethod = *payout.PaymentMethod
	}

	if payout.FailureReason != nil {
		proto.FailureReason = *payout.FailureReason
	}

	if payout.Metadata != nil {
		if data, err := json.Marshal(payout.Metadata); err == nil {
			proto.Metadata = string(data)
		}
	}

	return proto
}

func (s *GroupAccountService) receiptToProto(receipt *models.ContributionReceipt, user *models.User) *pb.ContributionReceiptMessage {
	proto := &pb.ContributionReceiptMessage{
		Id:             strconv.FormatUint(uint64(receipt.ID), 10),
		PaymentId:      strconv.FormatUint(uint64(receipt.PaymentID), 10),
		ContributionId: strconv.FormatUint(uint64(receipt.ContributionID), 10),
		GroupId:        strconv.FormatUint(uint64(receipt.GroupID), 10),
		UserId:         uint64(receipt.UserID),
		Amount:         uint64(receipt.Amount),
		Currency:       receipt.Currency,
		PaymentDate:    timestamppb.New(receipt.PaymentDate),
		GeneratedAt:    timestamppb.New(receipt.GeneratedAt),
		ReceiptNumber:  receipt.ReceiptNumber,
	}

	if user != nil {
		proto.UserName = user.FirstName + " " + user.LastName
	}

	if receipt.ReceiptData != nil {
		if data, err := json.Marshal(receipt.ReceiptData); err == nil {
			proto.ReceiptData = string(data)
		}
	}

	return proto
}

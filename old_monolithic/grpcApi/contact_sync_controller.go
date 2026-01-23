package grpcApi

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ContactSyncController handles gRPC requests for contact syncing
type ContactSyncController struct {
	pb.UnimplementedContactSyncServiceServer
	contactSyncService services.IContactSyncService
	userService        services.IUserService
}

// NewContactSyncController creates a new ContactSyncController
func NewContactSyncController(contactSyncService services.IContactSyncService, userService services.IUserService) *ContactSyncController {
	return &ContactSyncController{
		contactSyncService: contactSyncService,
		userService:        userService,
	}
}

// SyncContacts handles the gRPC request to sync device contacts
func (c *ContactSyncController) SyncContacts(ctx context.Context, req *pb.SyncContactsRequest) (*pb.SyncContactsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if len(req.GetContacts()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one contact is required")
	}

	result, err := c.contactSyncService.SyncContacts(ctx, user.ID, req.GetContacts(), req.GetReplaceAll())
	if err != nil {
		if errors.Is(err, services.ErrContactSyncFailed) {
			return nil, status.Errorf(codes.Internal, "failed to sync contacts: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "an unexpected error occurred: %v", err)
	}

	// Convert synced contacts to proto
	protoContacts := make([]*pb.SyncedContact, 0, len(result.SyncedContacts))
	for _, contact := range result.SyncedContacts {
		protoContacts = append(protoContacts, services.ConvertSyncedContactToProto(contact))
	}

	// Convert matched users to proto
	protoMatches := make([]*pb.LazerVaultUserMatch, 0, len(result.MatchedUsers))
	for _, match := range result.MatchedUsers {
		protoMatches = append(protoMatches, &pb.LazerVaultUserMatch{
			ContactId:       match.ContactID,
			UserId:          fmt.Sprint(match.UserID),
			Username:        match.Username,
			Name:            match.Name,
			ProfilePhotoUrl: match.ProfilePhotoURL,
			IsVerified:      match.IsVerified,
			MatchedBy:       match.MatchedBy,
		})
	}

	return &pb.SyncContactsResponse{
		SyncedContacts:    protoContacts,
		TotalSynced:       int32(result.TotalSynced),
		TotalMatchedUsers: int32(result.TotalMatchedUsers),
		MatchedUsers:      protoMatches,
	}, nil
}

// GetSyncedContacts handles the gRPC request to retrieve synced contacts
func (c *ContactSyncController) GetSyncedContacts(ctx context.Context, req *pb.GetSyncedContactsRequest) (*pb.GetSyncedContactsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	page := req.GetPage()
	pageSize := req.GetPageSize()
	if pageSize <= 0 {
		pageSize = 50 // Default page size
	}
	if pageSize > 100 {
		pageSize = 100 // Max page size
	}

	result, err := c.contactSyncService.GetSyncedContacts(
		ctx,
		user.ID,
		page,
		pageSize,
		req.GetSearchQuery(),
		req.GetOnlyLazervaultUsers(),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get synced contacts: %v", err)
	}

	// Convert to proto
	protoContacts := make([]*pb.SyncedContact, 0, len(result.Contacts))
	for _, contact := range result.Contacts {
		protoContacts = append(protoContacts, services.ConvertSyncedContactToProto(contact))
	}

	return &pb.GetSyncedContactsResponse{
		Contacts:   protoContacts,
		TotalCount: int32(result.TotalCount),
		Page:       result.Page,
		PageSize:   result.PageSize,
	}, nil
}

// DeleteSyncedContacts handles the gRPC request to delete synced contacts
func (c *ContactSyncController) DeleteSyncedContacts(ctx context.Context, req *pb.DeleteSyncedContactsRequest) (*pb.DeleteSyncedContactsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if !req.GetDeleteAll() && len(req.GetContactIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "must provide contact IDs or set delete_all to true")
	}

	deletedCount, err := c.contactSyncService.DeleteSyncedContacts(
		ctx,
		user.ID,
		req.GetContactIds(),
		req.GetDeleteAll(),
	)
	if err != nil {
		if errors.Is(err, services.ErrContactDeleteFailed) {
			return nil, status.Errorf(codes.Internal, "failed to delete contacts: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "an unexpected error occurred: %v", err)
	}

	return &pb.DeleteSyncedContactsResponse{
		DeletedCount: int32(deletedCount),
		Success:      true,
	}, nil
}

// ConvertContactToRecipient handles the gRPC request to convert a contact to a recipient
func (c *ContactSyncController) ConvertContactToRecipient(ctx context.Context, req *pb.ConvertContactToRecipientRequest) (*pb.ConvertContactToRecipientResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// Validate that either phone or email is provided
	if req.GetPhoneNumber() == "" && req.GetEmail() == "" {
		return nil, status.Error(codes.InvalidArgument, "either phone number or email is required")
	}

	result, err := c.contactSyncService.ConvertContactToRecipient(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrRecipientConversionFailed) {
			return nil, status.Errorf(codes.Internal, "failed to convert contact: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "an unexpected error occurred: %v", err)
	}

	response := &pb.ConvertContactToRecipientResponse{
		RecipientId:      fmt.Sprint(result.RecipientID),
		IsLazervaultUser: result.IsLazerVaultUser,
	}

	if result.LazerVaultUserID != nil {
		response.LazervaultUserId = fmt.Sprint(*result.LazerVaultUserID)
	}

	if result.Recipient != nil {
		response.Recipient = &pb.RecipientDetails{
			Id:            fmt.Sprint(result.Recipient.ID),
			Name:          result.Recipient.Name,
			AccountNumber: result.Recipient.AccountNumber,
			BankName:      result.Recipient.BankName,
			SortCode:      result.Recipient.SortCode,
			IsFavorite:    result.Recipient.IsFavorite,
		}
	}

	response.LazervaultUsername = result.LazerVaultUsername

	return response, nil
}

// FindLazerVaultUsers handles the gRPC request to find LazerVault users from contacts
func (c *ContactSyncController) FindLazerVaultUsers(ctx context.Context, req *pb.FindLazerVaultUsersRequest) (*pb.FindLazerVaultUsersResponse, error) {
	// Note: This endpoint doesn't require authentication as it's used for discovery
	// However, you may want to add rate limiting or other protections

	if len(req.GetPhoneNumbers()) == 0 && len(req.GetEmails()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one phone number or email is required")
	}

	matches, err := c.contactSyncService.FindLazerVaultUsers(
		ctx,
		req.GetPhoneNumbers(),
		req.GetEmails(),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to find LazerVault users: %v", err)
	}

	// Convert to proto
	protoMatches := make([]*pb.LazerVaultUserMatch, 0, len(matches))
	for _, match := range matches {
		protoMatches = append(protoMatches, &pb.LazerVaultUserMatch{
			ContactId:       match.ContactID,
			UserId:          fmt.Sprint(match.UserID),
			Username:        match.Username,
			Name:            match.Name,
			ProfilePhotoUrl: match.ProfilePhotoURL,
			IsVerified:      match.IsVerified,
			MatchedBy:       match.MatchedBy,
		})
	}

	return &pb.FindLazerVaultUsersResponse{
		MatchedUsers: protoMatches,
		TotalMatches: int32(len(protoMatches)),
	}, nil
}

// UpdateSyncPreferences handles the gRPC request to update sync preferences
func (c *ContactSyncController) UpdateSyncPreferences(ctx context.Context, req *pb.UpdateSyncPreferencesRequest) (*pb.UpdateSyncPreferencesResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	preferences, err := c.contactSyncService.UpdateSyncPreferences(ctx, user.ID, req)
	if err != nil {
		if errors.Is(err, services.ErrPreferencesUpdateFailed) {
			return nil, status.Errorf(codes.Internal, "failed to update preferences: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "an unexpected error occurred: %v", err)
	}

	return &pb.UpdateSyncPreferencesResponse{
		Preferences: services.ConvertSyncPreferencesToProto(preferences),
		Success:     true,
	}, nil
}

// Helper function to get user ID from string
func parseUserID(userIDStr string) (uint, error) {
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid user ID: %w", err)
	}
	return uint(userID), nil
}

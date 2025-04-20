package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RecipientController handles gRPC requests for the RecipientService.
type RecipientController struct {
	pb.UnimplementedRecipientServiceServer
	recipientService services.IRecipientService
	userService      services.IUserService
}

// NewRecipientController creates a new RecipientController.
func NewRecipientController(recipientService services.IRecipientService, userService services.IUserService) *RecipientController {
	return &RecipientController{
		recipientService: recipientService,
		userService:      userService,
	}
}

// Helper function to get User ID (uint) from email in token payload
func (c *RecipientController) getUserIDFromContext(ctx context.Context) (uint, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return 0, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}

	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			log.Printf("User not found for email in token: %s", authPayload.Email)
			return 0, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		log.Printf("Error finding user by email via service: %v", err)
		return 0, status.Errorf(codes.Internal, "failed to verify user identity")
	}

	return user.ID, nil
}

// convertRecipient converts a models.Recipient to a pb.Recipient.
func convertRecipient(recipient *models.Recipient) *pb.Recipient {
	if recipient == nil {
		return nil
	}
	return &pb.Recipient{
		Id:            uint64(recipient.ID),
		Name:          recipient.Name,
		AccountNumber: recipient.AccountNumber,
		SortCode:      recipient.SortCode,
		BankName:      recipient.BankName,
		IsFavorite:    recipient.IsFavorite,
		CreatedAt:     timestamppb.New(recipient.CreatedAt),
		UpdatedAt:     timestamppb.New(recipient.UpdatedAt),
	}
}

// AddRecipient handles the RPC for adding a new recipient.
func (c *RecipientController) AddRecipient(ctx context.Context, req *pb.AddRecipientRequest) (*pb.AddRecipientResponse, error) {
	// 1. Get Owner User ID (uint) using the helper
	ownerUserID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err // Error already formatted by helper
	}

	// 2. Validate Request (Basic)
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
	if req.GetAccountNumber() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "account_number is required")
	}
	if req.GetBankName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bank_name is required")
	}

	// 3. Prepare Service Request
	serviceReq := services.AddRecipientRequest{
		OwnerUserID:   ownerUserID,
		Name:          req.GetName(),
		AccountNumber: req.GetAccountNumber(),
		SortCode:      req.GetSortCode(), // Optional
		BankName:      req.GetBankName(),
		IsFavorite:    req.GetIsFavorite(), // Pass favorite status
	}

	// 4. Call Service
	newRecipient, err := c.recipientService.AddRecipient(ctx, serviceReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to add recipient: %v", err)
	}

	// 5. Convert and Return Response
	resp := &pb.AddRecipientResponse{
		Recipient: convertRecipient(newRecipient),
	}
	return resp, nil
}

// GetRecipients handles retrieving recipients, potentially filtered by favorite status.
func (c *RecipientController) GetRecipients(ctx context.Context, req *pb.GetRecipientsRequest) (*pb.GetRecipientsResponse, error) {
	// 1. Get Owner User ID (uint) using the helper
	ownerUserID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err // Error already formatted by helper
	}

	// 2. Call Service, passing the filter flag
	recipients, err := c.recipientService.GetRecipients(ctx, ownerUserID, req.GetOnlyFavorites())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve recipients: %v", err)
	}

	// 3. Convert and Return Response
	pbRecipients := make([]*pb.Recipient, 0, len(recipients))
	for i := range recipients {
		pbRecipients = append(pbRecipients, convertRecipient(&recipients[i]))
	}

	resp := &pb.GetRecipientsResponse{
		Recipients: pbRecipients,
	}
	return resp, nil
}

// UpdateRecipientFavoriteStatus handles updating the favorite status of a recipient.
func (c *RecipientController) UpdateRecipientFavoriteStatus(ctx context.Context, req *pb.UpdateRecipientFavoriteStatusRequest) (*pb.UpdateRecipientFavoriteStatusResponse, error) {
	// 1. Get Owner User ID (uint) using the helper
	ownerUserID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err // Error already formatted by helper
	}

	// 2. Validate Request
	if req.GetId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "recipient ID is required")
	}
	// No validation needed for is_favorite boolean

	// 3. Prepare Service Request
	serviceReq := services.UpdateRecipientFavoriteStatusRequest{
		OwnerUserID: ownerUserID,
		RecipientID: uint(req.GetId()),
		IsFavorite:  req.GetIsFavorite(),
	}

	// 4. Call Service
	updatedRecipient, err := c.recipientService.UpdateRecipientFavoriteStatus(ctx, serviceReq)
	if err != nil {
		if errors.Is(err, services.ErrRecipientNotFound) {
			return nil, status.Errorf(codes.NotFound, "recipient not found or permission denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to update recipient: %v", err)
	}

	// 5. Convert and Return Response
	resp := &pb.UpdateRecipientFavoriteStatusResponse{
		Recipient: convertRecipient(updatedRecipient),
	}
	return resp, nil
}

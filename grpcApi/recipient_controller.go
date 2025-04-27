package grpcApi

import (
	"context"
	"errors" // Needed for getUserFromContext dependency
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RecipientController handles gRPC requests for recipient management.
type RecipientController struct {
	pb.UnimplementedRecipientServiceServer
	recipientService services.IRecipientService
	userService      services.IUserService // Needed for getUserFromContext
}

// NewRecipientController creates a new RecipientController.
func NewRecipientController(recipientService services.IRecipientService, userService services.IUserService) *RecipientController {
	return &RecipientController{
		recipientService: recipientService,
		userService:      userService,
	}
}

// CreateRecipient handles the gRPC request to create a new recipient.
func (c *RecipientController) CreateRecipient(ctx context.Context, req *pb.CreateRecipientRequest) (*pb.CreateRecipientResponse, error) {
	user, err := getUserFromContext(ctx, c.userService) // Use shared helper
	if err != nil {
		return nil, err
	}

	// Basic validation (service might do more)
	if req.GetName() == "" || req.GetAccountNumber() == "" || req.GetBankName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name, account_number, and bank_name are required")
	}

	createdModel, err := c.recipientService.CreateRecipient(ctx, user.ID, req)
	if err != nil {
		// Map service errors
		if errors.Is(err, services.ErrRecipientCreateFailed) {
			return nil, status.Errorf(codes.Internal, "failed to create recipient: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "an unexpected error occurred: %v", err)
	}

	return &pb.CreateRecipientResponse{
		Recipient: services.ConvertRecipientToProto(createdModel), // Use helper
	}, nil
}

// ListRecipients handles the gRPC request to list recipients.
func (c *RecipientController) ListRecipients(ctx context.Context, req *pb.ListRecipientsRequest) (*pb.ListRecipientsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	recipientModels, err := c.recipientService.ListRecipients(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list recipients: %v", err)
	}

	// Convert models to protos
	protoRecipients := make([]*pb.Recipient, 0, len(recipientModels))
	for _, model := range recipientModels {
		protoRecipients = append(protoRecipients, services.ConvertRecipientToProto(model))
	}

	return &pb.ListRecipientsResponse{
		Recipients: protoRecipients,
	}, nil
}

// UpdateRecipient handles the gRPC request to update a recipient.
func (c *RecipientController) UpdateRecipient(ctx context.Context, req *pb.UpdateRecipientRequest) (*pb.UpdateRecipientResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.GetRecipientId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "recipient_id is required")
	}

	updatedModel, err := c.recipientService.UpdateRecipient(ctx, user.ID, req)
	if err != nil {
		// Map service errors
		if errors.Is(err, services.ErrRecipientNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		} else if errors.Is(err, services.ErrRecipientUpdateFailed) {
			return nil, status.Errorf(codes.Internal, "failed to update recipient: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "an unexpected error occurred: %v", err)
	}

	return &pb.UpdateRecipientResponse{
		Recipient: services.ConvertRecipientToProto(updatedModel),
	}, nil
}

// DeleteRecipient handles the gRPC request to delete a recipient.
func (c *RecipientController) DeleteRecipient(ctx context.Context, req *pb.DeleteRecipientRequest) (*pb.DeleteRecipientResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	recipientID := uint(req.GetRecipientId())
	if recipientID == 0 {
		return nil, status.Error(codes.InvalidArgument, "recipient_id is required")
	}

	err = c.recipientService.DeleteRecipient(ctx, user.ID, recipientID)
	if err != nil {
		// Map service errors
		if errors.Is(err, services.ErrRecipientNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		} else if errors.Is(err, services.ErrRecipientDeleteFailed) {
			return nil, status.Errorf(codes.Internal, "failed to delete recipient: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "an unexpected error occurred: %v", err)
	}

	return &pb.DeleteRecipientResponse{
		Message: "Recipient deleted successfully",
	}, nil
}

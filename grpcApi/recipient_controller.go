package grpcApi

import (
	"context"
	"errors" // Needed for getUserFromContext dependency
	"fmt"
	"lazervaultGo/models" // Added for models.User to *pb.SimilarRecipientUser conversion
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

// GetRecipient handles the gRPC request to retrieve a specific recipient.
func (c *RecipientController) GetRecipient(ctx context.Context, req *pb.GetRecipientRequest) (*pb.GetRecipientResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err // Propagate auth/user lookup errors
	}

	recipientID := uint(req.GetRecipientId())
	if recipientID == 0 {
		return nil, status.Error(codes.InvalidArgument, "recipient_id is required")
	}

	foundModel, err := c.recipientService.GetRecipientByID(ctx, recipientID, user.ID)
	if err != nil {
		if errors.Is(err, services.ErrRecipientNotFound) {
			return nil, status.Errorf(codes.NotFound, "recipient with id %d not found", recipientID)
		} else if errors.Is(err, services.ErrRecipientAccessDenied) {
			return nil, status.Errorf(codes.NotFound, "recipient with id %d not found", recipientID)
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve recipient: %v", err)
	}

	return &pb.GetRecipientResponse{
		Recipient: services.ConvertRecipientToProto(foundModel),
	}, nil
}

// GetSimilarRecipientsByName handles the gRPC request to search for a user's saved recipients by name.
func (c *RecipientController) GetSimilarRecipientsByName(ctx context.Context, req *pb.GetSimilarRecipientsByNameRequest) (*pb.GetSimilarRecipientsByNameResponse, error) {
	// Get authenticated user
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err // Propagate auth/user lookup errors
	}

	if req.GetName() == "" { // Updated from GetNameQuery()
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	// Call the service layer method, now scoped to the authenticated user
	foundRecipientModels, err := c.recipientService.GetSimilarRecipientsByName(ctx, req.GetName(), user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search for recipients by name: %v", err)
	}

	// Convert models.Recipient to pb.FoundRecipientResult
	pbFoundRecipients := make([]*pb.FoundRecipientResult, 0, len(foundRecipientModels))
	for _, recipientModel := range foundRecipientModels {
		if recipientModel != nil {
			pbFoundRecipients = append(pbFoundRecipients, ConvertRecipientModelToFoundRecipientResultProto(recipientModel))
		}
	}

	return &pb.GetSimilarRecipientsByNameResponse{
		FoundRecipients: pbFoundRecipients, // Updated field name
	}, nil
}

// Helper to convert models.Recipient to pb.FoundRecipientResult
func ConvertRecipientModelToFoundRecipientResultProto(r *models.Recipient) *pb.FoundRecipientResult {
	if r == nil {
		return nil
	}
	return &pb.FoundRecipientResult{
		RecipientId: fmt.Sprint(r.ID),
		Name:        r.Name,
	}
}

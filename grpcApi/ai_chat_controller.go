package grpcApi

import (
	"context" // Added for GetAuthPayload
	"fmt"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"log" // Use standard Go log package

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AIChatController wraps the AIChatService and implements the gRPC server interface.
type AIChatController struct {
	pb.UnimplementedAIChatServiceServer // Embed for forward compatibility
	service                             *services.AIChatService
	userService                         services.IUserService // Added UserService dependency
}

// NewAIChatController creates a new AIChatController.
func NewAIChatController(service *services.AIChatService, userService services.IUserService) *AIChatController {
	return &AIChatController{
		service:     service,
		userService: userService,
	}
}

// ProcessChat extracts user ID and delegates the request to the underlying AIChatService.
func (c *AIChatController) ProcessChat(ctx context.Context, req *pb.ProcessChatRequest) (*pb.ProcessChatResponse, error) {
	// Extract authentication payload from context
	user, err := getUserFromContext(ctx, c.userService) // Use shared helper
	if err != nil {
		return nil, err // Error already includes status code
	}

	userID := user.ID
	if userID == 0 {
		log.Println("WARN: AIChatController: Invalid UserID (0) in auth payload")
		return nil, status.Error(codes.Unauthenticated, "invalid user ID in token")
	}

	log.Printf("INFO: AIChatController: Processing chat for User ID: %d", userID)

	// Delegate to the service, passing the extracted userID
	return c.service.ProcessChat(ctx, userID, req)
}

// IndexChatHistory triggers indexing for the authenticated user's chat history file.
func (c *AIChatController) IndexChatHistory(ctx context.Context, req *pb.IndexChatHistoryRequest) (*pb.IndexChatHistoryResponse, error) {
	// Extract authentication payload from context
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	userID := user.ID
	if userID == 0 {
		log.Println("WARN: AIChatController: Invalid UserID (0) in auth payload for IndexChatHistory")
		return nil, status.Error(codes.Unauthenticated, "invalid user ID in token")
	}

	log.Printf("INFO: AIChatController: Triggering chat history indexing for User ID: %d", userID)

	// Delegate to the service
	err = c.service.TriggerChatHistoryIndexing(ctx, userID)
	if err != nil {
		// The service layer should return appropriate gRPC status errors
		log.Printf("ERROR: AIChatController: Failed TriggerChatHistoryIndexing for User ID %d: %v", userID, err)
		return &pb.IndexChatHistoryResponse{
			Success: false,
			Msg:     fmt.Sprintf("Failed to trigger chat history indexing: %v", err),
		}, nil // Return error details in response, not as gRPC error unless internal
	}

	return &pb.IndexChatHistoryResponse{
		Success: true,
		Msg:     "Chat history indexing triggered successfully",
	}, nil
}

// IndexTransactionFile triggers indexing for the authenticated user's transaction file.
func (c *AIChatController) IndexTransactionFile(ctx context.Context, req *pb.IndexTransactionFileRequest) (*pb.IndexTransactionFileResponse, error) {
	// Extract authentication payload from context
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	userID := user.ID
	if userID == 0 {
		log.Println("WARN: AIChatController: Invalid UserID (0) in auth payload for IndexTransactionFile")
		return nil, status.Error(codes.Unauthenticated, "invalid user ID in token")
	}

	log.Printf("INFO: AIChatController: Triggering transaction file indexing for User ID: %d", userID)

	// Delegate to the service
	err = c.service.TriggerTransactionFileIndexing(ctx, userID)
	if err != nil {
		log.Printf("ERROR: AIChatController: Failed TriggerTransactionFileIndexing for User ID %d: %v", userID, err)
		return &pb.IndexTransactionFileResponse{
			Success: false,
			Msg:     fmt.Sprintf("Failed to trigger transaction file indexing: %v", err),
		}, nil // Return error details in response
	}

	return &pb.IndexTransactionFileResponse{
		Success: true,
		Msg:     "Transaction file indexing triggered successfully",
	}, nil
}

// GetAIChatHistory retrieves the AI chat history for the authenticated user.
func (c *AIChatController) GetAIChatHistory(ctx context.Context, req *pb.GetAIChatHistoryRequest) (*pb.GetAIChatHistoryResponse, error) {
	// Extract authentication payload from context
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err // Error already includes status code
	}

	userID := user.ID
	if userID == 0 {
		log.Println("WARN: AIChatController: Invalid UserID (0) in auth payload for GetAIChatHistory")
		return nil, status.Error(codes.Unauthenticated, "invalid user ID in token")
	}

	log.Printf("INFO: AIChatController: Getting AI chat history for User ID: %d", userID)

	// Delegate to the service
	resp, err := c.service.GetAIChatHistory(ctx, userID)
	if err != nil {
		// Service layer should return appropriate gRPC status errors
		log.Printf("ERROR: AIChatController: Failed GetAIChatHistory for User ID %d: %v", userID, err)
		return nil, err // Pass the status error up
	}

	return resp, nil
}

package grpcApi

import (
	"context" // Added for GetAuthPayload
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

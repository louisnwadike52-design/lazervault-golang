package grpcApi

import (
	"context"
	"fmt"

	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// GenerateTxDataController wraps the service and handles gRPC requests
type GenerateTxDataController struct {
	pb.UnimplementedGenerateTxDataServiceServer                                // Embed for forward compatibility
	service                                     services.GenerateTxDataService // The actual service logic
	tokenMaker                                  token.Maker                    // For validating access tokens
	db                                          *gorm.DB                       // Add db field to fetch user
}

// NewGenerateTxDataController creates a new controller
func NewGenerateTxDataController(service services.GenerateTxDataService, tokenMaker token.Maker, db *gorm.DB) *GenerateTxDataController {
	return &GenerateTxDataController{
		service:    service,
		tokenMaker: tokenMaker,
		db:         db, // Initialize db field
	}
}

// GenerateUserTxDataFile handles the gRPC request, relying on middleware for authentication.
func (c *GenerateTxDataController) GenerateUserTxDataFile(ctx context.Context, req *pb.GenerateUserTxDataFileRequest) (*pb.GenerateUserTxDataFileResponse, error) {
	// 1. Retrieve Payload from Context (injected by middleware)
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}

	// 2. Get Authenticated User from DB using Email from Payload
	var user models.User
	if err := c.db.WithContext(ctx).Where("email = ?", authPayload.Email).Select("id").First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		fmt.Printf("ERROR: Failed to query user by email '%s': %v\n", authPayload.Email, err)
		return nil, status.Errorf(codes.Internal, "failed to retrieve user information")
	}
	authenticatedUserID := user.ID // User ID is uint

	// 3. Prepare Request for Service (using authenticated user ID)
	serviceReq := &pb.GenerateUserTxDataFileRequest{
		UserId: fmt.Sprintf("%d", authenticatedUserID),
	}

	// 4. Call the Service
	resp, err := c.service.GenerateUserTxDataFile(ctx, serviceReq)
	if err != nil {
		return nil, err
	}

	// 5. Return Response
	return resp, nil
}

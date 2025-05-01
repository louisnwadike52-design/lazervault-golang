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

// Uses the same authorizationPayloadKey defined (or to be defined) globally or in a common place
// const authorizationPayloadKey = "authorization_payload"

// TxFileController handles gRPC requests for transaction file operations.
type TxFileController struct {
	pb.UnimplementedTxFileServiceServer                         // Embed for forward compatibility
	service                             services.ITxFileService // Use the interface
	tokenMaker                          token.Maker
	db                                  *gorm.DB
}

// NewTxFileController creates a new controller.
func NewTxFileController(service services.ITxFileService, tokenMaker token.Maker, db *gorm.DB) *TxFileController {
	return &TxFileController{
		service:    service,
		tokenMaker: tokenMaker,
		db:         db,
	}
}

// GetUserTxFilePath handles authentication via middleware and calls the service.
func (c *TxFileController) GetUserTxFilePath(ctx context.Context, req *pb.GetUserTxFilePathRequest) (*pb.GetUserTxFilePathResponse, error) {
	// 1. Retrieve Payload from Context (injected by middleware)
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}

	// 2. Get User ID from DB using Email from Payload (to ensure user exists)
	var user models.User
	if err := c.db.WithContext(ctx).Where("email = ?", authPayload.Email).Select("id").First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		fmt.Printf("ERROR: Failed to query user by email '%s': %v\n", authPayload.Email, err)
		return nil, status.Errorf(codes.Internal, "failed to retrieve user information")
	}
	authenticatedUserIDStr := fmt.Sprintf("%d", user.ID)

	// 3. Call the Service (passing the string user ID)
	resp, err := c.service.GetUserTxFilePath(ctx, authenticatedUserIDStr)
	if err != nil {
		// Pass service errors (including NotFound) through
		return nil, err
	}

	// 4. Return Response (already contains the path)
	return resp, nil
}

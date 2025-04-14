package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/grpcApi/middleware" // Import middleware package
	"lazervaultGo/models"             // For user lookup
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token" // For getting payload from context
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// TransferController holds the dependencies for the transfer gRPC service
type TransferController struct {
	pb.UnimplementedTransferServiceServer
	transferService *services.TransferService
	db              *gorm.DB // Need db to fetch user by email
}

// NewTransferController creates a new TransferController
func NewTransferController(transferService *services.TransferService, db *gorm.DB) *TransferController {
	return &TransferController{
		transferService: transferService,
		db:              db,
	}
}

// InitiateTransfer handles the gRPC request to initiate a transfer
func (c *TransferController) InitiateTransfer(ctx context.Context, req *pb.InitiateTransferRequest) (*pb.InitiateTransferResponse, error) {
	// 1. Get authenticated user payload from context using the exported key
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}

	// 2. Get sender User ID from email in payload
	var fromUser models.User
	if err := c.db.Where("email = ?", authPayload.Email).First(&fromUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "sender user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to find sender user: %v", err)
	}

	// 3. Validate request (basic validation, more can be added)
	if req.GetAmount() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetToUserId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "to_user_id is required")
	}
	if uint(req.GetToUserId()) == fromUser.ID {
		return nil, status.Errorf(codes.InvalidArgument, "cannot transfer to self")
	}

	// 4. Prepare service request
	serviceReq := services.TransferRequest{
		FromUserID:  fromUser.ID, // Use ID fetched from DB
		ToUserID:    uint(req.GetToUserId()),
		Amount:      req.GetAmount(),
		Category:    req.GetCategory(),
		Reference:   req.GetReference(),
		ScheduledAt: nil, // Handle optional timestamp
	}

	// Check if scheduled_at is provided and valid
	if req.ScheduledAt != nil && req.ScheduledAt.IsValid() {
		scheduledTime := req.ScheduledAt.AsTime()
		if scheduledTime.After(time.Now()) { // Ensure schedule is in the future
			serviceReq.ScheduledAt = &scheduledTime
		} else {
			// Optional: return error for past schedule time, or just ignore it
			// return nil, status.Errorf(codes.InvalidArgument, "scheduled_at must be in the future")
		}
	}

	// 5. Call the service
	res, err := c.transferService.InitiateTransfer(ctx, serviceReq.FromUserID, serviceReq)
	if err != nil {
		// TODO: Map service errors to appropriate gRPC codes
		return nil, status.Errorf(codes.Internal, "failed to initiate transfer: %v", err)
	}

	// 6. Prepare gRPC response
	grpcRes := &pb.InitiateTransferResponse{
		TransferId:  uint64(res.TransferID),
		Status:      res.Status,
		Amount:      res.Amount,
		Fee:         res.Fee,
		TotalAmount: res.TotalAmount,
		CreatedAt:   timestamppb.New(res.CreatedAt),
	}

	return grpcRes, nil
}

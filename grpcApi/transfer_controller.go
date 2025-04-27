package grpcApi

import (
	"context"
	"errors"
	"fmt"
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

// Helper to convert Transfer model to GetTransferDetailsResponse proto
func convertTransferModelToProtoDetails(t *models.Transfer) *pb.GetTransferDetailsResponse {
	if t == nil {
		return nil
	}
	resp := &pb.GetTransferDetailsResponse{
		TransferId:    uint64(t.ID),
		FromAccountId: uint64(t.FromAccountID),
		ToAccountId:   uint64(t.ToAccountID),
		FromUserId:    uint64(t.FromUserID),
		ToUserId:      uint64(t.ToUserID),
		Amount:        uint64(t.Amount),
		Fee:           uint64(t.Fee),
		TotalAmount:   uint64(t.TotalAmount),
		// Currency: Need to fetch from account if not stored on transfer model
		Status:        string(t.Status),
		Reference:     t.Reference,
		Category:      t.Category,
		CreatedAt:     timestamppb.New(t.CreatedAt),
		FailureReason: t.FailureReason,
	}
	if t.ScheduledAt != nil {
		resp.ScheduledAt = timestamppb.New(*t.ScheduledAt)
	}
	if t.CompletedAt != nil {
		resp.CompletedAt = timestamppb.New(*t.CompletedAt)
	}
	if t.FailedAt != nil {
		resp.FailedAt = timestamppb.New(*t.FailedAt)
	}
	// Assuming Currency needs to be fetched if not on model
	// if t.FromAccount != nil { // Example if preloaded
	// 	 resp.Currency = t.FromAccount.Currency
	// } else if t.ToAccount != nil {
	// 	 resp.Currency = t.ToAccount.Currency
	// }
	return resp
}

// InitiateTransfer handles the gRPC request to initiate a transfer
func (c *TransferController) InitiateTransfer(ctx context.Context, req *pb.InitiateTransferRequest) (*pb.InitiateTransferResponse, error) {
	// 1. Get authenticated user payload
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

	// 3. Validate request using new fields
	if req.GetAmount() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetFromAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "from_account_id is required")
	}
	if req.GetToAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "to_account_id is required")
	}
	if req.GetFromAccountId() == req.GetToAccountId() {
		return nil, status.Errorf(codes.InvalidArgument, "cannot transfer to the same account")
	}

	// 4. Prepare service request using new fields
	serviceReq := services.TransferRequest{
		FromUserID:    fromUser.ID, // Pass authenticated user ID
		FromAccountID: uint(req.GetFromAccountId()),
		ToAccountID:   uint(req.GetToAccountId()),
		Amount:        int64(req.GetAmount()), // Convert uint64 to int64
		Category:      req.GetCategory(),
		Reference:     req.GetReference(),
		ScheduledAt:   nil,
	}

	if req.ScheduledAt != nil && req.ScheduledAt.IsValid() {
		scheduledTime := req.ScheduledAt.AsTime()
		if scheduledTime.After(time.Now()) {
			serviceReq.ScheduledAt = &scheduledTime
		}
	}

	// 5. Call the service
	res, err := c.transferService.InitiateTransfer(ctx, serviceReq.FromUserID, serviceReq)
	if err != nil {
		// Map specific service errors to gRPC codes
		if errors.Is(err, services.ErrTransferFromAccountNotFound) || errors.Is(err, services.ErrTransferToAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		} else if errors.Is(err, services.ErrCannotTransferToSelfAccount) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		} else if errors.Is(err, services.ErrTransferAccountLookupFailed) {
			// Log this internal error
			fmt.Printf("ERROR looking up accounts during transfer: %v\n", err)
			return nil, status.Errorf(codes.Internal, "failed to validate accounts")
		} // Add more specific error mappings if needed (e.g., currency mismatch)

		return nil, status.Errorf(codes.Internal, "failed to initiate transfer: %v", err)
	}

	// 6. Prepare gRPC response
	grpcRes := &pb.InitiateTransferResponse{
		TransferId:  uint64(res.TransferID),
		Status:      res.Status,
		Amount:      uint64(res.Amount),
		Fee:         uint64(res.Fee),
		TotalAmount: uint64(res.TotalAmount),
		CreatedAt:   timestamppb.New(res.CreatedAt),
	}

	return grpcRes, nil
}

// GetTransferDetails handles the gRPC request to retrieve transfer details.
func (c *TransferController) GetTransferDetails(ctx context.Context, req *pb.GetTransferDetailsRequest) (*pb.GetTransferDetailsResponse, error) {
	// 1. Get User ID from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	var user models.User // Need user ID for service call
	if err := c.db.Where("email = ?", authPayload.Email).First(&user).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to find user: %v", err)
	}

	// 2. Validate Request
	transferID := req.GetTransferId()
	if transferID == 0 {
		return nil, status.Error(codes.InvalidArgument, "transfer_id is required")
	}

	// 3. Call Service
	transferModel, err := c.transferService.GetTransferDetails(ctx, uint(transferID), user.ID)
	if err != nil {
		// Map service errors
		if errors.Is(err, services.ErrTransferNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		} else if errors.Is(err, services.ErrTransferAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "failed to get transfer details: %v", err)
	}

	// 4. Convert model to proto response
	resp := convertTransferModelToProtoDetails(transferModel)
	// TODO: Fetch currency if needed and not included in model/preload
	// if resp.Currency == "" { ... fetch from account ... }

	return resp, nil
}

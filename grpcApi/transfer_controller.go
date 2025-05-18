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
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// TransferController holds the dependencies for the transfer gRPC service
type TransferController struct {
	pb.UnimplementedTransferServiceServer
	transferService services.ITransferService // Use interface
	db              *gorm.DB
}

// NewTransferController creates a new TransferController
func NewTransferController(transferService services.ITransferService, db *gorm.DB) *TransferController {
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
		ToAccountId:   0, // Default to 0 if nil
		FromUserId:    uint64(t.FromUserID),
		ToUserId:      0, // Default to 0 if nil
		Amount:        uint64(t.Amount),
		Fee:           uint64(t.Fee),
		TotalAmount:   uint64(t.TotalAmount),
		Status:        string(t.Status),
		Reference:     t.Reference,
		Category:      t.Category,
		CreatedAt:     timestamppb.New(t.CreatedAt),
		FailureReason: t.FailureReason,
	}
	if t.ToAccountID != nil {
		resp.ToAccountId = uint64(*t.ToAccountID)
	}
	if t.ToUserID != nil {
		resp.ToUserId = uint64(*t.ToUserID)
	}
	if t.ScheduledAt != nil {
		resp.ScheduledAt = t.ScheduledAt // Already a string pointer
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

	// 3. Prepare service request from proto request
	serviceReq := services.TransferRequest{
		FromUserID:    fromUser.ID,
		FromAccountID: uint(req.GetFromAccountId()),
		Amount:        int64(req.GetAmount()),
		Category:      req.GetCategory(),
		Reference:     req.GetReference(),
		ScheduledAt:   nil,
		// Initialize destination fields as nil
		ToAccountID: nil,
		RecipientID: nil,
	}

	// --- Map Destination (Only one will be set) ---
	if req.GetToAccountId() > 0 {
		toAccountID := uint(req.GetToAccountId())
		serviceReq.ToAccountID = &toAccountID
	} else if req.GetRecipientId() > 0 {
		recipientID := uint(req.GetRecipientId())
		serviceReq.RecipientID = &recipientID
	}
	// Service layer handles validation that exactly one was provided

	// --- Remove mapping for deleted fields (already removed) ---

	if req.GetScheduledAt() != "" {
		// Parse the scheduled time string to validate format
		scheduledTime, err := time.Parse(time.RFC3339, req.GetScheduledAt())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "Invalid scheduled_at format. Expected ISO 8601 UTC format (e.g., 2024-03-20T15:04:05Z)")
		}

		// Validate scheduled time is in the future
		if scheduledTime.Before(time.Now().UTC()) {
			return nil, status.Error(codes.InvalidArgument, "Scheduled time must be in the future")
		}

		// Use the validated string directly
		scheduledAt := req.GetScheduledAt()
		serviceReq.ScheduledAt = &scheduledAt
	}

	// 5. Call the service
	res, err := c.transferService.InitiateTransfer(ctx, serviceReq.FromUserID, serviceReq)
	if err != nil {
		// Updated Error Mapping
		// Check specific account service errors
		if errors.Is(err, services.ErrSvcAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, "account not found: %v", err)
		} else if errors.Is(err, services.ErrSvcAccountAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "account access denied: %v", err)
		} else if errors.Is(err, services.ErrSvcInsufficientFunds) {
			return nil, status.Errorf(codes.FailedPrecondition, "insufficient funds: %v", err)
		}
		// Check for specific transfer service errors
		if errors.Is(err, services.ErrCannotTransferToSelfAccount) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}
		// Handle recipient errors based on wrapped error message or potentially specific types
		if strings.Contains(err.Error(), "recipient") { // Basic check, improve if recipient service exports errors
			if strings.Contains(err.Error(), "not found") {
				return nil, status.Errorf(codes.NotFound, err.Error())
			} else if strings.Contains(err.Error(), "access denied") {
				return nil, status.Errorf(codes.PermissionDenied, err.Error())
			} else if strings.Contains(err.Error(), "missing required") || strings.Contains(err.Error(), "missing linked account ID") || strings.Contains(err.Error(), "invalid or unsupported") {
				return nil, status.Errorf(codes.InvalidArgument, err.Error())
			}
		}
		// Check for the "provide either/or" validation error from the service
		if strings.Contains(err.Error(), "provide either to_account_id OR recipient_id") || strings.Contains(err.Error(), "either to_account_id OR recipient_id must be provided") {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}

		// Log other internal errors
		log.Error().Err(err).Msg("failed to initiate transfer")
		return nil, status.Errorf(codes.Internal, "failed to initiate transfer")
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
	var user models.User
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
		if errors.Is(err, services.ErrTransferNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		} else if errors.Is(err, services.ErrTransferAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, err.Error())
		}
		fmt.Printf("ERROR GetTransferDetails: %v\n", err)
		return nil, status.Errorf(codes.Internal, "failed to get transfer details")
	}

	// 4. Convert model to proto response
	resp := convertTransferModelToProtoDetails(transferModel)
	// TODO: Add currency fetching logic if needed

	return resp, nil
}

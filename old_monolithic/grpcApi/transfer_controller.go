package grpcApi

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"lazervaultGo/utils"
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

// ListTransfers handles the gRPC request to list all transfers with pagination
func (c *TransferController) ListTransfers(ctx context.Context, req *pb.ListTransfersRequest) (*pb.ListTransfersResponse, error) {
	// 1. Get User ID from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	var user models.User
	if err := c.db.Where("email = ?", authPayload.Email).First(&user).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to find user: %v", err)
	}

	// 2. Set up pagination parameters
	page := int(req.GetPage())
	if page == 0 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100 // Enforce max page size
	}

	params := &utils.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    req.GetSortBy(),
		SortOrder: req.GetSortOrder(),
	}

	// Set defaults
	if params.SortBy == "" {
		params.SortBy = "created_at"
	}
	if params.SortOrder == "" {
		params.SortOrder = "desc"
	}

	// 3. Build query - filter by user (sender or recipient)
	query := c.db.Model(&models.Transfer{}).Where("from_user_id = ? OR to_user_id = ?", user.ID, user.ID)

	// 4. Apply optional filters
	if req.GetStatus() != "" {
		query = query.Where("status = ?", req.GetStatus())
	}

	// Apply search if provided (search in reference and category)
	if req.GetSearch() != "" {
		searchPattern := "%" + req.GetSearch() + "%"
		query = query.Where("reference ILIKE ? OR category ILIKE ?", searchPattern, searchPattern)
	}

	// 5. Get paginated results
	var transfers []models.Transfer
	paginatedResp, err := utils.Paginate(query, params, &transfers)
	if err != nil {
		log.Error().Err(err).Msg("failed to paginate transfers")
		return nil, status.Errorf(codes.Internal, "failed to retrieve transfers")
	}

	// 6. Convert to proto responses
	transferProtos := make([]*pb.GetTransferDetailsResponse, len(transfers))
	for i, t := range transfers {
		transferProtos[i] = convertTransferModelToProtoDetails(&t)
	}

	// 7. Build pagination metadata
	paginationMeta := &pb.TransferPaginationInfo{
		CurrentPage:  int32(paginatedResp.Pagination.CurrentPage),
		TotalPages:   int32(paginatedResp.Pagination.TotalPages),
		TotalItems:   int32(paginatedResp.Pagination.TotalRecords),
		ItemsPerPage: int32(paginatedResp.Pagination.PageSize),
		HasNext:      paginatedResp.Pagination.HasNext,
		HasPrev:      paginatedResp.Pagination.HasPrevious,
	}

	return &pb.ListTransfersResponse{
		Transfers:  transferProtos,
		Pagination: paginationMeta,
	}, nil
}

// InitiateBatchTransfer handles the gRPC request to initiate a batch transfer
func (c *TransferController) InitiateBatchTransfer(ctx context.Context, req *pb.InitiateBatchTransferRequest) (*pb.InitiateBatchTransferResponse, error) {
	// 1. Get authenticated user payload
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}

	// 2. Get user ID from email
	var fromUser models.User
	if err := c.db.Where("email = ?", authPayload.Email).First(&fromUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to find user: %v", err)
	}

	// 3. Validate request
	if req.GetFromAccountId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "from_account_id is required")
	}
	if len(req.GetRecipients()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one recipient is required")
	}

	// 4. Convert proto recipients to service recipients
	recipients := make([]services.BatchTransferRecipient, len(req.GetRecipients()))
	for i, protoRecipient := range req.GetRecipients() {
		recipient := services.BatchTransferRecipient{
			Amount:    int64(protoRecipient.GetAmount()),
			Reference: protoRecipient.GetReference(),
			Category:  protoRecipient.GetCategory(),
		}

		// Set destination (recipient_id or to_account_id)
		if protoRecipient.GetRecipientId() > 0 {
			recipientID := uint(protoRecipient.GetRecipientId())
			recipient.RecipientID = &recipientID
		} else if protoRecipient.GetToAccountId() > 0 {
			toAccountID := uint(protoRecipient.GetToAccountId())
			recipient.ToAccountID = &toAccountID
		}

		recipients[i] = recipient
	}

	// 5. Prepare service request
	serviceReq := services.BatchTransferRequest{
		FromAccountID: uint(req.GetFromAccountId()),
		Recipients:    recipients,
		ScheduledAt:   nil,
	}

	// Handle scheduled_at if provided
	if req.GetScheduledAt() != "" {
		scheduledTime, err := time.Parse(time.RFC3339, req.GetScheduledAt())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid scheduled_at format. Expected ISO 8601 UTC format")
		}
		if scheduledTime.Before(time.Now().UTC()) {
			return nil, status.Error(codes.InvalidArgument, "scheduled time must be in the future")
		}
		scheduledAt := req.GetScheduledAt()
		serviceReq.ScheduledAt = &scheduledAt
	}

	// 6. Call service
	res, err := c.transferService.InitiateBatchTransfer(ctx, fromUser.ID, serviceReq)
	if err != nil {
		// Error handling
		if errors.Is(err, services.ErrSvcAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, "account not found: %v", err)
		} else if errors.Is(err, services.ErrSvcAccountAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "account access denied: %v", err)
		} else if errors.Is(err, services.ErrSvcInsufficientFunds) {
			return nil, status.Errorf(codes.FailedPrecondition, "insufficient funds: %v", err)
		}

		log.Error().Err(err).Msg("failed to initiate batch transfer")
		return nil, status.Errorf(codes.Internal, "failed to initiate batch transfer")
	}

	// 7. Convert service response to proto response
	results := make([]*pb.BatchTransferResult, len(res.Results))
	for i, result := range res.Results {
		results[i] = &pb.BatchTransferResult{
			TransferId:       uint64(result.TransferID),
			Status:           result.Status,
			Amount:           uint64(result.Amount),
			Fee:              uint64(result.Fee),
			RecipientName:    result.RecipientName,
			RecipientAccount: result.RecipientAccount,
			FailureReason:    result.FailureReason,
		}
	}

	var completedAt *timestamppb.Timestamp
	if res.CompletedAt != nil {
		completedAt = timestamppb.New(*res.CompletedAt)
	}

	grpcRes := &pb.InitiateBatchTransferResponse{
		BatchId:             uint64(res.BatchID),
		Status:              res.Status,
		TotalAmount:         uint64(res.TotalAmount),
		TotalFee:            uint64(res.TotalFee),
		TotalAmountWithFee:  uint64(res.TotalAmountWithFee),
		SuccessfulTransfers: res.SuccessfulTransfers,
		FailedTransfers:     res.FailedTransfers,
		TotalTransfers:      res.TotalTransfers,
		Results:             results,
		CreatedAt:           timestamppb.New(res.CreatedAt),
		CompletedAt:         completedAt,
	}

	return grpcRes, nil
}

// GetBatchTransferStatus handles the gRPC request to get batch transfer status
func (c *TransferController) GetBatchTransferStatus(ctx context.Context, req *pb.GetBatchTransferStatusRequest) (*pb.GetBatchTransferStatusResponse, error) {
	// 1. Get authenticated user payload
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}

	// 2. Get user ID from email
	var user models.User
	if err := c.db.Where("email = ?", authPayload.Email).First(&user).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to find user: %v", err)
	}

	// 3. Validate request
	batchID := req.GetBatchId()
	if batchID == 0 {
		return nil, status.Error(codes.InvalidArgument, "batch_id is required")
	}

	// 4. Call service
	res, err := c.transferService.GetBatchTransferStatus(ctx, uint(batchID), user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, err.Error())
		} else if strings.Contains(err.Error(), "access denied") {
			return nil, status.Errorf(codes.PermissionDenied, err.Error())
		}
		log.Error().Err(err).Msg("failed to get batch transfer status")
		return nil, status.Errorf(codes.Internal, "failed to get batch transfer status")
	}

	// 5. Convert service response to proto response
	results := make([]*pb.BatchTransferResult, len(res.Results))
	for i, result := range res.Results {
		results[i] = &pb.BatchTransferResult{
			TransferId:       uint64(result.TransferID),
			Status:           result.Status,
			Amount:           uint64(result.Amount),
			Fee:              uint64(result.Fee),
			RecipientName:    result.RecipientName,
			RecipientAccount: result.RecipientAccount,
			FailureReason:    result.FailureReason,
		}
	}

	var completedAt *timestamppb.Timestamp
	if res.CompletedAt != nil {
		completedAt = timestamppb.New(*res.CompletedAt)
	}

	grpcRes := &pb.GetBatchTransferStatusResponse{
		BatchId:             uint64(res.BatchID),
		Status:              res.Status,
		TotalAmount:         uint64(res.TotalAmount),
		TotalFee:            uint64(res.TotalFee),
		TotalAmountWithFee:  uint64(res.TotalAmountWithFee),
		SuccessfulTransfers: res.SuccessfulTransfers,
		FailedTransfers:     res.FailedTransfers,
		TotalTransfers:      res.TotalTransfers,
		Results:             results,
		CreatedAt:           timestamppb.New(res.CreatedAt),
		CompletedAt:         completedAt,
	}

	return grpcRes, nil
}

// GetBatchTransferHistory handles the gRPC request to get batch transfer history
func (c *TransferController) GetBatchTransferHistory(ctx context.Context, req *pb.GetBatchTransferHistoryRequest) (*pb.GetBatchTransferHistoryResponse, error) {
	// 1. Get authenticated user payload
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}

	// 2. Get user ID from email
	var user models.User
	if err := c.db.Where("email = ?", authPayload.Email).First(&user).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to find user: %v", err)
	}

	// 3. Set up pagination parameters
	page := req.GetPage()
	if page == 0 {
		page = 1
	}
	pageSize := req.GetPageSize()
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 4. Call service
	res, err := c.transferService.GetBatchTransferHistory(ctx, user.ID, page, pageSize, req.GetStatus())
	if err != nil {
		log.Error().Err(err).Msg("failed to get batch transfer history")
		return nil, status.Errorf(codes.Internal, "failed to get batch transfer history")
	}

	// 5. Convert service response to proto response
	batches := make([]*pb.GetBatchTransferStatusResponse, len(res.Batches))
	for i, batch := range res.Batches {
		var completedAt *timestamppb.Timestamp
		if batch.CompletedAt != nil {
			completedAt = timestamppb.New(*batch.CompletedAt)
		}

		batches[i] = &pb.GetBatchTransferStatusResponse{
			BatchId:             uint64(batch.BatchID),
			Status:              batch.Status,
			TotalAmount:         uint64(batch.TotalAmount),
			TotalFee:            uint64(batch.TotalFee),
			TotalAmountWithFee:  uint64(batch.TotalAmountWithFee),
			SuccessfulTransfers: batch.SuccessfulTransfers,
			FailedTransfers:     batch.FailedTransfers,
			TotalTransfers:      batch.TotalTransfers,
			Results:             nil, // History doesn't include individual results
			CreatedAt:           timestamppb.New(batch.CreatedAt),
			CompletedAt:         completedAt,
		}
	}

	paginationMeta := &pb.TransferPaginationInfo{
		CurrentPage:  res.CurrentPage,
		TotalPages:   res.TotalPages,
		TotalItems:   res.TotalItems,
		ItemsPerPage: res.ItemsPerPage,
		HasNext:      res.HasNext,
		HasPrev:      res.HasPrev,
	}

	grpcRes := &pb.GetBatchTransferHistoryResponse{
		Batches:    batches,
		Pagination: paginationMeta,
	}

	return grpcRes, nil
}

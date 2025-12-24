package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"lazervaultGo/utils"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// DepositController handles gRPC requests for the DepositService.
type DepositController struct {
	pb.UnimplementedDepositServiceServer // Embed for forward compatibility
	depositService                       services.IDepositService
	userService                          services.IUserService // Needed to get UserID
	db                                   *gorm.DB              // For direct DB queries
}

// NewDepositController creates a new DepositController.
func NewDepositController(depositService services.IDepositService, userService services.IUserService, db *gorm.DB) *DepositController {
	return &DepositController{
		depositService: depositService,
		userService:    userService,
		db:             db,
	}
}

// InitiateDeposit handles the gRPC request to start an asynchronous deposit.
func (c *DepositController) InitiateDeposit(ctx context.Context, req *pb.InitiateDepositRequest) (*pb.InitiateDepositResponse, error) {
	// 1. Get User ID from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	userID := user.ID // uint user ID

	// 2. Basic Input Validation (Service layer does more)
	if req.GetTargetAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "target_account_id is required")
	}
	if req.GetAmount() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetCurrency() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "currency is required")
	}
	if req.GetSourceBankName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "source_bank_name is required")
	}

	// 3. Call Service
	initiationResponse, err := c.depositService.InitiateDeposit(ctx, userID, req)
	if err != nil {
		// Map service errors to gRPC status codes
		if errors.Is(err, services.ErrDepositInvalidAmount) || errors.Is(err, services.ErrDepositCurrencyMissing) || errors.Is(err, services.ErrDepositSourceBankMissing) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		} else if errors.Is(err, services.ErrDepositInitiationFailed) {
			// Log internal error details if possible
			return nil, status.Errorf(codes.Internal, "failed to save deposit request")
		} else if errors.Is(err, services.ErrDepositEnqueueTaskFailed) {
			// Log internal error details if possible
			return nil, status.Errorf(codes.Internal, "failed to schedule deposit processing")
		}
		// Generic internal error for unexpected issues
		return nil, status.Errorf(codes.Internal, "failed to initiate deposit: %v", err)
	}

	// 4. Return successful acknowledgement response from service
	return initiationResponse, nil
}

// GetDepositDetails handles the gRPC request to retrieve deposit details.
func (c *DepositController) GetDepositDetails(ctx context.Context, req *pb.GetDepositDetailsRequest) (*pb.GetDepositDetailsResponse, error) {
	// 1. Get User ID from context using the shared helper
	// We need access to userService, which is already injected into the controller.
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err // Error already contains gRPC status
	}

	// 2. Validate Request
	depositID := req.GetDepositId()
	if depositID == "" {
		return nil, status.Error(codes.InvalidArgument, "deposit_id is required")
	}
	// Basic UUID validation (optional but good practice)
	// if _, err := uuid.Parse(depositID); err != nil {
	// 	 return nil, status.Errorf(codes.InvalidArgument, "invalid deposit_id format: %v", err)
	// }

	// 3. Call Service
	detailsResponse, err := c.depositService.GetDepositDetails(ctx, depositID, user.ID)
	if err != nil {
		// Map service errors to gRPC status codes
		if errors.Is(err, services.ErrDepositNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		} else if errors.Is(err, services.ErrDepositAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, err.Error())
		}
		// Handle other potential errors (e.g., DB connection issues)
		return nil, status.Errorf(codes.Internal, "failed to get deposit details: %v", err)
	}

	// 4. Return successful response from service
	return detailsResponse, nil
}

// Helper to convert Deposit model to GetDepositDetailsResponse proto
func convertDepositModelToProtoDetails(d *models.Deposit) *pb.GetDepositDetailsResponse {
	if d == nil {
		return nil
	}
	resp := &pb.GetDepositDetailsResponse{
		DepositId:             d.ID,
		TargetAccountId:       uint64(d.TargetAccountID),
		Amount:                uint64(d.Amount),
		Currency:              d.Currency,
		SourceBankName:        d.SourceBankName,
		Status:                pb.DepositStatus(pb.DepositStatus_value["DEPOSIT_STATUS_"+string(d.Status)]),
		CreatedAt:             timestamppb.New(d.CreatedAt),
		ExternalTransactionId: "",
		FailureReason:         "",
	}
	if d.ExternalTransactionID != nil {
		resp.ExternalTransactionId = *d.ExternalTransactionID
	}
	if d.FailureReason != nil {
		resp.FailureReason = *d.FailureReason
	}
	if d.ProcessingAt != nil {
		resp.ProcessingAt = timestamppb.New(*d.ProcessingAt)
	}
	if d.CompletedAt != nil {
		resp.CompletedAt = timestamppb.New(*d.CompletedAt)
	}
	if d.FailedAt != nil {
		resp.FailedAt = timestamppb.New(*d.FailedAt)
	}
	return resp
}

// ListDeposits handles the gRPC request to list all deposits with pagination
func (c *DepositController) ListDeposits(ctx context.Context, req *pb.ListDepositsRequest) (*pb.ListDepositsResponse, error) {
	// 1. Get User ID from context
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err // Error already contains gRPC status
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

	// 3. Build query - filter by user
	query := c.db.Model(&models.Deposit{}).Where("user_id = ?", user.ID)

	// 4. Apply optional filters
	if req.GetStatus() != "" {
		query = query.Where("status = ?", req.GetStatus())
	}

	// 5. Get paginated results
	var deposits []models.Deposit
	paginatedResp, err := utils.Paginate(query, params, &deposits)
	if err != nil {
		log.Error().Err(err).Msg("failed to paginate deposits")
		return nil, status.Errorf(codes.Internal, "failed to retrieve deposits")
	}

	// 6. Convert to proto responses
	depositProtos := make([]*pb.GetDepositDetailsResponse, len(deposits))
	for i, d := range deposits {
		depositProtos[i] = convertDepositModelToProtoDetails(&d)
	}

	// 7. Build pagination metadata
	paginationMeta := &pb.DepositPaginationInfo{
		CurrentPage:  int32(paginatedResp.Pagination.CurrentPage),
		TotalPages:   int32(paginatedResp.Pagination.TotalPages),
		TotalItems:   int32(paginatedResp.Pagination.TotalRecords),
		ItemsPerPage: int32(paginatedResp.Pagination.PageSize),
		HasNext:      paginatedResp.Pagination.HasNext,
		HasPrev:      paginatedResp.Pagination.HasPrevious,
	}

	return &pb.ListDepositsResponse{
		Deposits:   depositProtos,
		Pagination: paginationMeta,
	}, nil
}

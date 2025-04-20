package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AccountController handles gRPC requests for the AccountService.
type AccountController struct {
	pb.UnimplementedAccountServiceServer                          // Embed for forward compatibility
	accountService                       services.IAccountService // Use interface type
	userService                          services.IUserService    // Inject userService
	// db                                   *gorm.DB // Remove db if only used for user lookup
}

// NewAccountController creates a new AccountController.
func NewAccountController(accountService services.IAccountService, userService services.IUserService) *AccountController { // Accept interfaces
	return &AccountController{
		accountService: accountService,
		userService:    userService,
		// db:             db,
	}
}

// convertAccount converts a models.Account to a pb.Account.
func convertAccount(account *models.Account) *pb.Account {
	if account == nil {
		return nil
	}
	return &pb.Account{
		Id:            uint64(account.ID),
		AccountType:   account.AccountType,
		Currency:      account.Currency,
		Balance:       account.Balance,
		AccountNumber: account.AccountNumber,
		IsActive:      account.IsActive,
		CreatedAt:     timestamppb.New(account.CreatedAt),
		UpdatedAt:     timestamppb.New(account.UpdatedAt),
	}
}

// CreateAccount handles the RPC for creating a new user account.
func (c *AccountController) CreateAccount(ctx context.Context, req *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
	// 1. Get Owner User ID from context using userService
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	// Fetch user via userService
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	ownerUserID := user.ID // Use uint ID

	// 2. Validate Request
	if req.GetAccountType() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "account_type is required")
	}
	if req.GetCurrency() == "" { // TODO: Add proper currency code validation (e.g., check against a list)
		return nil, status.Errorf(codes.InvalidArgument, "currency is required")
	}

	// 3. Prepare Service Request (Service expects uint OwnerUserID)
	serviceReq := services.CreateAccountRequest{
		OwnerUserID: ownerUserID, // Pass uint ID directly
		AccountType: req.GetAccountType(),
		Currency:    req.GetCurrency(),
	}

	// 4. Call Service
	newAccount, err := c.accountService.CreateAccount(ctx, serviceReq)
	if err != nil {
		if errors.Is(err, services.ErrInvalidAccountType) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid account_type: %v", req.GetAccountType())
		}
		// Handle other potential service errors
		return nil, status.Errorf(codes.Internal, "failed to create account: %v", err)
	}

	// 5. Convert and Return Response
	resp := &pb.CreateAccountResponse{
		Account: convertAccount(newAccount),
	}
	return resp, nil
}

// GetAccounts handles the RPC for retrieving all accounts for the authenticated user.
func (c *AccountController) GetAccounts(ctx context.Context, req *pb.GetAccountsRequest) (*pb.GetAccountsResponse, error) {
	// 1. Get Owner User ID from context using userService
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	// Fetch user via userService
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	ownerUserID := user.ID // Use uint ID

	// 2. Call Service (Service expects uint ownerUserID)
	accounts, err := c.accountService.GetAccounts(ctx, ownerUserID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve accounts: %v", err)
	}

	// 3. Convert and Return Response
	pbAccounts := make([]*pb.Account, 0, len(accounts))
	for i := range accounts {
		pbAccounts = append(pbAccounts, convertAccount(&accounts[i]))
	}

	resp := &pb.GetAccountsResponse{
		Accounts: pbAccounts,
	}
	return resp, nil
}

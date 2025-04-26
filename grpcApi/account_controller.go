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
)

// AccountController handles gRPC requests related to accounts.
type AccountController struct {
	pb.UnimplementedAccountServiceServer // Embed for forward compatibility
	accountService                       services.IAccountService
	userService                          services.IUserService // Added userService dependency
}

// NewAccountController creates a new AccountController.
func NewAccountController(accountService services.IAccountService, userService services.IUserService) *AccountController {
	return &AccountController{
		accountService: accountService,
		userService:    userService, // Store userService
	}
}

// getUserFromContext retrieves the user model based on the auth payload in the context.
func (c *AccountController) getUserFromContext(ctx context.Context) (*models.User, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return nil, status.Error(codes.Unauthenticated, "missing authentication payload")
	}

	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user from token not found: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user: %v", err)
	}
	return user, nil
}

// ListUserAccounts retrieves accounts for the authenticated user.
func (c *AccountController) ListUserAccounts(ctx context.Context, req *pb.ListUserAccountsRequest) (*pb.ListUserAccountsResponse, error) {
	user, err := c.getUserFromContext(ctx)
	if err != nil {
		return nil, err // Error already includes status code
	}

	// Call the service layer with user ID
	summaries, err := c.accountService.GetAccountsByUserID(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve accounts: %v", err)
	}

	// Construct and return the response
	resp := &pb.ListUserAccountsResponse{
		Accounts: summaries,
	}
	return resp, nil
}

// GetAccountDetails retrieves detailed information for a specific account.
func (c *AccountController) GetAccountDetails(ctx context.Context, req *pb.GetAccountDetailsRequest) (*pb.GetAccountDetailsResponse, error) {
	user, err := c.getUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	accountID := uint(req.GetAccountId())

	// Call the service layer with user ID
	details, err := c.accountService.GetAccountDetails(ctx, user.ID, accountID)
	if err != nil {
		if errors.Is(err, services.ErrSvcAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, "account not found")
		} else if errors.Is(err, services.ErrSvcAccountAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "you do not have permission to access this account")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve account details: %v", err)
	}

	// Construct and return the response
	resp := &pb.GetAccountDetailsResponse{
		Account: details,
	}
	return resp, nil
}

// UpdateAccountStatus handles requests to update an account's status.
func (c *AccountController) UpdateAccountStatus(ctx context.Context, req *pb.UpdateAccountStatusRequest) (*pb.UpdateAccountStatusResponse, error) {
	user, err := c.getUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	accountID := uint(req.GetAccountId())
	newStatus := req.GetStatus()
	reason := req.GetReason()

	// Call the service layer with user ID
	updatedAccountModel, err := c.accountService.UpdateAccountStatus(ctx, user.ID, accountID, newStatus, reason)
	if err != nil {
		if errors.Is(err, services.ErrSvcAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, "account not found")
		} else if errors.Is(err, services.ErrSvcAccountAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		} else if errors.Is(err, services.ErrSvcInvalidAccountStatus) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid status provided: %s", newStatus)
		}
		return nil, status.Errorf(codes.Internal, "failed to update account status: %v", err)
	}

	// Convert the updated GORM model back to Protobuf details
	updatedDetails := services.ConvertAccountToProtoDetails(updatedAccountModel)

	resp := &pb.UpdateAccountStatusResponse{
		Account: updatedDetails,
	}
	return resp, nil
}

// UpdateSecuritySettings handles requests to update account security flags.
func (c *AccountController) UpdateSecuritySettings(ctx context.Context, req *pb.UpdateSecuritySettingsRequest) (*pb.UpdateSecuritySettingsResponse, error) {
	user, err := c.getUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	accountID := uint(req.GetAccountId())
	settings := req.GetSettings()

	if settings == nil {
		return nil, status.Error(codes.InvalidArgument, "security settings payload is required")
	}

	// Call the service layer with user ID
	updatedAccountModel, err := c.accountService.UpdateSecuritySettings(ctx, user.ID, accountID, settings)
	if err != nil {
		if errors.Is(err, services.ErrSvcAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, "account not found")
		} else if errors.Is(err, services.ErrSvcAccountAccessDenied) {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to update security settings: %v", err)
	}

	// Convert the updated GORM model back to Protobuf details
	updatedDetails := services.ConvertAccountToProtoDetails(updatedAccountModel)

	resp := &pb.UpdateSecuritySettingsResponse{
		Account: updatedDetails,
	}
	return resp, nil
}

// Note: CreateAccount and RevealPIN gRPC methods are not implemented here yet.
// CreateAccount might be better handled within UserService during signup.
// RevealPIN needs careful security implementation (e.g., password check, OTP).

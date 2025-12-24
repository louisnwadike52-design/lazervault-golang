package grpcApi

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

// getUserFromContext is now a shared helper function in helpers.go

// GetUserAccounts retrieves accounts for the authenticated user.
// Renamed from ListUserAccounts
func (c *AccountController) GetUserAccounts(ctx context.Context, req *pb.GetUserAccountsRequest) (*pb.GetUserAccountsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService) // Use shared helper
	if err != nil {
		return nil, err
	}

	// Extract country code from metadata if present
	var countryCode string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if countryValues := md.Get("x-country-code"); len(countryValues) > 0 {
			countryCode = countryValues[0]
			fmt.Printf("[GetUserAccounts] Country filter requested: %s for user ID: %d\n", countryCode, user.ID)
		} else {
			fmt.Printf("[GetUserAccounts] No country filter for user ID: %d\n", user.ID)
		}
	}

	// Call the renamed service layer method with country filter
	summaries, err := c.accountService.GetUserAccounts(ctx, user.ID, countryCode)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve accounts: %v", err)
	}

	// If no accounts found for this country and country is specified, create default accounts
	if len(summaries) == 0 && countryCode != "" {
		fmt.Printf("[GetUserAccounts] No accounts found for country %s, creating defaults for user ID: %d\n", countryCode, user.ID)

		// Create default accounts for this country
		if err := c.accountService.CreateDefaultAccountsForCountry(ctx, user.ID, countryCode); err != nil {
			fmt.Printf("[GetUserAccounts] Failed to create default accounts for country %s: %v\n", countryCode, err)
			// Don't return error, just log it and return empty list
		} else {
			// Fetch accounts again after creation
			summaries, err = c.accountService.GetUserAccounts(ctx, user.ID, countryCode)
			if err != nil {
				fmt.Printf("[GetUserAccounts] Failed to fetch newly created accounts: %v\n", err)
			} else {
				fmt.Printf("[GetUserAccounts] Successfully created %d accounts for country %s\n", len(summaries), countryCode)
			}
		}
	}

	// Construct and return the renamed response type
	resp := &pb.GetUserAccountsResponse{
		Accounts: summaries,
	}
	return resp, nil
}

// GetAccountDetails retrieves detailed information for a specific account.
func (c *AccountController) GetAccountDetails(ctx context.Context, req *pb.GetAccountDetailsRequest) (*pb.GetAccountDetailsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService) // Use shared helper
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
	user, err := getUserFromContext(ctx, c.userService) // Use shared helper
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
	user, err := getUserFromContext(ctx, c.userService) // Use shared helper
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

// CreateAccount handles the gRPC request to create a new account.
func (c *AccountController) CreateAccount(ctx context.Context, req *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
	user, err := getUserFromContext(ctx, c.userService) // Use shared helper
	if err != nil {
		return nil, err
	}

	// Basic validation (service layer might do more)
	if req.GetAccountType() == "" {
		return nil, status.Error(codes.InvalidArgument, "account_type is required")
	}
	if req.GetCurrency() == "" {
		return nil, status.Error(codes.InvalidArgument, "currency is required")
	}
	// Optional PIN validation (format check is in service layer)
	if req.Pin != nil && len(req.GetPin()) != 4 {
		// Quick check here, though service layer does regex check
		return nil, status.Error(codes.InvalidArgument, "PIN must be exactly 4 digits")
	}

	// Call the service layer
	createdAccountModel, err := c.accountService.CreateAccount(ctx, user.ID, req)
	if err != nil {
		// Map service errors to gRPC status codes
		if errors.Is(err, services.ErrSvcAccountTypeExists) {
			return nil, status.Errorf(codes.AlreadyExists, "account type '%s' already exists for this user", req.GetAccountType())
		} else if errors.Is(err, services.ErrSvcInvalidPINFormat) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		} else if errors.Is(err, services.ErrSvcPINHashingFailed) {
			// Log internal error details if possible
			return nil, status.Error(codes.Internal, "failed to process PIN")
		} else if errors.Is(err, services.ErrSvcAccountCreationFailed) {
			// Log internal error details if possible
			return nil, status.Error(codes.Internal, "failed to create account")
		}
		// Handle other potential errors (e.g., DB connection issues)
		return nil, status.Errorf(codes.Internal, "failed to create account: %v", err)
	}

	// Convert the created GORM model to Protobuf details
	accountDetails := services.ConvertAccountToProtoDetails(createdAccountModel)

	// Construct and return the response
	resp := &pb.CreateAccountResponse{
		Account: accountDetails,
	}

	return resp, nil
}

// Note: RevealPIN gRPC method is not implemented here yet.
// RevealPIN needs careful security implementation (e.g., password check, OTP).

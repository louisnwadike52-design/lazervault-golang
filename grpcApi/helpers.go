package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/services"
	"lazervaultGo/token"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// getUserFromContext retrieves the user model based on the auth payload in the context.
// It requires a UserService instance to fetch user details.
func getUserFromContext(ctx context.Context, userService services.IUserService) (*models.User, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return nil, status.Error(codes.Unauthenticated, "missing authentication payload")
	}

	user, err := userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user from token not found: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user: %v", err)
	}
	return user, nil
}

// getUserAndTokenFromContext retrieves both the user and the access token from context.
// Used by AI services that need to pass the token to external AI microservices.
func getUserAndTokenFromContext(ctx context.Context, userService services.IUserService) (*models.User, string, error) {
	// Get user using existing helper
	user, err := getUserFromContext(ctx, userService)
	if err != nil {
		return nil, "", err
	}

	// Extract access token from context
	accessToken, ok := ctx.Value(middleware.AccessTokenKey).(string)
	if !ok || accessToken == "" {
		return nil, "", status.Error(codes.Unauthenticated, "missing access token")
	}

	return user, accessToken, nil
}

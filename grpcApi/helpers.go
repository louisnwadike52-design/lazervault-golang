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

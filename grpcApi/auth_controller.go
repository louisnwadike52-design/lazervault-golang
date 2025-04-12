package grpcApi

import (
	"context"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AuthController struct {
	pb.UnimplementedAuthServiceServer
	authService *services.AuthService
}

func NewAuthController(authService *services.AuthService) *AuthController {
	return &AuthController{
		authService: authService,
	}
}

func (c *AuthController) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	loginReq := &services.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	}

	result, err := c.authService.Login(loginReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "login failed: %v", err)
	}

	return &pb.LoginResponse{
		User: &pb.User{
			Email:       result.User.Email,
			FirstName:   result.User.FirstName,
			LastName:    result.User.LastName,
			PhoneNumber: result.User.PhoneNumber,
			Role:        result.User.Role,
			Verified:    result.User.Verified,
			CreatedAt:   timestamppb.New(result.User.CreatedAt),
			UpdatedAt:   timestamppb.New(result.User.UpdatedAt),
		},
		Metadata: &pb.Metadata{
			AccessToken: result.Metadata.AccessToken,
			ExpiresAt:   result.Metadata.ExpiresAt,
		},
		Success: result.Success,
		Msg:     result.Msg,
	}, nil
}

package grpcApi

import (
	"context"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	loginReq := &pb.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	}

	result, err := c.authService.Login(loginReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "login failed: %v", err)
	}

	return &pb.LoginResponse{
		User: &pb.User{
			Id:        result.User.Id,
			Email:     result.User.Email,
			FirstName: result.User.FirstName,
			LastName:  result.User.LastName,
		},
		Metadata: &pb.Metadata{
			AccessToken: result.Metadata.AccessToken,
			ExpiresAt:   result.Metadata.ExpiresAt,
		},
		Success: result.Success,
		Msg:     result.Msg,
	}, nil
}

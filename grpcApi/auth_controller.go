package grpcApi

import (
	"context"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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
	// Get client metadata
	md, _ := metadata.FromIncomingContext(ctx)
	userAgent := strings.Join(md.Get("user-agent"), "")
	clientIP := strings.Join(md.Get("x-forwarded-for"), "")

	loginReq := &services.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	}

	result, err := c.authService.Login(loginReq, userAgent, clientIP)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "login failed: %v", err)
	}

	return &pb.LoginResponse{
		User: &pb.User{
			Id:              uint64(result.User.ID),
			Email:           result.User.Email,
			FirstName:       result.User.FirstName,
			LastName:        result.User.LastName,
			PhoneNumber:     result.User.PhoneNumber,
			IsEmailVerified: result.User.Verified,
			CreatedAt:       timestamppb.New(result.User.CreatedAt),
			UpdatedAt:       timestamppb.New(result.User.UpdatedAt),
		},
		Metadata: &pb.Metadata{
			AccessToken:           result.Metadata.AccessToken,
			RefreshToken:          result.Metadata.RefreshToken,
			ExpiresAt:             result.Metadata.AccessTokenExpiresAt.Format(time.RFC3339),
			RefreshTokenExpiresAt: result.Metadata.RefreshTokenExpiresAt.Format(time.RFC3339),
			SessionId:             result.Metadata.SessionID,
		},
		Success: result.Success,
		Msg:     result.Msg,
	}, nil
}

func (c *AuthController) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	refreshReq := &services.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	}

	result, err := c.authService.RefreshToken(refreshReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "token refresh failed: %v", err)
	}

	return &pb.RefreshTokenResponse{
		Metadata: &pb.Metadata{
			AccessToken:           result.AccessToken,
			RefreshToken:          result.RefreshToken,
			ExpiresAt:             result.AccessTokenExpiresAt.Format(time.RFC3339),
			RefreshTokenExpiresAt: result.RefreshTokenExpiresAt.Format(time.RFC3339),
		},
	}, nil
}

func (c *AuthController) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	err := c.authService.Logout(req.SessionId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "logout failed: %v", err)
	}

	return &pb.LogoutResponse{
		Success: true,
		Msg:     "Logged out successfully",
	}, nil
}

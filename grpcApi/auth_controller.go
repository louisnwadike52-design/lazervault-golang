package grpcApi

import (
	"context"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"strings"

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
		Data: &pb.Data{
			User: &pb.User{
				Id:              uint64(result.Data.User.ID),
				Email:           result.Data.User.Email,
				FirstName:       result.Data.User.FirstName,
				LastName:        result.Data.User.LastName,
				PhoneNumber:     result.Data.User.PhoneNumber,
				IsEmailVerified: result.Data.User.Verified,
				CreatedAt:       timestamppb.New(result.Data.User.CreatedAt),
				UpdatedAt:       timestamppb.New(result.Data.User.UpdatedAt),
			},
			Session: &pb.Session{
				Id:                    result.Data.Session.SessionID,
				UserId:                uint64(result.Data.User.ID),
				AccessToken:           result.Data.Session.AccessToken,
				RefreshToken:          result.Data.Session.RefreshToken,
				AccessTokenExpiresAt:  timestamppb.New(result.Data.Session.AccessTokenExpiresAt),
				RefreshTokenExpiresAt: timestamppb.New(result.Data.Session.RefreshTokenExpiresAt),
			},
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
		Data: &pb.Data{
			Session: &pb.Session{
				AccessToken:           result.AccessToken,
				RefreshToken:          result.RefreshToken,
				AccessTokenExpiresAt:  timestamppb.New(result.AccessTokenExpiresAt),
				RefreshTokenExpiresAt: timestamppb.New(result.RefreshTokenExpiresAt),
			},
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

package grpcApi

import (
	"context"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type UserController struct {
	pb.UnimplementedUserServiceServer
	server *Server
}

func NewUserController(server *Server) *UserController {
	return &UserController{
		server: server,
	}
}

func (c *UserController) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	// Get client metadata for session creation
	md, _ := metadata.FromIncomingContext(ctx)
	userAgent := strings.Join(md.Get("user-agent"), "")
	clientIP := strings.Join(md.Get("x-forwarded-for"), "")

	// Log request received
	log.Info().
		Str("email", req.Email).
		Str("first_name", req.FirstName).
		Str("last_name", req.LastName).
		Str("phone_number", req.PhoneNumber).
		Str("role", req.Role).
		Msg("Received CreateUser request")

	// Set default role if not provided
	role := req.Role
	if role == "" {
		role = "user" // Default role for new users
	}

	// Create user model from request
	user := &models.User{
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Email:       req.Email,
		Password:    &req.Password,
		PhoneNumber: req.PhoneNumber,
		Role:        role,
	}

	// Set login passcode if provided
	if req.LoginPasscode != "" {
		if err := user.SetLoginPasscode(req.LoginPasscode); err != nil {
			log.Error().Err(err).Str("email", req.Email).Msg("Failed to set login passcode")
			return c.createErrorResponse(codes.InvalidArgument, fmt.Sprintf("Invalid passcode: %v", err), req.Email)
		}
	}

	userService := services.NewUserService(c.server.db, c.server.config, c.server.tokenMaker)

	// Create user in database
	if err := userService.CreateUser(ctx, user); err != nil {
		// Logging handled in createErrorResponse
		return c.createErrorResponse(codes.InvalidArgument, err.Error(), req.Email)
	}

	// Create session after successful user creation
	// Create access token
	accessToken, accessPayload, err := c.server.tokenMaker.CreateToken(
		user.Email,
		time.Duration(c.server.config.AccessTokenDuration),
	)

	if err != nil {
		log.Error().Err(err).Msg("Failed to create access token")
		return c.createErrorResponse(codes.Internal, "Failed to create session", req.Email)
	}

	// Create refresh token
	refreshToken, refreshPayload, err := c.server.tokenMaker.CreateToken(
		user.Email,
		time.Duration(c.server.config.RefreshTokenDuration),
	)

	if err != nil {
		log.Error().Err(err).Msg("Failed to create refresh token")
		return c.createErrorResponse(codes.Internal, "Failed to create session", req.Email)
	}

	// Create session in database
	session := models.Session{
		ID:           uuid.New().String(),
		UserID:       user.ID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		UserAgent:    userAgent,
		ClientIP:     clientIP,
		IsBlocked:    false,
		ExpiresAt:    refreshPayload.ExpiredAt,
	}

	if err := c.server.db.Create(&session).Error; err != nil {
		log.Error().Err(err).Msg("Failed to create session in database")
		return c.createErrorResponse(codes.Internal, "Failed to create session", req.Email)
	}

	// Create default account for new user
	accountService := services.NewAccountService(c.server.db, c.server.redisWorker.GetDistributor())
	defaultAccountReq := &pb.CreateAccountRequest{
		AccountType: "savings", // Default account type
		Currency:    "GBP",     // Default currency
	}

	_, err = accountService.CreateAccount(ctx, user.ID, defaultAccountReq)
	if err != nil {
		log.Warn().Err(err).Uint("user_id", user.ID).Msg("Failed to create default account for new user")
		// Don't fail user creation if account creation fails - log and continue
	} else {
		log.Info().Uint("user_id", user.ID).Msg("Default account created successfully for new user")
	}

	// Log success
	log.Info().Uint("user_id", user.ID).Str("email", user.Email).Str("session_id", session.ID).Msg("User created successfully with session")

	// Convert to protobuf response
	return &pb.CreateUserResponse{
		Success: true,
		Message: "User created successfully",
		Data: &pb.Data{
			User: &pb.User{
				Id:              uint64(user.ID),
				FirstName:       user.FirstName,
				LastName:        user.LastName,
				Email:           user.Email,
				PhoneNumber:     user.PhoneNumber,
				Role:            user.Role,
				Verified:        user.Verified,
				IsEmailVerified: user.Verified,
				CreatedAt:       timestamppb.New(user.CreatedAt),
				UpdatedAt:       timestamppb.New(user.UpdatedAt),
			},
			Session: &pb.Session{
				Id:                    session.ID,
				UserId:                uint64(user.ID),
				AccessToken:           accessToken,
				RefreshToken:          refreshToken,
				AccessTokenExpiresAt:  timestamppb.New(accessPayload.ExpiredAt),
				RefreshTokenExpiresAt: timestamppb.New(refreshPayload.ExpiredAt),
			},
		},
	}, nil
}

func (c *UserController) createErrorResponse(errorCode codes.Code, errorMessage string, email string) (*pb.CreateUserResponse, error) {
	// Log the error
	log.Error().
		Str("email_attempted", email).
		Str("error", errorMessage).
		Str("grpc_code", errorCode.String()).
		Msg("CreateUser failed")

	fmt.Println("grpc error: ", errorMessage)

	return &pb.CreateUserResponse{
		Success: false,
		Message: errorMessage,
		Data:    nil,
	}, status.Errorf(errorCode, errorMessage)

}

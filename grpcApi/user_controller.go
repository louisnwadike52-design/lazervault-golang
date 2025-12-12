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
		Str("username", req.Username).
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
		Password:    req.Password,
		PhoneNumber: req.PhoneNumber,
		Role:        role,
	}

	// Set username if provided
	if req.Username != "" {
		user.Username = &req.Username
	}

	// Set login passcode if provided
	if req.LoginPasscode != "" {
		if err := user.SetLoginPasscode(req.LoginPasscode); err != nil {
			log.Error().Err(err).Str("email", req.Email).Msg("Failed to set login passcode")
			return c.createErrorResponse(codes.InvalidArgument, fmt.Sprintf("Invalid passcode: %v", err), req.Email)
		}
	}

	userService := services.NewUserService(c.server.db, c.server.config, c.server.tokenMaker)

	// Check if there's an existing partial user with this email
	existingUser, err := userService.GetUserByEmail(ctx, req.Email)
	if err == nil && existingUser != nil && existingUser.IsPartial {
		// Convert partial user to full user
		log.Info().
			Uint("user_id", existingUser.ID).
			Str("email", req.Email).
			Msg("Converting partial user to full user")

		existingUser.FirstName = req.FirstName
		existingUser.LastName = req.LastName
		existingUser.Password = req.Password
		existingUser.PhoneNumber = req.PhoneNumber
		existingUser.Role = role
		existingUser.IsPartial = false

		if req.Username != "" {
			existingUser.Username = &req.Username
		}

		if req.LoginPasscode != "" {
			if err := existingUser.SetLoginPasscode(req.LoginPasscode); err != nil {
				log.Error().Err(err).Str("email", req.Email).Msg("Failed to set login passcode")
				return c.createErrorResponse(codes.InvalidArgument, fmt.Sprintf("Invalid passcode: %v", err), req.Email)
			}
		}

		// Update the user
		if err := c.server.db.Save(existingUser).Error; err != nil {
			log.Error().Err(err).Str("email", req.Email).Msg("Failed to convert partial user")
			return c.createErrorResponse(codes.Internal, "Failed to complete user registration", req.Email)
		}

		user = existingUser

		log.Info().
			Uint("user_id", user.ID).
			Str("email", user.Email).
			Msg("Successfully converted partial user to full user")
	} else {
		// Create new user in database
		if err := userService.CreateUser(ctx, user); err != nil {
			// Logging handled in createErrorResponse
			return c.createErrorResponse(codes.InvalidArgument, err.Error(), req.Email)
		}
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

	// Send email verification after successful user creation
	authService := services.NewAuthService(c.server.db, c.server.config, c.server.tokenMaker, c.server.redisWorker.GetDistributor())
	if err := authService.RequestEmailVerification(ctx, user.Email); err != nil {
		log.Warn().Err(err).Str("email", user.Email).Msg("Failed to send verification email - user created but email not sent")
		// Don't fail the signup - the user is already created, we can retry email later
	} else {
		log.Info().Str("email", user.Email).Msg("Verification email sent successfully")
	}

	// Create 3 default accounts for new user (Personal, Savings, Investment)
	accountService := services.NewAccountService(c.server.db, c.server.redisWorker.GetDistributor())

	accountTypes := []struct {
		accountType string
		currency    string
	}{
		{accountType: "personal", currency: "GBP"},
		{accountType: "savings", currency: "GBP"},
		{accountType: "investment", currency: "GBP"},
	}

	accountsCreated := 0
	for _, accInfo := range accountTypes {
		accountReq := &pb.CreateAccountRequest{
			AccountType: accInfo.accountType,
			Currency:    accInfo.currency,
		}

		_, err = accountService.CreateAccount(ctx, user.ID, accountReq)
		if err != nil {
			log.Warn().
				Err(err).
				Uint("user_id", user.ID).
				Str("account_type", accInfo.accountType).
				Msg("Failed to create account for new user")
			// Don't fail user creation if account creation fails - log and continue
		} else {
			accountsCreated++
			log.Info().
				Uint("user_id", user.ID).
				Str("account_type", accInfo.accountType).
				Msg("Account created successfully for new user")
		}
	}

	log.Info().
		Uint("user_id", user.ID).
		Int("accounts_created", accountsCreated).
		Msg("Default accounts creation completed for new user")

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

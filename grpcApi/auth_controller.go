package grpcApi

import (
	"context"
	"errors"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"lazervaultGo/utils"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AuthController struct {
	pb.UnimplementedAuthServiceServer
	authService services.IAuthService
}

func NewAuthController(authService services.IAuthService) *AuthController {
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

	// Convert User model to pb.User
	pbUser := &pb.User{
		Id:              uint64(result.Data.User.ID),
		Email:           result.Data.User.Email,
		FirstName:       result.Data.User.FirstName,
		LastName:        result.Data.User.LastName,
		PhoneNumber:     result.Data.User.PhoneNumber,
		IsEmailVerified: result.Data.User.Verified,
		CreatedAt:       timestamppb.New(result.Data.User.CreatedAt),
		UpdatedAt:       timestamppb.New(result.Data.User.UpdatedAt),
	}
	// Convert Session model to pb.Session
	pbSession := &pb.Session{
		Id:                    result.Data.Session.SessionID,
		UserId:                uint64(result.Data.User.ID),
		AccessToken:           result.Data.Session.AccessToken,
		RefreshToken:          result.Data.Session.RefreshToken,
		AccessTokenExpiresAt:  timestamppb.New(result.Data.Session.AccessTokenExpiresAt),
		RefreshTokenExpiresAt: timestamppb.New(result.Data.Session.RefreshTokenExpiresAt),
	}

	return &pb.LoginResponse{
		Data: &pb.Data{
			User:    pbUser,
			Session: pbSession,
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

	pbSession := &pb.Session{
		AccessToken:           result.AccessToken,
		RefreshToken:          result.RefreshToken,
		AccessTokenExpiresAt:  timestamppb.New(result.AccessTokenExpiresAt),
		RefreshTokenExpiresAt: timestamppb.New(result.RefreshTokenExpiresAt),
	}

	return &pb.RefreshTokenResponse{
		Data: &pb.Data{
			Session: pbSession,
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

// --- Email Verification Handlers ---

func (c *AuthController) RequestEmailVerification(ctx context.Context, req *pb.RequestEmailVerificationRequest) (*pb.RequestEmailVerificationResponse, error) {
	// 1. Get user details from context (set by auth middleware)
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization payload")
	}

	// We need UserID (uint) and Email (string)
	// The service layer now handles fetching the user ID from the email.
	// userID, err := c.getUserIDFromEmail(ctx, authPayload.Email) // REMOVE THIS
	// if err != nil {
	// 	// Handle ErrUserNotFound appropriately (e.g., Unauthenticated)
	// 	return nil, status.Errorf(codes.Internal, "failed to identify user: %v", err)
	// }

	// 2. Call the service method (passing email directly)
	err := c.authService.RequestEmailVerification(ctx, authPayload.Email)
	if err != nil {
		// Handle specific service errors
		if errors.Is(err, services.ErrUserAlreadyVerified) {
			return nil, status.Errorf(codes.FailedPrecondition, err.Error())
		}
		// Log unexpected errors
		// log.Printf("RequestEmailVerification service error: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to request email verification: %v", err)
	}

	// 3. Return success response
	return &pb.RequestEmailVerificationResponse{
		Success: true,
		Msg:     "Verification email sending process initiated.",
	}, nil
}

func (c *AuthController) VerifyEmail(ctx context.Context, req *pb.VerifyEmailRequest) (*pb.VerifyEmailResponse, error) {
	// 1. Validate request
	verificationCode := strings.TrimSpace(req.GetVerificationCode())
	if verificationCode == "" {
		return nil, status.Errorf(codes.InvalidArgument, "verification_code is required")
	}

	// 2. Call the service method
	err := c.authService.VerifyEmail(ctx, verificationCode)
	if err != nil {
		// Handle specific service errors
		if errors.Is(err, services.ErrVerificationCodeNotFound) {
			return nil, status.Errorf(codes.NotFound, err.Error())
		}
		if errors.Is(err, services.ErrVerificationCodeExpired) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}
		if errors.Is(err, services.ErrVerificationCodeUsed) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}
		if errors.Is(err, services.ErrUserNotFound) {
			// This might indicate an internal issue if the code was valid but user vanished
			return nil, status.Errorf(codes.Internal, "associated user not found")
		}
		// Log unexpected errors
		// log.Printf("VerifyEmail service error: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to verify email: %v", err)
	}

	// 3. Return success response
	return &pb.VerifyEmailResponse{
		Success: true,
		Msg:     "Email verified successfully.",
	}, nil
}

// --- End Email Verification Handlers ---

// --- Password Reset Handlers ---

func (c *AuthController) RequestPasswordReset(ctx context.Context, req *pb.RequestPasswordResetRequest) (*pb.RequestPasswordResetResponse, error) {
	email := strings.TrimSpace(req.GetEmail())
	if !utils.IsValidEmail(email) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid email format")
	}

	err := c.authService.RequestPasswordReset(ctx, email)
	if err != nil {
		// Service layer should ideally handle logging internal errors
		// and preventing user enumeration by always returning nil error here.
		// However, if the service *can* return specific errors safely, handle them:
		// if errors.Is(err, services.SomeSpecificError) { ... }

		// Log unexpected internal errors
		// log.Printf("RequestPasswordReset service error: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to process password reset request")
	}

	// Always return a generic success message to prevent user enumeration
	return &pb.RequestPasswordResetResponse{
		Success: true,
		Msg:     "If an account with that email exists, password reset instructions have been sent.",
	}, nil
}

func (c *AuthController) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) (*pb.ResetPasswordResponse, error) {
	// 1. Validate request
	email := strings.TrimSpace(req.GetEmail())
	token := strings.TrimSpace(req.GetToken())
	newPassword := req.GetNewPassword() // Don't trim password

	if !utils.IsValidEmail(email) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid email format")
	}
	if token == "" {
		return nil, status.Errorf(codes.InvalidArgument, "token is required")
	}
	if newPassword == "" {
		return nil, status.Errorf(codes.InvalidArgument, "new_password is required")
	}
	if len(newPassword) < 8 {
		return nil, status.Errorf(codes.InvalidArgument, services.ErrInvalidNewPasswordFormat.Error())
	}

	// 2. Call service
	err := c.authService.ResetPassword(ctx, email, token, newPassword)
	if err != nil {
		// Handle specific, user-facing errors
		if errors.Is(err, services.ErrInvalidResetToken) ||
			errors.Is(err, services.ErrResetTokenExpired) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid or expired password reset token")
		}
		if errors.Is(err, services.ErrInvalidNewPasswordFormat) {
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}
		// Consider if ErrUserNotFound should be exposed or masked
		if errors.Is(err, services.ErrUserNotFound) {
			// Masking it as InvalidArgument for security
			return nil, status.Errorf(codes.InvalidArgument, "invalid or expired password reset token")
		}

		// Log unexpected internal errors
		// log.Printf("ResetPassword service error: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to reset password: %v", err)
	}

	// 3. Return success
	return &pb.ResetPasswordResponse{
		Success: true,
		Msg:     "Password has been reset successfully.",
	}, nil
}

// --- End Password Reset Handlers ---

// --- PIN Verification Handler ---

func (c *AuthController) VerifyPin(ctx context.Context, req *pb.VerifyPinRequest) (*pb.VerifyPinResponse, error) {
	// 1. Get User Email from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization payload")
	}
	// Use email from the token payload
	email := authPayload.Email
	if email == "" { // Should not happen if token is valid, but check anyway
		return nil, status.Errorf(codes.Unauthenticated, "invalid token payload: missing email")
	}

	// 2. Validate request PIN
	pin := strings.TrimSpace(req.GetPin())
	if pin == "" {
		return nil, status.Errorf(codes.InvalidArgument, "pin is required")
	}
	if len(pin) != 4 { // Assuming 4-digit PIN
		return nil, status.Errorf(codes.InvalidArgument, "PIN must be 4 digits")
	}

	// 3. Call service with email
	err := c.authService.VerifyPin(ctx, email, pin)
	if err != nil {
		// Handle specific service errors
		if errors.Is(err, services.ErrPinNotSet) {
			return nil, status.Errorf(codes.FailedPrecondition, err.Error()) // User needs to set a PIN first
		}
		if errors.Is(err, services.ErrInvalidPin) {
			// Don't reveal PIN is wrong, use generic invalid argument
			return nil, status.Errorf(codes.InvalidArgument, "invalid PIN provided")
		}
		if errors.Is(err, services.ErrUserNotFound) {
			// Should not happen if token was valid and user exists
			return nil, status.Errorf(codes.Unauthenticated, "associated user not found during PIN verification")
		}

		// Log unexpected internal errors
		// log.Printf("VerifyPin service error: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to verify PIN: %v", err)
	}

	// 4. Return success
	return &pb.VerifyPinResponse{
		Success: true,
		Msg:     "PIN verified successfully.",
	}, nil
}

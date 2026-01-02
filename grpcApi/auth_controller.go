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
		// Return generic error message for security (don't reveal if email or password is wrong)
		return nil, status.Errorf(codes.Unauthenticated, "Invalid email or password")
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

func (c *AuthController) LoginWithPasscode(ctx context.Context, req *pb.LoginWithPasscodeRequest) (*pb.LoginResponse, error) {
	// Get client metadata
	md, _ := metadata.FromIncomingContext(ctx)
	userAgent := strings.Join(md.Get("user-agent"), "")
	clientIP := strings.Join(md.Get("x-forwarded-for"), "")

	loginReq := &services.LoginWithPasscodeRequest{
		Email:         req.Email,
		LoginPasscode: req.LoginPasscode,
	}

	result, err := c.authService.LoginWithPasscode(loginReq, userAgent, clientIP)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "passcode login failed: %v", err)
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

func (c *AuthController) RegisterPasscode(ctx context.Context, req *pb.RegisterPasscodeRequest) (*pb.RegisterPasscodeResponse, error) {
	// 1. Get user details from context (set by auth middleware)
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization payload")
	}

	// 2. Validate passcode
	passcode := strings.TrimSpace(req.GetLoginPasscode())
	if passcode == "" {
		return nil, status.Errorf(codes.InvalidArgument, "passcode is required")
	}
	if len(passcode) < 4 || len(passcode) > 6 {
		return nil, status.Errorf(codes.InvalidArgument, "passcode must be between 4 and 6 digits")
	}

	// 3. Call service to register passcode
	err := c.authService.RegisterPasscode(ctx, authPayload.Email, passcode)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to register passcode: %v", err)
	}

	// 4. Return success
	return &pb.RegisterPasscodeResponse{
		Success: true,
		Msg:     "Passcode registered successfully",
	}, nil
}

func (c *AuthController) ChangePasscode(ctx context.Context, req *pb.ChangePasscodeRequest) (*pb.ChangePasscodeResponse, error) {
	// 1. Get user details from context (set by auth middleware)
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization payload")
	}

	// 2. Validate old passcode
	oldPasscode := strings.TrimSpace(req.GetOldPasscode())
	if oldPasscode == "" {
		return nil, status.Errorf(codes.InvalidArgument, "old passcode is required")
	}

	// 3. Validate new passcode
	newPasscode := strings.TrimSpace(req.GetNewPasscode())
	if newPasscode == "" {
		return nil, status.Errorf(codes.InvalidArgument, "new passcode is required")
	}
	if len(newPasscode) < 4 || len(newPasscode) > 6 {
		return nil, status.Errorf(codes.InvalidArgument, "new passcode must be between 4 and 6 digits")
	}

	// 4. Check that old and new passcodes are different
	if oldPasscode == newPasscode {
		return nil, status.Errorf(codes.InvalidArgument, "new passcode must be different from old passcode")
	}

	// 5. Call service to change passcode
	err := c.authService.ChangePasscode(ctx, authPayload.Email, oldPasscode, newPasscode)
	if err != nil {
		// Check if error is due to incorrect old passcode
		if strings.Contains(err.Error(), "incorrect old passcode") {
			return nil, status.Errorf(codes.InvalidArgument, "incorrect old passcode")
		}
		return nil, status.Errorf(codes.Internal, "failed to change passcode: %v", err)
	}

	// 6. Return success
	return &pb.ChangePasscodeResponse{
		Success: true,
		Msg:     "Passcode changed successfully",
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

// --- Email Availability Check Handler ---

func (c *AuthController) CheckEmailAvailability(ctx context.Context, req *pb.CheckEmailAvailabilityRequest) (*pb.CheckEmailAvailabilityResponse, error) {
	// Validate request
	email := strings.TrimSpace(req.GetEmail())
	if email == "" {
		return nil, status.Errorf(codes.InvalidArgument, "email is required")
	}

	if !utils.IsValidEmail(email) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid email format")
	}

	// Check email availability
	available, err := c.authService.CheckEmailAvailability(ctx, email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check email availability: %v", err)
	}

	msg := "Email is available"
	if !available {
		msg = "Email already in use"
	}

	return &pb.CheckEmailAvailabilityResponse{
		Available: available,
		Msg:       msg,
	}, nil
}

// --- End Email Availability Check Handler ---

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

	// Default to SMS if not specified for backward compatibility
	deliveryMethod := "SMS"
	// Note: When we update the proto, we'll get delivery_method from req.GetDeliveryMethod()

	err := c.authService.RequestPasswordReset(ctx, email, deliveryMethod)
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

// --- Password Reset Code Verification Handler ---

func (c *AuthController) VerifyPasswordResetCode(ctx context.Context, req *pb.VerifyPasswordResetCodeRequest) (*pb.VerifyPasswordResetCodeResponse, error) {
	// 1. Validate request
	email := strings.TrimSpace(req.GetEmail())
	code := strings.TrimSpace(req.GetCode())

	if !utils.IsValidEmail(email) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid email format")
	}
	if code == "" {
		return nil, status.Errorf(codes.InvalidArgument, "code is required")
	}
	if len(code) != 6 {
		return nil, status.Errorf(codes.InvalidArgument, "code must be 6 digits")
	}

	// 2. Call service to verify code and get reset token
	resetToken, err := c.authService.VerifyPasswordResetCode(ctx, email, code)
	if err != nil {
		// Generic error message to prevent timing attacks
		return nil, status.Errorf(codes.InvalidArgument, "invalid or expired verification code")
	}

	// 3. Return success with reset token
	return &pb.VerifyPasswordResetCodeResponse{
		Success:    true,
		Msg:        "Code verified successfully. You can now reset your password.",
		ResetToken: resetToken,
	}, nil
}

// --- End Password Reset Code Verification Handler ---

// --- Facial Recognition Handlers ---

func (c *AuthController) LoginWithFace(ctx context.Context, req *pb.LoginWithFaceRequest) (*pb.LoginWithFaceResponse, error) {
	// 1. Validate request
	email := strings.TrimSpace(req.GetEmail())
	if !utils.IsValidEmail(email) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid email format")
	}
	if len(req.GetImageData()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "image_data is required")
	}

	// 2. Get client metadata
	md, _ := metadata.FromIncomingContext(ctx)
	userAgent := strings.Join(md.Get("user-agent"), "")
	clientIP := strings.Join(md.Get("x-forwarded-for"), "")

	// 3. Call service to perform facial recognition login
	result, err := c.authService.LoginWithFace(ctx, email, req.GetImageData(), userAgent, clientIP)
	if err != nil {
		// Return generic error for security
		return nil, status.Errorf(codes.Unauthenticated, "facial recognition login failed")
	}

	// 4. Convert User model to pb.User
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

	// 5. Convert Session model to pb.Session
	pbSession := &pb.Session{
		Id:                    result.Data.Session.SessionID,
		UserId:                uint64(result.Data.User.ID),
		AccessToken:           result.Data.Session.AccessToken,
		RefreshToken:          result.Data.Session.RefreshToken,
		AccessTokenExpiresAt:  timestamppb.New(result.Data.Session.AccessTokenExpiresAt),
		RefreshTokenExpiresAt: timestamppb.New(result.Data.Session.RefreshTokenExpiresAt),
	}

	// 6. Return success response
	return &pb.LoginWithFaceResponse{
		Data: &pb.Data{
			User:    pbUser,
			Session: pbSession,
		},
		Success:    result.Success,
		Msg:        result.Msg,
		Confidence: 0.0, // TODO: Get confidence from facial recognition service
	}, nil
}

func (c *AuthController) CheckFaceRegistration(ctx context.Context, req *pb.CheckFaceRegistrationRequest) (*pb.CheckFaceRegistrationResponse, error) {
	// 1. Get user details from context (set by auth middleware)
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization payload")
	}

	// 2. Call service to check face registration status
	isRegistered, registeredAt, err := c.authService.CheckFaceRegistration(ctx, authPayload.Email)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check face registration status: %v", err)
	}

	// 3. Prepare response
	msg := "Face recognition is not enabled"
	registeredAtStr := ""
	if isRegistered && registeredAt != nil {
		msg = "Face recognition is enabled"
		registeredAtStr = registeredAt.Format("2006-01-02T15:04:05Z07:00") // ISO 8601 format
	}

	// 4. Return response
	return &pb.CheckFaceRegistrationResponse{
		IsRegistered: isRegistered,
		RegisteredAt: registeredAtStr,
		Msg:          msg,
	}, nil
}

// --- End Facial Recognition Handlers ---

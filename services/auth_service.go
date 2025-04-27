package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/tasks"
	"lazervaultGo/token"
	"lazervaultGo/utils"
	"lazervaultGo/validators"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq" // Needed for hashing new password
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// --- Service Errors ---
var (
	ErrVerificationCodeNotFound = errors.New("verification code not found")
	ErrVerificationCodeExpired  = errors.New("verification code has expired")
	ErrVerificationCodeUsed     = errors.New("verification code has already been used")
	ErrUserAlreadyVerified      = errors.New("user email is already verified")
	ErrInvalidNewPasswordFormat = errors.New("new password must be at least 8 characters long")
	ErrPhoneNumberMissing       = errors.New("user does not have a registered phone number")

	// Password Reset (Email/Token) Errors
	ErrInvalidResetToken = errors.New("invalid or expired password reset token")
	ErrResetTokenExpired = errors.New("password reset token has expired")

	// PIN Verification Errors
	ErrPinNotSet  = errors.New("transaction PIN is not set for this user")
	ErrInvalidPin = errors.New("invalid transaction PIN")
)

// --- Interface ---

// IAuthService defines the interface for authentication and related services.
type IAuthService interface {
	Login(req *LoginRequest, userAgent, clientIP string) (*LoginResponse, error)
	RefreshToken(req *RefreshTokenRequest) (*RefreshTokenResponse, error)
	Logout(sessionID string) error
	RequestEmailVerification(ctx context.Context, email string) error
	VerifyEmail(ctx context.Context, verificationCode string) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, email, token, newPassword string) error
	VerifyPin(ctx context.Context, email string, pin string) error
}

// --- Struct ---

type AuthService struct {
	db              *gorm.DB
	config          *configs.Config
	tokenMaker      token.Maker
	taskDistributor tasks.TaskDistributor
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

type Data struct {
	User    models.User `json:"user"`
	Session *Session    `json:"session"`
}

type Session struct {
	AccessToken           string    `json:"access_token"`
	RefreshToken          string    `json:"refresh_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
	SessionID             string    `json:"session_id"`
}

type LoginResponse struct {
	Data    Data   `json:"data"`
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type RefreshTokenResponse struct {
	AccessToken           string    `json:"access_token"`
	RefreshToken          string    `json:"refresh_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

// --- Constructor ---

func NewAuthService(db *gorm.DB, config *configs.Config, tokenMaker token.Maker, taskDistributor tasks.TaskDistributor) IAuthService {
	return &AuthService{
		db:              db,
		config:          config,
		tokenMaker:      tokenMaker,
		taskDistributor: taskDistributor,
	}
}

// --- Methods ---

func (s *AuthService) Login(req *LoginRequest, userAgent, clientIP string) (*LoginResponse, error) {
	// Validate request
	if err := validators.ValidateLoginUser(&models.User{Email: req.Email, Password: &req.Password}); err != nil {
		return nil, err
	}

	user, err := s.getUserByEmail(req.Email)
	if err != nil {
		return nil, err
	}

	if ok, err := user.ComparePassword(req.Password); err != nil || !ok {
		return nil, models.ErrPasswordMismatch
	}

	// Create access token
	accessToken, accessPayload, err := s.tokenMaker.CreateToken(
		user.Email,
		time.Duration(s.config.AccessTokenDuration),
	)
	if err != nil {
		return nil, err
	}

	// Create refresh token
	refreshToken, refreshPayload, err := s.tokenMaker.CreateToken(
		user.Email,
		time.Duration(s.config.RefreshTokenDuration),
	)
	if err != nil {
		return nil, err
	}

	// Create session
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

	if err := s.db.Create(&session).Error; err != nil {
		return nil, err
	}

	return &LoginResponse{
		Data: Data{
			User: *user,
			Session: &Session{
				AccessToken:           accessToken,
				RefreshToken:          refreshToken,
				AccessTokenExpiresAt:  accessPayload.ExpiredAt,
				RefreshTokenExpiresAt: refreshPayload.ExpiredAt,
				SessionID:             session.ID,
			},
		},
		Success: true,
		Msg:     "Login successful",
	}, nil
}

func (s *AuthService) getUserByEmail(email string) (*models.User, error) {
	var user models.User
	err := s.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) getUserByID(userID uint) (*models.User, error) {
	var user models.User
	err := s.db.Where("id = ?", userID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) RefreshToken(req *RefreshTokenRequest) (*RefreshTokenResponse, error) {
	// Verify refresh token
	payload, err := s.tokenMaker.VerifyToken(req.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Get session
	var session models.Session
	err = s.db.Where("refresh_token = ? AND is_blocked = ?", req.RefreshToken, false).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid refresh token")
		}
		return nil, err
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		return nil, errors.New("refresh token expired")
	}

	// Create new access token
	accessToken, accessPayload, err := s.tokenMaker.CreateToken(
		payload.Email,
		time.Duration(s.config.AccessTokenDuration),
	)
	if err != nil {
		return nil, err
	}

	// Create new refresh token
	refreshToken, refreshPayload, err := s.tokenMaker.CreateToken(
		payload.Email,
		time.Duration(s.config.RefreshTokenDuration),
	)
	if err != nil {
		return nil, err
	}

	// Update session
	session.AccessToken = accessToken
	session.RefreshToken = refreshToken
	session.ExpiresAt = refreshPayload.ExpiredAt
	if err := s.db.Save(&session).Error; err != nil {
		return nil, err
	}

	return &RefreshTokenResponse{
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		AccessTokenExpiresAt:  accessPayload.ExpiredAt,
		RefreshTokenExpiresAt: refreshPayload.ExpiredAt,
	}, nil
}

func (s *AuthService) Logout(sessionID string) error {
	result := s.db.Model(&models.Session{}).
		Where("id = ?", sessionID).
		Update("is_blocked", true)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("session not found")
	}

	return nil
}

// generateVerificationCode generates a random, URL-safe string.
func generateVerificationCode(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// RequestEmailVerification generates a verification code and enqueues an email task.
func (s *AuthService) RequestEmailVerification(ctx context.Context, email string) error {
	// Get user by email first
	user, err := s.getUserByEmail(email)
	if err != nil {
		// If user not found by email provided (which should match token), it's an issue.
		return err // ErrUserNotFound or DB error
	}

	// Check if user already verified
	if user.Verified {
		return ErrUserAlreadyVerified
	}

	// Generate code
	code, err := generateVerificationCode(32)
	if err != nil {
		return fmt.Errorf("failed to generate verification code: %w", err)
	}

	// Create verification record
	verification := models.EmailVerification{
		UserID:    user.ID, // Use ID from fetched user
		Email:     email,
		Code:      code,
		ExpiresAt: time.Now().Add(s.config.VerificationCodeDuration),
	}

	if err := s.db.Create(&verification).Error; err != nil {
		return fmt.Errorf("failed to save verification code: %w", err)
	}

	// Enqueue task to send email
	taskPayload := &tasks.PayloadSendVerifyEmail{
		UserID:     user.ID,
		Email:      email,
		Username:   user.FirstName + " " + user.LastName,
		SecretCode: code,
	}
	// Use default options for now
	opts := []asynq.Option{
		asynq.MaxRetry(5),
		asynq.Timeout(1 * time.Minute),
	}

	err = s.taskDistributor.DistributeTaskSendVerifyEmail(ctx, taskPayload, opts...)
	if err != nil {
		// Log error but don't necessarily fail the user request here
		// The code is saved, email sending can be retried or handled later
		fmt.Printf("WARN: Failed to enqueue verification email task for %s: %v\n", email, err)
		// Maybe return a specific warning error if needed?
	}

	return nil
}

// VerifyEmail validates the code and marks the user as verified.
func (s *AuthService) VerifyEmail(ctx context.Context, verificationCode string) error {
	var verification models.EmailVerification

	// Find the verification record by code
	err := s.db.Where("code = ?", verificationCode).First(&verification).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrVerificationCodeNotFound
		}
		return fmt.Errorf("database error finding verification code: %w", err)
	}

	// Check if already used
	if verification.UsedAt != nil {
		return ErrVerificationCodeUsed
	}

	// Check if expired
	if time.Now().After(verification.ExpiresAt) {
		return ErrVerificationCodeExpired
	}

	// --- Transaction: Mark code used and user verified ---
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Mark code as used
		now := time.Now()
		verification.UsedAt = &now
		if err := tx.Save(&verification).Error; err != nil {
			return fmt.Errorf("failed to mark verification code as used: %w", err)
		}

		// Mark user as verified
		result := tx.Model(&models.User{}).
			Where("id = ?", verification.UserID).
			Updates(map[string]interface{}{"verified": true, "verified_at": now})

		if result.Error != nil {
			return fmt.Errorf("failed to update user verification status: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			// This shouldn't happen if the verification record existed, but check anyway
			return ErrUserNotFound
		}

		return nil // Commit transaction
	})

	return txErr // Return result of transaction
}

// --- Password Reset Methods ---

// getUserByEmailOrPhone finds a user by email or phone number.
func (s *AuthService) getUserByEmailOrPhone(identifier string) (*models.User, error) {
	var user models.User
	// Try finding by email first
	err := s.db.Where("email = ?", identifier).First(&user).Error
	if err == nil {
		return &user, nil // Found by email
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database error finding user by email: %w", err) // Other DB error
	}

	// If not found by email, try finding by phone number
	err = s.db.Where("phone_number = ?", identifier).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound // Not found by email or phone
		}
		return nil, fmt.Errorf("database error finding user by phone: %w", err) // Other DB error
	}

	return &user, nil // Found by phone
}

// generateNumericOTP generates a random numeric string of a given length.
func generateNumericOTP(length int) (string, error) {
	const digits = "0123456789"
	max := big.NewInt(int64(len(digits)))
	var otp strings.Builder
	otp.Grow(length)

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		otp.WriteByte(digits[num.Int64()])
	}

	return otp.String(), nil
}

// RequestPasswordReset handles the initiation of the password reset flow.
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) error {
	// 1. Find user by identifier (email or phone)
	user, err := s.getUserByEmailOrPhone(email)
	if err != nil {
		// Don't reveal if user exists for security, but log internally if needed.
		// Return nil to indicate success (or potential success) to the caller.
		if !errors.Is(err, ErrUserNotFound) {
			fmt.Printf("ERROR finding user for password reset (%s): %v\n", email, err)
		}
		// Always return success to prevent user enumeration
		return nil
	}

	// 2. Check if user has a phone number
	if user.PhoneNumber == "" {
		fmt.Printf("WARN: User %d requested password reset but has no phone number.\n", user.ID)
		// Still return success to prevent revealing information
		return nil
	}

	// 3. Generate OTP (e.g., 6 digits)
	otpCode, err := generateNumericOTP(6)
	if err != nil {
		return fmt.Errorf("failed to generate password reset OTP: %w", err)
	}

	// 4. Create OTP record
	otpRecord := models.PasswordResetOTP{
		UserID:     user.ID,
		Identifier: email,   // Store the identifier used (email or phone)
		OTPCode:    otpCode, // Store the plain OTP for now
		ExpiresAt:  time.Now().Add(s.config.PasswordResetOTPDuration),
	}

	if err := s.db.Create(&otpRecord).Error; err != nil {
		return fmt.Errorf("failed to save password reset OTP record: %w", err)
	}

	// 5. Enqueue task to send OTP via Twilio SMS
	taskPayload := &tasks.PayloadSendPasswordResetOTP{
		PhoneNumber: user.PhoneNumber,
		OTPCode:     otpCode,
	}
	opts := []asynq.Option{
		asynq.MaxRetry(3), // Fewer retries for OTP
		asynq.Timeout(30 * time.Second),
	}

	err = s.taskDistributor.DistributeTaskSendPasswordResetOTP(ctx, taskPayload, opts...)
	if err != nil {
		fmt.Printf("WARN: Failed to enqueue password reset OTP task for user %d: %v\n", user.ID, err)
		// Log, but don't fail the request here.
	}

	return nil // Indicate success (task enqueued)
}

// ResetPassword verifies the OTP and updates the user's password.
func (s *AuthService) ResetPassword(ctx context.Context, email, token, newPassword string) error {
	var otpRecord models.PasswordResetOTP

	// 1. Find the latest, unused, unexpired OTP record for the identifier and code
	err := s.db.Where("identifier = ? AND otp_code = ? AND used_at IS NULL AND expires_at > ?",
		email, token, time.Now()).
		Order("created_at DESC").
		First(&otpRecord).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidResetToken
		}
		return fmt.Errorf("database error finding password reset OTP: %w", err)
	}

	// Note: Checks for expiry and used status are implicitly done by the query,
	// but double-checking UsedAt doesn't hurt in case of race conditions if not using transactions properly.
	// if otpRecord.UsedAt != nil { return ErrPasswordResetOTPUsed }
	// if time.Now().After(otpRecord.ExpiresAt) { return ErrPasswordResetOTPExpired }

	// 2. Validate new password format
	if len(newPassword) < 8 { // Use constant or config value for minimum length
		return ErrInvalidNewPasswordFormat
	}

	// 3. Hash the new password
	hashedPassword, err := utils.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	// 4. Transaction: Mark OTP used and update user password
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Mark OTP as used
		now := time.Now()
		otpRecord.UsedAt = &now
		if err := tx.Save(&otpRecord).Error; err != nil {
			return fmt.Errorf("failed to mark password reset OTP as used: %w", err)
		}

		// Update user's password
		result := tx.Model(&models.User{}).
			Where("id = ?", otpRecord.UserID).
			Update("password", hashedPassword)

		if result.Error != nil {
			return fmt.Errorf("failed to update user password: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			// Should not happen if OTP record was valid
			return ErrUserNotFound
		}

		return nil // Commit transaction
	})

	return txErr
}

// VerifyPin checks if the provided PIN matches the user's stored PIN hash.
func (s *AuthService) VerifyPin(ctx context.Context, email string, pin string) error {
	// 1. Get User by Email
	user, err := s.getUserByEmail(email)
	if err != nil {
		// Propagate ErrUserNotFound or other DB errors
		return err
	}

	// 2. Check if PIN is set
	if user.TransactionPin == nil || *user.TransactionPin == "" {
		return ErrPinNotSet
	}

	// 3. Compare provided PIN with the stored hash using bcrypt
	err = bcrypt.CompareHashAndPassword([]byte(*user.TransactionPin), []byte(pin))
	if err != nil {
		// bcrypt.CompareHashAndPassword returns specific error on mismatch
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidPin
		}
		// Log other potential errors from comparison
		fmt.Printf("ERROR comparing PIN hash for user %d: %v\n", user.ID, err)
		return fmt.Errorf("internal error during PIN verification: %w", err)
	}

	// 4. PIN is valid
	return nil
}

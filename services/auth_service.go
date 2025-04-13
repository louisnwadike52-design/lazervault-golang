package services

import (
	"errors"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/token"
	"lazervaultGo/validators"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuthService struct {
	db         *gorm.DB
	config     *configs.Config
	tokenMaker token.Maker
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

func NewAuthService(db *gorm.DB, config *configs.Config, tokenMaker token.Maker) *AuthService {
	return &AuthService{
		db:         db,
		config:     config,
		tokenMaker: tokenMaker,
	}
}

func (s *AuthService) Login(req *LoginRequest, userAgent, clientIP string) (*LoginResponse, error) {
	// Validate request
	if err := validators.ValidateLoginUser(&models.User{Email: req.Email, Password: req.Password}); err != nil {
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
			User: user,
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

func (s *AuthService) getUserByEmail(email string) (models.User, error) {
	var user models.User
	err := s.db.Where("email = ?", email).First(&user).Error
	return user, err
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

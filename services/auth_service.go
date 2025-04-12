package services

import (
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/token"
	"lazervaultGo/validators"
	"time"

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

type Metadata struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
}

type LoginResponse struct {
	User     models.User `json:"user"`
	Metadata Metadata    `json:"metadata"`
	Success  bool        `json:"success"`
	Msg      string      `json:"msg"`
}

func NewAuthService(db *gorm.DB, config *configs.Config, tokenMaker token.Maker) *AuthService {
	return &AuthService{
		db:         db,
		config:     config,
		tokenMaker: tokenMaker,
	}
}

func (s *AuthService) Login(req *LoginRequest) (*LoginResponse, error) {
	// validate request
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

	accessToken, payload, err := s.tokenMaker.CreateToken(
		user.Email,
		time.Hour*24, // 24 hour token
	)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		User: models.User{
			Email:       user.Email,
			FirstName:   user.FirstName,
			LastName:    user.LastName,
			PhoneNumber: user.PhoneNumber,
			Role:        user.Role,
			Verified:    user.Verified,
			CreatedAt:   user.CreatedAt,
			UpdatedAt:   user.UpdatedAt,
		},
		Metadata: Metadata{
			AccessToken: accessToken,
			ExpiresAt:   payload.ExpiredAt.Format(time.RFC3339),
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

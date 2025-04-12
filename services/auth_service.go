package services

import (
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/pb"
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

func NewAuthService(db *gorm.DB, config *configs.Config, tokenMaker token.Maker) *AuthService {
	return &AuthService{
		db:         db,
		config:     config,
		tokenMaker: tokenMaker,
	}
}

func (s *AuthService) Login(req *pb.LoginRequest) (*pb.LoginResponse, error) {
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

	return &pb.LoginResponse{
		User: &pb.User{
			Id:        uint64(user.ID),
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
		},
		Metadata: &pb.Metadata{
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

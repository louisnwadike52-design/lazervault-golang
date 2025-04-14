package services

import (
	"context"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/token"
	"lazervaultGo/validators"

	"gorm.io/gorm"
)

type UserService struct {
	db         *gorm.DB
	config     *configs.Config
	tokenMaker token.Maker
}

func NewUserService(db *gorm.DB, config *configs.Config, tokenMaker token.Maker) *UserService {
	return &UserService{
		db:         db,
		config:     config,
		tokenMaker: tokenMaker,
	}
}

func (s *UserService) CreateUser(ctx context.Context, user *models.User) error {
	err := validators.ValidateUser(user)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(user).Error
}

func (s *UserService) GetUser(ctx context.Context, userID uint) (*models.User, error) {
	return models.User{}.FindById(s.db.WithContext(ctx), userID)
}

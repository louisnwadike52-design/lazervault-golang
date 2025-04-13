package services

import (
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

func (s *UserService) CreateUser(user *models.User) error {
	err := validators.ValidateUser(user)
	if err != nil {
		return err
	}
	return s.db.Create(user).Error
}

func (s *UserService) GetUser(user *models.User) error {
	return s.db.Where("id = ?", user.ID).First(user).Error
}

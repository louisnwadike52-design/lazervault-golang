package services

import (
	"context"
	"errors"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/token"
	"lazervaultGo/validators"

	"gorm.io/gorm"
)

var (
	ErrUserNotFound = errors.New("user not found")
)

// IUserService defines the interface for user operations
type IUserService interface {
	CreateUser(ctx context.Context, user *models.User) error
	GetUserByID(ctx context.Context, userID uint) (*models.User, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
}

type UserService struct {
	db         *gorm.DB
	config     *configs.Config
	tokenMaker token.Maker
}

// Ensure UserService implements IUserService
var _ IUserService = (*UserService)(nil)

func NewUserService(db *gorm.DB, config *configs.Config, tokenMaker token.Maker) IUserService {
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
	// TODO: Add password hashing here before saving
	return s.db.WithContext(ctx).Create(user).Error
}

// GetUserByID retrieves a user by their primary key ID
func (s *UserService) GetUserByID(ctx context.Context, userID uint) (*models.User, error) {
	var user models.User
	err := s.db.WithContext(ctx).First(&user, userID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by their email address
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := s.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

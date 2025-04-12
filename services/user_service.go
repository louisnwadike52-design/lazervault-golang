package services

import (
	"lazervaultGo/models"
	"lazervaultGo/validators"

	"gorm.io/gorm"
)

func CreateUser(db *gorm.DB, user *models.User) error {
	err := validators.ValidateUser(user)
	if err != nil {
		return err
	}
	return db.Create(user).Error
}

func GetUser(db *gorm.DB, user *models.User) error {
	return db.Where("id = ?", user.ID).First(user).Error
}

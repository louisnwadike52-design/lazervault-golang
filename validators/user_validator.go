package validators

import (
	"lazervaultGo/models"

	"gorm.io/gorm"
)

func ValidateUser(user *models.User, db *gorm.DB) error {
	if user.FirstName == "" {
		return models.ErrFirstNameRequired
	}
	if user.LastName == "" {
		return models.ErrLastNameRequired
	}
	if user.Email == "" {
		return models.ErrEmailRequired
	}
	if user.Password == "" {
		return models.ErrPasswordRequired
	}
	if user.PhoneNumber == "" {
		return models.ErrPhoneNumberRequired
	}
	if user.Role == "" {
		return models.ErrRoleRequired
	}
	return nil
}

// func ValidateUser(err error, ctx *gin.Context) {
// 	switch err {
// 	case models.ErrDuplicateEmail:
// 		ctx.JSON(http.StatusConflict, gin.H{
// 			"success": false,
// 			"message": "Email already exists",
// 		})
// 	case models.ErrDuplicatePhone:
// 		ctx.JSON(http.StatusConflict, gin.H{
// 			"success": false,
// 			"message": "Phone number already exists",
// 		})
// 	case models.ErrInvalidNameLength:
// 		ctx.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Name must be between 2 and 255 characters",
// 		})
// 	case models.ErrInvalidEmail:
// 		ctx.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Invalid email format",
// 		})
// 	case models.ErrInvalidPassword:
// 		ctx.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Password must be at least 8 characters",
// 		})
// 	case models.ErrInvalidPhone:
// 		ctx.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Invalid phone number format",
// 		})
// 	case models.ErrInvalidRole:
// 		ctx.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Role must be either 'admin' or 'user'",
// 		})
// 	case gorm.ErrRecordNotFound:
// 		ctx.JSON(http.StatusNotFound, gin.H{
// 			"success": false,
// 			"message": "Record not found",
// 		})
// 	default:
// 		ctx.JSON(http.StatusInternalServerError, gin.H{
// 			"success": false,
// 			"message": "Failed to create user",
// 			"error":   err.Error(),
// 		})
// 	}

// }

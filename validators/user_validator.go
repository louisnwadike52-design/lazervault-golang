package validators

import (
	"lazervaultGo/models"
	"lazervaultGo/utils"
)

func ValidateUser(user *models.User) error {
	if user.FirstName == "" {
		return models.ErrFirstNameRequired
	}
	if user.LastName == "" {
		return models.ErrLastNameRequired
	}
	if user.Email == "" || !utils.IsValidEmail(user.Email) {
		return models.ErrInvalidEmail
	}
	if user.Password != "" && !utils.IsValidPassword(user.Password) {
		return models.ErrInvalidPassword
	}
	if user.PhoneNumber == "" || !utils.IsValidPhoneNumber(user.PhoneNumber) {
		return models.ErrInvalidPhone
	}
	if user.Role == "" {
		return models.ErrRoleRequired
	}
	return nil
}

func ValidateLoginUser(user *models.User) error {
	if user.Email == "" || !utils.IsValidEmail(user.Email) {
		return models.ErrInvalidEmail
	}
	// Don't validate password complexity during login - only check it's not empty
	if user.Password == "" {
		return models.ErrInvalidPassword
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

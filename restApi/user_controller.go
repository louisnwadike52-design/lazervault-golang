package restApi

import (
	"lazervaultGo/models"
	"lazervaultGo/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserController struct {
	server *Server
}

func (c *UserController) CreateUser(ctx *gin.Context) {
	var user models.User
	if err := ctx.ShouldBindJSON(&user); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	// Create user in database
	if err := services.CreateUser(c.server.DB, &user); err != nil {
		handleUserError(ctx, err)
		return
	}

	// Success response
	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "User created successfully",
		"data":    user.ToJson(),
	})
}

func handleUserError(ctx *gin.Context, err error) {
	switch err {
	case models.ErrDuplicateEmail:
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Email already exists",
		})
	case models.ErrDuplicatePhone:
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Phone number already exists",
		})
	default:
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create user",
		})
	case models.ErrInvalidNameLength:
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid name length",
		})
	case models.ErrInvalidEmail:
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid email",
		})
	}
}

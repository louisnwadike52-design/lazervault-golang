package restApi

import (
	"lazervaultGo/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AuthController struct {
	authService *services.AuthService
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

func (c *AuthController) Login(ctx *gin.Context) {
	var loginRequest LoginRequest
	if err := ctx.ShouldBindJSON(&loginRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	userAgent := ctx.Request.UserAgent()
	clientIP := ctx.ClientIP()

	loginResponse, err := c.authService.Login(&services.LoginRequest{
		Email:    loginRequest.Email,
		Password: loginRequest.Password,
	}, userAgent, clientIP)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    loginResponse,
	})
}

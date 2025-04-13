package restApi

import (
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/services"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserController struct {
	server      *Server
	userService *services.UserService
}

func (c *UserController) CreateUser(ctx *gin.Context) {
	var user models.User
	if err := ctx.ShouldBindJSON(&user); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"msg":     "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	// Create user in database
	if err := c.userService.CreateUser(&user); err != nil {
		handleUserError(ctx, err)
		return
	}

	// Success response
	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"msg":     "User created successfully",
		"data":    user.ToJson(),
	})
}

func handleUserError(ctx *gin.Context, err error) {
	fmt.Println("Error: ", err)
	log.Println("Error: ", err)
	ctx.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"msg":     err.Error(),
	})
}

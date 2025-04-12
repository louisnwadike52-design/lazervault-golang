package restApi

import (
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/restApi/middleware"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	apiVersion = "v1"
)

type Server struct {
	DB         *gorm.DB
	Config     *configs.Config
	TokenMaker token.Maker
	restServer *GinServer
}

type GinServer struct {
	router *gin.Engine
}

type IServerMethods interface {
	Serve() (server *Server, err error)
	ErrorResponse(err error) gin.H
}

func (s *Server) Serve() (server *Server, err error) {

	gin.SetMode(gin.DebugMode)
	s.restServer = &GinServer{
		router: gin.Default(),
	}
	s.restServer.router.Use(middleware.AuthMiddleware(s.TokenMaker))

	s.setupRouter()

	s.Start(":8080")
	log.Println("Server started on port 8080")
	return s, nil
}

func (s *Server) setupRouter() {
	userController := UserController{
		server: s,
	}
	authService := services.NewAuthService(s.DB, s.Config, s.TokenMaker)
	authController := AuthController{
		authService: authService,
	}
	s.restServer.router.POST(fmt.Sprintf("/%s/users", apiVersion), userController.CreateUser)
	s.restServer.router.POST(fmt.Sprintf("/%s/auth/login", apiVersion), authController.Login)
}

// Start runs the HTTP server on a specific address.
func (s *Server) Start(address string) error {
	return s.restServer.router.Run(address)
}

func (Server) ErrorResponse(err error) gin.H {
	return gin.H{"success": false, "error": err}
}

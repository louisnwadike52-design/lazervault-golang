package api

import (
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Server struct {
	DB         *gorm.DB
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

	s.setupRouter()

	s.Start(":8080")
	log.Println("Server started on port 8080")
	return s, nil
}

func (s *Server) setupRouter() {
	userController := UserController{
		server: s,
	}
	s.restServer.router.POST("/users", userController.CreateUser)

}

// Start runs the HTTP server on a specific address.
func (s *Server) Start(address string) error {
	return s.restServer.router.Run(address)
}

func (Server) ErrorResponse(err error) gin.H {
	return gin.H{"success": false, "error": err}
}

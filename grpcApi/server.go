package grpcApi

import (
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"net"

	"google.golang.org/grpc"
	"gorm.io/gorm"
)

type GRPCServer struct {
	pb.UnimplementedUserServiceServer
	pb.UnimplementedAuthServiceServer
	db         *gorm.DB
	config     *configs.Config
	tokenMaker token.Maker
}

func NewGRPCServer(db *gorm.DB, config *configs.Config, tokenMaker token.Maker) *GRPCServer {
	return &GRPCServer{
		db:         db,
		config:     config,
		tokenMaker: tokenMaker,
	}
}

func RunGRPCServer(db *gorm.DB, tokenMaker token.Maker, config *configs.Config) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", config.GRPCServerPort))
	if err != nil {
		return err
	}

	server := grpc.NewServer(
		grpc.UnaryInterceptor(middleware.AuthInterceptor(tokenMaker)),
	)

	authController := NewAuthController(services.NewAuthService(db, config, tokenMaker))

	pb.RegisterAuthServiceServer(server, authController)
	pb.RegisterUserServiceServer(server, NewGRPCServer(db, config, tokenMaker))

	return server.Serve(listener)
}

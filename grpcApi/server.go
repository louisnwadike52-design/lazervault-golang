package grpcApi

import (
	"lazervaultGo/pb"
	"net"

	"google.golang.org/grpc"
	"gorm.io/gorm"
)

type GRPCServer struct {
	pb.UnimplementedUserServiceServer
	db *gorm.DB
}

func NewGRPCServer(db *gorm.DB) *GRPCServer {
	return &GRPCServer{
		db: db,
	}
}

func RunGRPCServer(db *gorm.DB, listener net.Listener) error {
	server := grpc.NewServer()
	pb.RegisterUserServiceServer(server, NewGRPCServer(db))
	return server.Serve(listener)
}

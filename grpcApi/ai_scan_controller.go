package grpcApi

import (
	"lazervaultGo/configs"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// RegisterAiScanService registers the AI scan service with the gRPC server
func RegisterAiScanService(grpcServer *grpc.Server, db *gorm.DB, config *configs.Config) {
	aiScanService := services.NewAiScanService(db, config)
	pb.RegisterAiScanServiceServer(grpcServer, aiScanService)
}

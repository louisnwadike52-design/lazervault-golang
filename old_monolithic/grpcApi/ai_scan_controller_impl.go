package grpcApi

import (
	"lazervaultGo/pb"
	"lazervaultGo/services"
)

// AiScanController wraps the AI scan service
type AiScanController struct {
	pb.UnimplementedAiScanServiceServer
	service *services.AiScanService
}

// NewAiScanController creates a new AI scan controller
func NewAiScanController(service *services.AiScanService) *AiScanController {
	return &AiScanController{
		service: service,
	}
}

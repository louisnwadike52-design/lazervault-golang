package grpcApi

import (
	"context"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	// "github.com/rs/zerolog/log"
)

// VoiceSessionController handles gRPC requests for voice sessions.
type VoiceSessionController struct {
	pb.UnimplementedVoiceSessionServiceServer
	voiceSessionService services.IVoiceSessionService
	userService         services.IUserService
}

// NewVoiceSessionController creates a new VoiceSessionController.
func NewVoiceSessionController(
	voiceSessionService services.IVoiceSessionService,
	userService services.IUserService,
) *VoiceSessionController {
	return &VoiceSessionController{
		voiceSessionService: voiceSessionService,
		userService:         userService,
	}
}

// StartVoiceSession handles the gRPC request to start a voice session.
func (c *VoiceSessionController) StartVoiceSession(ctx context.Context, req *pb.StartVoiceSessionRequest) (*pb.StartVoiceSessionResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	appTokenString, ok := ctx.Value(middleware.AccessTokenKey).(string)
	if !ok || appTokenString == "" {
		return nil, status.Errorf(codes.Unauthenticated, "application access token not found in context")
	}

	resp, err := c.voiceSessionService.StartSession(ctx, user, appTokenString)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to start voice session: %v", err)
	}

	return resp, nil
}

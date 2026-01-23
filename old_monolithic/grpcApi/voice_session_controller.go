package grpcApi

import (
	"context"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"log"

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

	// Log service name for debugging
	log.Printf("INFO: Starting voice session for user %d with service: %s", user.ID, req.ServiceName)

	resp, err := c.voiceSessionService.StartSession(ctx, user, appTokenString, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to start voice session: %v", err)
	}

	return resp, nil
}

// ProcessVoiceNote handles the gRPC request to process a voice note.
func (c *VoiceSessionController) ProcessVoiceNote(ctx context.Context, req *pb.ProcessVoiceNoteRequest) (*pb.ProcessVoiceNoteResponse, error) {
	// Extract authentication payload from context
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err // Error already includes status code
	}

	userID := user.ID
	if userID == 0 {
		log.Println("WARN: VoiceSessionController: Invalid UserID (0) in auth payload")
		return nil, status.Error(codes.Unauthenticated, "invalid user ID in token")
	}

	// Extract access token from context
	appTokenString, ok := ctx.Value(middleware.AccessTokenKey).(string)
	if !ok || appTokenString == "" {
		return nil, status.Errorf(codes.Unauthenticated, "application access token not found in context")
	}

	log.Printf("INFO: VoiceSessionController: Processing voice note for User ID: %d", userID)

	// This method should not be called directly anymore - use the HTTP multipart handler instead
	return nil, status.Error(codes.Unimplemented, "use the multipart upload endpoint /v1/voice/note/upload instead")
}

// ProcessVoiceNoteMultipart handles multipart form uploads (called from HTTP handler)
func (c *VoiceSessionController) ProcessVoiceNoteMultipart(ctx context.Context, userID uint, accessToken string, audioContent []byte, filename string, contentType string, req *pb.ProcessVoiceNoteRequest) (*pb.ProcessVoiceNoteResponse, error) {
	log.Printf("INFO: VoiceSessionController: Processing voice note multipart for User ID: %d", userID)

	// Delegate to the service, passing the extracted userID and access token
	return c.voiceSessionService.ProcessVoiceNote(ctx, userID, accessToken, audioContent, filename, contentType, req)
}

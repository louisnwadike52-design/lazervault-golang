package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/token"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"gorm.io/gorm"
)

// IVoiceSessionService defines the interface for voice session operations.
type IVoiceSessionService interface {
	StartSession(ctx context.Context, authenticatedUser *models.User, appTokenString string) (*pb.StartVoiceSessionResponse, error)
}

// VoiceSessionService implements IVoiceSessionService.
type VoiceSessionService struct {
	db           *gorm.DB
	config       *configs.Config
	tokenMaker   token.Maker
	lkRoomClient *lksdk.RoomServiceClient
}

// NewVoiceSessionService creates a new VoiceSessionService.
func NewVoiceSessionService(db *gorm.DB, config *configs.Config, tokenMaker token.Maker) IVoiceSessionService {
	lkRoomClient := lksdk.NewRoomServiceClient(config.LiveKitHost, config.LiveKitAPIKey, config.LiveKitAPISecret)

	return &VoiceSessionService{
		db:           db,
		config:       config,
		tokenMaker:   tokenMaker,
		lkRoomClient: lkRoomClient,
	}
}

// StartSession handles the logic to start a voice session.
func (s *VoiceSessionService) StartSession(ctx context.Context, authenticatedUser *models.User, appTokenString string) (*pb.StartVoiceSessionResponse, error) {
	randomSuffixBytes := make([]byte, 8)
	if _, err := rand.Read(randomSuffixBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random suffix for room: %w", err)
	}
	roomName := fmt.Sprintf("user-%d-%s", authenticatedUser.ID, hex.EncodeToString(randomSuffixBytes))
	participantIdentity := fmt.Sprint(authenticatedUser.ID)

	// Prepare metadata that will be used for both room creation and participant token
	agentContextMetadataMap := map[string]string{
		"access_token": appTokenString,
		"user_id":      participantIdentity,
		"email":        authenticatedUser.Email,
	}
	agentContextMetadataBytes, err := json.Marshal(agentContextMetadataMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal agent context metadata: %w", err)
	}
	agentContextMetadataString := string(agentContextMetadataBytes)

	// Create the room with metadata included
	_, err = s.lkRoomClient.CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:     roomName,
		Metadata: agentContextMetadataString, // Set metadata at creation
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create LiveKit room '%s' with metadata: %w", roomName, err)
	}

	at := auth.NewAccessToken(s.config.LiveKitAPIKey, s.config.LiveKitAPISecret)
	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     roomName,
	}
	grant.SetCanPublish(true)
	grant.SetCanSubscribe(true)
	grant.SetCanPublishData(true)

	clientToken, err := at.SetIdentity(participantIdentity).
		SetName(fmt.Sprintf("%s %s", authenticatedUser.FirstName, authenticatedUser.LastName)).
		SetMetadata(agentContextMetadataString). // Use the same metadata string for participant
		SetVideoGrant(grant).
		SetValidFor(1 * time.Hour).
		ToJWT()
	if err != nil {
		return nil, fmt.Errorf("failed to generate LiveKit token for client: %w", err)
	}

	agentParticipantIdentity := "voice-agent-for-" + roomName

	return &pb.StartVoiceSessionResponse{
		RoomName:     roomName,
		LivekitToken: clientToken,
		AgentId:      agentParticipantIdentity,
	}, nil
}

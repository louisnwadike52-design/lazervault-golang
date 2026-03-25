package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	pb "lazervaultGo/pb"
	"github.com/rs/zerolog/log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// VoiceSessionServiceProxy proxies VoiceSessionService gRPC requests to the Python voice-agent-gateway via HTTP
type VoiceSessionServiceProxy struct {
	pb.UnimplementedVoiceSessionServiceServer
	voiceGatewayURL string
	httpClient      *http.Client
}

// NewVoiceSessionServiceProxy creates a new VoiceSessionServiceProxy
func NewVoiceSessionServiceProxy(voiceGatewayURL string) *VoiceSessionServiceProxy {
	return &VoiceSessionServiceProxy{
		voiceGatewayURL: voiceGatewayURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// voiceSessionRequest is the JSON payload sent to the Python voice-agent-gateway
type voiceSessionRequest struct {
	ServiceName     string `json:"serviceName,omitempty"`
	Language        string `json:"language,omitempty"`
	VoicePreference string `json:"voice_preference,omitempty"`
}

// voiceSessionResponse is the JSON response from the Python voice-agent-gateway
type voiceSessionResponse struct {
	RoomName     string `json:"roomName"`
	LivekitToken string `json:"livekitToken"`
	SessionID    string `json:"sessionId"`
	AgentURL     string `json:"agentUrl"`
	Error        string `json:"error,omitempty"`
}

// StartVoiceSession creates a LiveKit room and returns connection credentials
func (p *VoiceSessionServiceProxy) StartVoiceSession(ctx context.Context, req *pb.StartVoiceSessionRequest) (*pb.StartVoiceSessionResponse, error) {
	// Extract access token from gRPC metadata
	accessToken := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("authorization"); len(vals) > 0 {
			accessToken = vals[0]
		}
	}
	if accessToken == "" {
		// Try getting from auth interceptor
		if userID, err := authinterceptor.GetUserID(ctx); err == nil && userID != "" {
			// We have the user but still need the token for forwarding
			log.Debug().Str("user_id", userID).Msg("User authenticated but no bearer token in metadata")
		}
		return nil, status.Error(codes.Unauthenticated, "missing authorization token")
	}

	// Build request body — forward all fields to Python gateway
	body := voiceSessionRequest{
		ServiceName:     req.ServiceName,
		Language:        req.Language,
		VoicePreference: req.VoicePreference,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal request: %v", err)
	}

	// Forward to Python voice-agent-gateway
	url := fmt.Sprintf("%s/voice/session/start", p.voiceGatewayURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create request: %v", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", accessToken)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", url).Msg("Failed to call voice-agent-gateway")
		return nil, status.Errorf(codes.Unavailable, "voice agent gateway unavailable: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status", resp.StatusCode).Str("body", string(respBody)).Msg("Voice agent gateway error")
		return nil, status.Errorf(codes.Internal, "voice session creation failed: %s", string(respBody))
	}

	var sessionResp voiceSessionResponse
	if err := json.Unmarshal(respBody, &sessionResp); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to parse response: %v", err)
	}

	return &pb.StartVoiceSessionResponse{
		RoomName:     sessionResp.RoomName,
		LivekitToken: sessionResp.LivekitToken,
		AgentUrl:     sessionResp.AgentURL,
	}, nil
}

// ProcessVoiceNote handles voice note processing (not yet implemented)
func (p *VoiceSessionServiceProxy) ProcessVoiceNote(ctx context.Context, req *pb.ProcessVoiceNoteRequest) (*pb.ProcessVoiceNoteResponse, error) {
	return nil, status.Error(codes.Unimplemented, "voice note processing not yet implemented")
}

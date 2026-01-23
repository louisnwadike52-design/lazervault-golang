package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/token"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// ServicePortMapping maps service names to voice agent ports
var ServicePortMapping = map[string]int32{
	// Category 1: Core Banking (Ports 3001-3009)
	"auth":        3001,
	"accounts":    3002,
	"deposits":    3003,
	"withdrawals": 3004,
	"transfers":   3005,
	"recipients":  3006,
	"user":        3007,
	"cards":       3008,

	// Category 2: Investments (Ports 3010-3014)
	"stocks":    3010,
	"crypto":    3011,
	"portfolio": 3012,
	"analytics": 3013,
	"exchange":  3014,

	// Category 3: Payments (Ports 3015-3022)
	"invoices":        3015,
	"invoice-payment": 3016,
	"tagpay":          3017,
	"barcode":         3018,
	"electricity":     3019,
	"airtime":         3020,
	"bills":           3021,
	"expenses":        3022,

	// Category 4: Financial Products (Ports 3023-3030)
	"autosave":  3023,
	"lockfunds": 3024,
	"crowdfund": 3025,
	"groups":    3026,
	"insurance": 3027,
	"loans":     3028,
	"referrals": 3029,
	"giftcards": 3030,

	// Category 5: User Services (Ports 3031-3037)
	"contact-sync":  3031,
	"ai-chat":       3032,
	"ai-scan":       3033,
	"statements":    3034,
	"statistics":    3035,
	"support":       3036,
	"voice-session": 3037,
}

// IVoiceSessionService defines the interface for voice session operations.
type IVoiceSessionService interface {
	StartSession(ctx context.Context, authenticatedUser *models.User, appTokenString string, req *pb.StartVoiceSessionRequest) (*pb.StartVoiceSessionResponse, error)
	ProcessVoiceNote(ctx context.Context, userID uint, accessToken string, audioContent []byte, filename string, contentType string, req *pb.ProcessVoiceNoteRequest) (*pb.ProcessVoiceNoteResponse, error)
}

// VoiceSessionService implements IVoiceSessionService.
type VoiceSessionService struct {
	db            *gorm.DB
	config        *configs.Config
	tokenMaker    token.Maker
	lkRoomClient  *lksdk.RoomServiceClient
	httpClient    *http.Client
	aiChatService *AIChatService
}

// NewVoiceSessionService creates a new VoiceSessionService.
func NewVoiceSessionService(db *gorm.DB, config *configs.Config, tokenMaker token.Maker, aiChatService *AIChatService) IVoiceSessionService {
	lkRoomClient := lksdk.NewRoomServiceClient(config.LiveKitHost, config.LiveKitAPIKey, config.LiveKitAPISecret)

	return &VoiceSessionService{
		db:           db,
		config:       config,
		tokenMaker:   tokenMaker,
		lkRoomClient: lkRoomClient,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Longer timeout for voice processing
		},
		aiChatService: aiChatService,
	}
}

// StartSession handles the logic to start a voice session.
func (s *VoiceSessionService) StartSession(ctx context.Context, authenticatedUser *models.User, appTokenString string, req *pb.StartVoiceSessionRequest) (*pb.StartVoiceSessionResponse, error) {
	// Get service name from request, default to "general" if not provided
	serviceName := req.ServiceName
	if serviceName == "" {
		serviceName = "general"
	}

	// Get port for the service
	agentPort, ok := ServicePortMapping[serviceName]
	if !ok {
		return nil, fmt.Errorf("unknown service name: %s", serviceName)
	}

	// Build agent URL (use localhost for local development, or configured host)
	agentHost := os.Getenv("VOICE_AGENT_HOST")
	if agentHost == "" {
		agentHost = "localhost"
	}
	agentURL := fmt.Sprintf("http://%s:%d", agentHost, agentPort)

	randomSuffixBytes := make([]byte, 8)
	if _, err := rand.Read(randomSuffixBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random suffix for room: %w", err)
	}
	roomName := fmt.Sprintf("user-%d-%s-%s", authenticatedUser.ID, serviceName, hex.EncodeToString(randomSuffixBytes))
	participantIdentity := fmt.Sprint(authenticatedUser.ID)

	// Prepare metadata that will be used for both room creation and participant token
	agentContextMetadataMap := map[string]string{
		"access_token": appTokenString,
		"user_id":      participantIdentity,
		"email":        authenticatedUser.Email,
		"service_name": serviceName,
		"agent_url":    agentURL,
		"agent_port":   fmt.Sprintf("%d", agentPort),
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
		AgentUrl:     agentURL,
		AgentPort:    agentPort,
	}, nil
}

// ProcessVoiceNote processes an audio file and returns AI response with transcription
func (s *VoiceSessionService) ProcessVoiceNote(ctx context.Context, userID uint, accessToken string, audioContent []byte, filename string, contentType string, req *pb.ProcessVoiceNoteRequest) (*pb.ProcessVoiceNoteResponse, error) {
	startTime := time.Now()

	// Validate audio content
	if len(audioContent) == 0 {
		return nil, status.Error(codes.InvalidArgument, "audio content cannot be empty")
	}

	// Check file size (25MB limit)
	const maxFileSize = 25 * 1024 * 1024 // 25MB
	if len(audioContent) > maxFileSize {
		return nil, status.Error(codes.InvalidArgument, "audio file size exceeds 25MB limit")
	}

	// Validate file extension if filename is provided
	if filename != "" {
		if !s.isValidAudioFile(filename) {
			return nil, status.Error(codes.InvalidArgument, "invalid audio file format. Supported formats: .mp3, .wav, .m4a, .ogg, .flac, .aac, .mp4, .webm")
		}
	}

	// Check if AI service URL is configured
	if s.config.AiServiceURL == "" {
		return nil, status.Error(codes.Internal, "AI service URL is not configured")
	}

	// DEBUG: Log the request details
	fmt.Printf("DEBUG: ProcessVoiceNote - UserID: %d, AudioSize: %d bytes, Filename: %s, ContentType: %s\n",
		userID, len(audioContent), filename, contentType)

	// Upload audio file to GCS and get public URL
	if filename == "" {
		filename = "voice_note.wav" // Default filename
	}

	audioFileURL, err := s.uploadVoiceFileToGCS(ctx, userID, audioContent, filename, contentType)
	if err != nil {
		fmt.Printf("ERROR: ProcessVoiceNote: Failed to upload audio file to GCS for User ID %d: %v\n", userID, err)
		return nil, status.Error(codes.Internal, "failed to upload audio file to storage")
	}

	// Save voice file record to database
	voiceFile := models.UserVoiceFile{
		UserID:      userID,
		FileURL:     audioFileURL,
		Filename:    filename,
		ContentType: contentType,
		FileSize:    int64(len(audioContent)),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if dbErr := s.db.WithContext(ctx).Create(&voiceFile).Error; dbErr != nil {
		fmt.Printf("ERROR: ProcessVoiceNote: Failed to save voice file record for User ID %d: %v\n", userID, dbErr)
		// Continue processing even if DB save fails, as the file is already uploaded
	}

	// Get transaction history - use provided or fetch from database
	var txHistory string
	if req.TxHistory != "" {
		txHistory = req.TxHistory
	} else {
		// Try to get recent transactions, but default to empty array if it fails
		recentTxs, err := s.getRecentTransactions(ctx, userID)
		if err != nil {
			// Log the error but don't fail the request - Django accepts empty array
			fmt.Printf("Warning: failed to get recent transactions for user %d: %v\n", userID, err)
			txHistory = "[]"
		} else {
			if len(recentTxs) == 0 {
				txHistory = "[]"
			} else {
				txHistoryBytes, _ := json.Marshal(recentTxs)
				txHistory = string(txHistoryBytes)
			}
		}
	}

	// Prepare multipart form data for Django service
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add required form fields for Django AI service
	err = writer.WriteField("user_id", strconv.Itoa(int(userID)))
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to write user_id field")
	}

	// Always include tx_history (Django expects this field)
	err = writer.WriteField("tx_history", txHistory)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to write tx_history field")
	}

	// Add access token if provided
	if accessToken != "" {
		err = writer.WriteField("access_token", accessToken)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to write access_token field")
		}
	}

	// Add audio file URL instead of raw audio content
	err = writer.WriteField("audio_file_url", audioFileURL)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to write audio_file_url field")
	}

	writer.Close()

	// DEBUG: Log the request size and URL
	fmt.Printf("DEBUG: Multipart form size: %d bytes, Django URL: %s\n", buf.Len(), s.config.AiServiceURL+"/api/voice-note")

	// Make request to Django service
	djangoURL := s.config.AiServiceURL + "/api/voice-note"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", djangoURL, &buf)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create request to AI service")
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	// DEBUG: Log the content type
	fmt.Printf("DEBUG: Content-Type: %s\n", writer.FormDataContentType())

	// Make the request
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		fmt.Printf("ERROR: Failed to communicate with Django service: %v\n", err)
		return nil, status.Error(codes.Unavailable, "failed to communicate with AI service")
	}
	defer resp.Body.Close()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read AI service response")
	}

	// DEBUG: Log the response
	fmt.Printf("DEBUG: Django response status: %d, body: %s\n", resp.StatusCode, string(responseBody))

	// Handle error responses
	if resp.StatusCode != http.StatusOK {
		var errorResp map[string]interface{}
		if unmarshalErr := json.Unmarshal(responseBody, &errorResp); unmarshalErr == nil {
			if errorMsg, ok := errorResp["error"].(string); ok {
				// Include more details in the error message
				if details, ok := errorResp["details"].(string); ok {
					return &pb.ProcessVoiceNoteResponse{
						Success: false,
						Msg:     fmt.Sprintf("Django validation error: %s - Details: %s", errorMsg, details),
					}, nil
				}
				return &pb.ProcessVoiceNoteResponse{
					Success: false,
					Msg:     fmt.Sprintf("Django error: %s", errorMsg),
				}, nil
			}
		}
		return &pb.ProcessVoiceNoteResponse{
			Success: false,
			Msg:     fmt.Sprintf("AI service returned error %d: %s", resp.StatusCode, string(responseBody)),
		}, nil
	}

	// Parse successful response
	var aiResponse map[string]interface{}
	if err = json.Unmarshal(responseBody, &aiResponse); err != nil {
		return nil, status.Error(codes.Internal, "failed to parse AI service response")
	}

	// Extract response fields
	response := getStringFromAIResponse(aiResponse, "response")
	transcribedText := getStringFromAIResponse(aiResponse, "transcribed_text")
	processingTime := time.Since(startTime).Milliseconds()

	// If AI service provides processing time, use that instead
	if aiProcessingTime, ok := aiResponse["processing_time_ms"].(float64); ok {
		processingTime = int64(aiProcessingTime)
	}

	// Update the voice file record with AI response and transcription
	now := time.Now()
	updateData := map[string]interface{}{
		"processed_at":  &now,
		"response":      response,
		"transcription": transcribedText,
		"updated_at":    now,
	}

	if updateErr := s.db.WithContext(ctx).Model(&models.UserVoiceFile{}).
		Where("user_id = ? AND file_url = ?", userID, audioFileURL).
		Updates(updateData).Error; updateErr != nil {
		fmt.Printf("ERROR: ProcessVoiceNote: Failed to update voice file record with AI response for User ID %d: %v\n", userID, updateErr)
		// Continue and return the response even if DB update fails
	}

	// Save voice interaction to AI chat history for retrieval
	voiceQuery := fmt.Sprintf("🎤 Voice Note: %s", transcribedText)
	if transcribedText == "" {
		voiceQuery = "🎤 Voice Note (transcription unavailable)"
	}

	if chatErr := s.saveVoiceToChatHistory(ctx, userID, voiceQuery, response); chatErr != nil {
		fmt.Printf("ERROR: ProcessVoiceNote: Failed to save voice interaction to chat history for User ID %d: %v\n", userID, chatErr)
		// Continue and return the response even if chat history save fails
	} else {
		// Trigger chat history file update so voice interactions appear in AI context
		if updateErr := s.aiChatService.UpdateChatHistoryFile(ctx, userID); updateErr != nil {
			fmt.Printf("ERROR: ProcessVoiceNote: Failed to update chat history file for User ID %d: %v\n", userID, updateErr)
			// Continue even if chat history file update fails
		} else {
			fmt.Printf("INFO: ProcessVoiceNote: Successfully updated chat history file for User ID %d\n", userID)
		}
	}

	return &pb.ProcessVoiceNoteResponse{
		Success:          true,
		Msg:              "Voice note processed successfully",
		Response:         response,
		TranscribedText:  transcribedText,
		ProcessingTimeMs: processingTime,
	}, nil
}

// isValidAudioFile checks if the file has a valid audio extension
func (s *VoiceSessionService) isValidAudioFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	validExts := []string{".mp3", ".wav", ".m4a", ".ogg", ".flac", ".aac", ".mp4", ".webm"}

	for _, validExt := range validExts {
		if ext == validExt {
			return true
		}
	}
	return false
}

// getRecentTransactions fetches the last 5 transactions for a user
func (s *VoiceSessionService) getRecentTransactions(ctx context.Context, userID uint) ([]map[string]interface{}, error) {
	// Fetch transfers
	var transfers []models.Transfer
	if err := s.db.WithContext(ctx).
		Where("from_user_id = ? OR to_user_id = ?", userID, userID).
		Order("created_at desc").
		Limit(5).
		Find(&transfers).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch transfers: %w", err)
	}

	// Fetch deposits
	var deposits []models.Deposit
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc").
		Limit(5).
		Find(&deposits).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch deposits: %w", err)
	}

	// Fetch withdrawals
	var withdrawals []models.Withdrawal
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc").
		Limit(5).
		Find(&withdrawals).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch withdrawals: %w", err)
	}

	// Combine and format transactions (simplified version)
	var allTransactions []map[string]interface{}

	// Add transfers
	for _, t := range transfers {
		allTransactions = append(allTransactions, map[string]interface{}{
			"type":        "transfer",
			"amount":      t.Amount,
			"currency":    "USD",
			"status":      string(t.Status),
			"created_at":  t.CreatedAt,
			"description": t.Reference,
		})
	}

	// Add deposits
	for _, d := range deposits {
		allTransactions = append(allTransactions, map[string]interface{}{
			"type":        "deposit",
			"amount":      d.Amount,
			"currency":    d.Currency,
			"status":      string(d.Status),
			"created_at":  d.CreatedAt,
			"description": d.SourceBankName,
		})
	}

	// Add withdrawals
	for _, w := range withdrawals {
		allTransactions = append(allTransactions, map[string]interface{}{
			"type":        "withdrawal",
			"amount":      w.Amount,
			"currency":    w.Currency,
			"status":      string(w.Status),
			"created_at":  w.CreatedAt,
			"description": w.TargetBankName,
		})
	}

	// Take only the 5 most recent
	if len(allTransactions) > 5 {
		allTransactions = allTransactions[:5]
	}

	return allTransactions, nil
}

// getStringFromAIResponse safely extracts a string value from AI response map
func getStringFromAIResponse(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// generateFileID creates a unique file identifier
func (s *VoiceSessionService) generateFileID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// getGCSClient provides a GCS client instance.
// TODO: Consider making the client a singleton or part of the service struct if created frequently.
func (s *VoiceSessionService) getGCSClient(ctx context.Context) (*storage.Client, error) {
	var client *storage.Client
	var err error

	if s.config.GCSBucketName == "" {
		return nil, fmt.Errorf("GCS_BUCKET_NAME is not configured")
	}

	gcpCredsEnv := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if gcpCredsEnv != "" {
		client, err = storage.NewClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("storage.NewClient (ADC): %w", err)
		}
	} else if s.config.GCSCredentialsFile != "" {
		if _, statErr := os.Stat(s.config.GCSCredentialsFile); os.IsNotExist(statErr) {
			return nil, fmt.Errorf("GCS credentials file from config not found at: %s", s.config.GCSCredentialsFile)
		}
		client, err = storage.NewClient(ctx, option.WithCredentialsFile(s.config.GCSCredentialsFile))
		if err != nil {
			return nil, fmt.Errorf("storage.NewClient with credentials file from config: %w", err)
		}
	} else {
		return nil, fmt.Errorf("GCS authentication failed: Neither GOOGLE_APPLICATION_CREDENTIALS env var nor GCS_CREDENTIALS_FILE config is set")
	}
	// DO NOT close the client here if it's intended to be reused.
	// If it's created per call, defer client.Close() should be added.
	// Assuming shared client for now.
	return client, nil
}

// uploadVoiceFileToGCS uploads a voice file to Google Cloud Storage and returns the public URL
func (s *VoiceSessionService) uploadVoiceFileToGCS(ctx context.Context, userID uint, audioContent []byte, filename, contentType string) (string, error) {
	client, err := s.getGCSClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get GCS client: %w", err)
	}
	defer client.Close()

	// Generate unique object name
	fileID := s.generateFileID()
	extension := filepath.Ext(filename)
	objectName := fmt.Sprintf("voice-files/user_%d/%s_%s%s", userID, fileID, time.Now().Format("20060102_150405"), extension)

	bucket := client.Bucket(s.config.GCSBucketName)
	obj := bucket.Object(objectName)
	writer := obj.NewWriter(ctx)
	writer.ContentType = contentType

	if _, err := writer.Write(audioContent); err != nil {
		writer.Close()
		return "", fmt.Errorf("failed to write file to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close GCS writer: %w", err)
	}

	// Make the object publicly readable
	if err := obj.ACL().Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
		fmt.Printf("WARN: Failed to make object public: %v\n", err)
	}

	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.config.GCSBucketName, objectName)
	return publicURL, nil
}

// saveVoiceToChatHistory saves a voice interaction to the AI chat history table
func (s *VoiceSessionService) saveVoiceToChatHistory(ctx context.Context, userID uint, query, response string) error {
	chatEntry := models.AIChatHistory{
		UserID:    userID,
		Query:     query,
		Response:  response,
		CreatedAt: time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(&chatEntry).Error; err != nil {
		return fmt.Errorf("failed to save voice interaction to chat history: %w", err)
	}
	fmt.Printf("INFO: saveVoiceToChatHistory: Saved voice interaction to chat history for User ID: %d\n", userID)
	return nil
}

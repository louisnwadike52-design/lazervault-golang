package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"log" // Use standard Go log package
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// Placeholder for the context key used by auth middleware to store user ID.
// Adjust this based on your actual authentication implementation.
type contextKey string

const authUserIDKey contextKey = "user_id" // Example key

// ErrAIChatbotURLMissing indicates the configuration for the chatbot URL is missing.
var ErrAIChatbotURLMissing = status.Error(codes.Internal, "AI chatbot URL is not configured")

// ErrAIChatbotRequestFailed indicates failure communicating with the external AI service.
var ErrAIChatbotRequestFailed = status.Error(codes.Unavailable, "failed to communicate with AI chatbot service")

// ErrUserTxFileNotFound indicates the user's transaction file path wasn't found.
var ErrUserTxFileNotFound = status.Error(codes.NotFound, "user transaction data file not found")

// AIChatService implements the pb.AIChatServiceServer interface.
type AIChatService struct {
	db         *gorm.DB
	config     *configs.Config
	httpClient *http.Client // Use a shared HTTP client
	// userService                         IUserService // Removed UserService dependency
}

// NewAIChatService creates a new AIChatService instance.
func NewAIChatService(db *gorm.DB, config *configs.Config) *AIChatService { // Removed userService param
	return &AIChatService{
		db:     db,
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Set a reasonable timeout
		},
		// userService: userService, // Removed userService storage
	}
}

// ProcessChat handles the AI chat request.
func (s *AIChatService) ProcessChat(ctx context.Context, userID uint, req *pb.ProcessChatRequest) (*pb.ProcessChatResponse, error) { // Added userID param
	log.Printf("INFO: ProcessChat Service: Received request for User ID: %d", userID)

	// 1. Input Validation
	if req.GetQuery() == "" {
		log.Println("WARN: ProcessChat Service: Received empty query")
		return nil, status.Error(codes.InvalidArgument, "query cannot be empty")
	}

	// 3. Configuration Check
	if s.config.AiServiceURL == "" {
		log.Println("ERROR: ProcessChat: AI Chatbot URL is not configured")
		return nil, ErrAIChatbotURLMissing
	}

	// 4. Fetch User Transaction File Path
	var userTxFile models.UserTransactionFile
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&userTxFile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("WARN: ProcessChat: User transaction file not found for User ID: %d. err: %v", userID, err)
			return nil, ErrUserTxFileNotFound
		}
		log.Printf("ERROR: ProcessChat: Database error fetching transaction file for User ID: %d. err: %v", userID, err)
		return nil, status.Error(codes.Internal, "failed to retrieve user data")
	}
	if userTxFile.FilePath == "" {
		log.Printf("WARN: ProcessChat: User transaction file path is empty for User ID: %d", userID)
		// Treat as not found, as an empty path is unusable
		return nil, ErrUserTxFileNotFound
	}
	log.Printf("INFO: ProcessChat: Found Tx File Path for User ID %d: %s", userID, userTxFile.FilePath)

	// 5. Prepare Request for External Chatbot
	chatbotReqPayload := map[string]string{
		"query":        req.GetQuery(),
		"tx_file_path": userTxFile.FilePath,
	}
	payloadBytes, err := json.Marshal(chatbotReqPayload)
	if err != nil {
		log.Printf("ERROR: ProcessChat: Failed to marshal request payload for chatbot. err: %v", err)
		return nil, status.Error(codes.Internal, "failed to prepare request for AI service")
	}

	// 6. Make HTTP POST Request to Chatbot
	log.Printf("INFO: ProcessChat: Sending request to AI Chatbot at %s", s.config.AiServiceURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.config.AiServiceURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("ERROR: ProcessChat: Failed to create HTTP request for chatbot. err: %v", err)
		return nil, status.Error(codes.Internal, "failed to prepare request for AI service")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Potentially add an API key or other auth headers if the chatbot requires it
	// httpReq.Header.Set("Authorization", "Bearer <your-chatbot-api-key>")

	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("ERROR: ProcessChat: HTTP request to chatbot failed. err: %v", err)
		return nil, ErrAIChatbotRequestFailed
	}
	defer httpResp.Body.Close()

	// 7. Handle Chatbot Response
	if httpResp.StatusCode != http.StatusOK {
		// Try to read body for more info, but don't fail if reading fails
		bodyBytes, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			log.Printf("WARN: ProcessChat: Failed to read error response body from chatbot (Status %d). err: %v", httpResp.StatusCode, readErr)
		}
		log.Printf("ERROR: ProcessChat: Chatbot request failed with status %d. Body: %s", httpResp.StatusCode, string(bodyBytes))
		// Map common errors, otherwise return unavailable
		switch httpResp.StatusCode {
		case http.StatusBadRequest:
			return nil, status.Errorf(codes.InvalidArgument, "AI chatbot rejected request (status %d)", httpResp.StatusCode)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, status.Errorf(codes.Unauthenticated, "Authentication failed with AI chatbot (status %d)", httpResp.StatusCode)
		case http.StatusNotFound:
			return nil, status.Errorf(codes.NotFound, "AI chatbot endpoint not found (status %d)", httpResp.StatusCode)
		default:
			return nil, ErrAIChatbotRequestFailed // General failure for other codes
		}
	}

	// 8. Parse Successful Response
	var chatbotRespPayload map[string]string
	if err := json.NewDecoder(httpResp.Body).Decode(&chatbotRespPayload); err != nil {
		log.Printf("ERROR: ProcessChat: Failed to decode JSON response from chatbot. err: %v", err)
		return nil, status.Error(codes.Internal, "failed to parse response from AI service")
	}

	aiResponse, ok := chatbotRespPayload["response"]
	if !ok || aiResponse == "" {
		log.Println("WARN: ProcessChat: Chatbot response missing 'response' field or field is empty")
		// Consider if this is an error or just an empty response
		return nil, status.Error(codes.Internal, "invalid response format from AI service")
	}

	log.Printf("INFO: ProcessChat: Received successful response from AI Chatbot for User ID: %d", userID)

	// 9. Return Response
	return &pb.ProcessChatResponse{
		Response:       aiResponse,
		UserTxFilePath: userTxFile.FilePath,
	}, nil
}

// TODO: Consider adding error handling for `io.ReadAll` in the non-200 status check.
// TODO: Import `errors` and `io` packages.

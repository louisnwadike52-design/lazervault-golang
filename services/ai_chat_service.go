package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/tasks"
	"log" // Use standard Go log package
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage" // GCS Client
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// ErrChatHistoryUpdateFailed indicates failure updating the chat history file.
var ErrChatHistoryUpdateFailed = errors.New("failed to update chat history file")

// ErrAIIndexingFailed indicates failure triggering AI indexing.
var ErrAIIndexingFailed = errors.New("failed to trigger AI indexing")

// --- Payloads for AI Indexing Endpoints ---
type aiIndexChatPayload struct {
	FilePath string `json:"file_path"`
	UserID   string `json:"user_id"`
}

type aiIndexTransactionsPayload struct {
	FilePath string `json:"file_path"`
	UserID   string `json:"user_id"`
}

// AIChatService implements the pb.AIChatServiceServer interface.
type AIChatService struct {
	db              *gorm.DB
	config          *configs.Config
	httpClient      *http.Client // Use a shared HTTP client
	taskDistributor tasks.TaskDistributor
}

// getRecentTransactions fetches the last 5 transactions for a user and returns them as a slice of maps
func (s *AIChatService) getRecentTransactions(ctx context.Context, userID uint) ([]map[string]interface{}, error) {
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

	// Combine all transactions into a single slice
	type Transaction struct {
		Type        string    `json:"type"`
		Amount      int64     `json:"amount"`
		Currency    string    `json:"currency"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"created_at"`
		Description string    `json:"description,omitempty"`
	}

	var allTransactions []Transaction

	// Add transfers
	for _, t := range transfers {
		allTransactions = append(allTransactions, Transaction{
			Type:        "transfer",
			Amount:      t.Amount,
			Currency:    "USD", // Assuming USD as default, adjust if needed
			Status:      string(t.Status),
			CreatedAt:   t.CreatedAt,
			Description: t.Reference,
		})
	}

	// Add deposits
	for _, d := range deposits {
		allTransactions = append(allTransactions, Transaction{
			Type:        "deposit",
			Amount:      d.Amount,
			Currency:    d.Currency,
			Status:      string(d.Status),
			CreatedAt:   d.CreatedAt,
			Description: d.SourceBankName,
		})
	}

	// Add withdrawals
	for _, w := range withdrawals {
		allTransactions = append(allTransactions, Transaction{
			Type:        "withdrawal",
			Amount:      w.Amount,
			Currency:    w.Currency,
			Status:      string(w.Status),
			CreatedAt:   w.CreatedAt,
			Description: w.TargetBankName,
		})
	}

	// Sort by creation date (most recent first)
	sort.Slice(allTransactions, func(i, j int) bool {
		return allTransactions[i].CreatedAt.After(allTransactions[j].CreatedAt)
	})

	// Take only the 5 most recent transactions
	if len(allTransactions) > 5 {
		allTransactions = allTransactions[:5]
	}

	// Convert to []map[string]interface{}
	result := make([]map[string]interface{}, len(allTransactions))
	for i, tx := range allTransactions {
		b, _ := json.Marshal(tx)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		result[i] = m
	}

	return result, nil
}

// NewAIChatService creates a new AIChatService instance.
func NewAIChatService(db *gorm.DB, config *configs.Config, taskDistributor tasks.TaskDistributor) *AIChatService {
	return &AIChatService{
		db:     db,
		config: config,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Increased timeout slightly for potential AI calls
		},
		taskDistributor: taskDistributor, // Store the distributor
	}
}

// ProcessChat handles the primary AI chat request with file support.
func (s *AIChatService) ProcessChat(ctx context.Context, userID uint, accessToken string, req *pb.ProcessChatRequest) (*pb.ProcessChatResponse, error) {
	hasFile := req.GetUploadedFile() != nil
	var fileInfo string
	if hasFile {
		fileInfo = fmt.Sprintf(" with file: %s", req.GetUploadedFile().GetFilename())
	}
	log.Printf("INFO: ProcessChat Service: Received request for User ID: %d%s", userID, fileInfo)

	if req.GetQuery() == "" {
		log.Println("WARN: ProcessChat Service: Received empty query")
		return nil, status.Error(codes.InvalidArgument, "query cannot be empty")
	}
	if s.config.AiServiceURL == "" {
		log.Println("ERROR: ProcessChat: AI Chatbot URL is not configured")
		return nil, ErrAIChatbotURLMissing
	}

	// Fetch recent transactions as a slice
	txHistoryList, err := s.getRecentTransactions(ctx, userID)
	if err != nil {
		log.Printf("WARN: ProcessChat: Failed to fetch transaction history for User ID %d: %v", userID, err)
		txHistoryList = []map[string]interface{}{} // fallback to empty list
	}

	// Check if file is present
	hasFiles := req.GetUploadedFile() != nil

	if hasFiles {
		log.Printf("INFO: ProcessChat: Processing uploaded file: %s", req.GetUploadedFile().GetFilename())
		// Validate file has content
		if len(req.GetUploadedFile().GetFileContent()) == 0 {
			log.Printf("ERROR: ProcessChat: Uploaded file has no content for User ID %d", userID)
			return nil, status.Error(codes.InvalidArgument, "uploaded file has no content")
		}
	}

	userIDStr := strconv.FormatUint(uint64(userID), 10)

	// Convert tx_history to JSON string as expected by AI service
	txHistoryJSON, _ := json.Marshal(txHistoryList)

	// Remove access_token from payload since we'll pass it via header
	chatbotReqPayload := map[string]interface{}{
		"query":      req.GetQuery(),
		"user_id":    userIDStr,
		"tx_history": string(txHistoryJSON), // Send as JSON string
	}

	// Always use the same /api/chat endpoint
	chatEndpointURL := s.config.AiServiceURL + "/api/chat"
	var httpResp *http.Response

	if hasFiles {
		// Use multipart form data when files are present
		log.Printf("INFO: ProcessChat: Sending request with files to AI Chatbot at %s", chatEndpointURL)
		// Pass the original uploaded file directly to match AI service expectation
		httpResp, err = s.sendFilesToAIService(ctx, chatEndpointURL, accessToken, chatbotReqPayload, []*pb.ChatFile{req.GetUploadedFile()})
	} else {
		// Use regular JSON request when no files
		log.Printf("INFO: ProcessChat: Sending text-only query to AI Chatbot at %s", chatEndpointURL)
		payloadBytes, marshalErr := json.Marshal(chatbotReqPayload)
		if marshalErr != nil {
			log.Printf("ERROR: ProcessChat: Failed to marshal request payload: %v", marshalErr)
			return nil, status.Error(codes.Internal, "failed to prepare request for AI service")
		}

		httpReq, reqErr := http.NewRequestWithContext(ctx, "POST", chatEndpointURL, bytes.NewBuffer(payloadBytes))
		if reqErr != nil {
			log.Printf("ERROR: ProcessChat: Failed to create HTTP request: %v", reqErr)
			return nil, status.Error(codes.Internal, "failed to prepare request for AI service")
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		httpResp, err = s.httpClient.Do(httpReq)
	}

	if err != nil {
		log.Printf("ERROR: ProcessChat: HTTP request to AI service failed: %v", err)
		if os.IsTimeout(err) {
			log.Printf("ERROR: ProcessChat: Request timed out after %v", s.httpClient.Timeout)
			return nil, status.Error(codes.DeadlineExceeded, "AI service request timed out")
		}
		if strings.Contains(err.Error(), "connection refused") {
			log.Printf("ERROR: ProcessChat: Connection refused to AI service")
			return nil, status.Error(codes.Unavailable, "AI service is not running or not accessible")
		}
		return nil, ErrAIChatbotRequestFailed
	}
	defer httpResp.Body.Close()

	log.Printf("INFO: ProcessChat: Received response from AI service - Status: %d", httpResp.StatusCode)

	// Read response body
	bodyBytes, readErr := io.ReadAll(httpResp.Body)
	if readErr != nil {
		log.Printf("WARN: ProcessChat: Failed to read response body (Status %d): %v", httpResp.StatusCode, readErr)
	}

	if httpResp.StatusCode != http.StatusOK {
		log.Printf("ERROR: ProcessChat: AI service request failed with status %d. Body: %s", httpResp.StatusCode, string(bodyBytes))
		switch httpResp.StatusCode {
		case http.StatusBadRequest:
			return nil, status.Errorf(codes.InvalidArgument, "AI service rejected request (status %d)", httpResp.StatusCode)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, status.Errorf(codes.Unauthenticated, "Authentication failed with AI service (status %d)", httpResp.StatusCode)
		case http.StatusNotFound:
			return nil, status.Errorf(codes.NotFound, "AI service endpoint not found (status %d)", httpResp.StatusCode)
		default:
			return nil, ErrAIChatbotRequestFailed
		}
	}

	// Parse AI service response
	var response *pb.ProcessChatResponse
	if hasFiles {
		// Parse enhanced response with potential file data
		response, err = s.parseAIServiceResponse(bodyBytes)
		if err != nil {
			log.Printf("ERROR: ProcessChat: Failed to parse AI service response: %v", err)
			return nil, status.Error(codes.Internal, "failed to parse AI service response")
		}
	} else {
		// Parse simple text response
		var chatbotRespPayload map[string]string
		if err := json.Unmarshal(bodyBytes, &chatbotRespPayload); err != nil {
			log.Printf("ERROR: ProcessChat: Failed to unmarshal AI response: %v", err)
			return nil, status.Error(codes.Internal, "failed to parse AI service response")
		}

		aiResponse := chatbotRespPayload["response"]
		if aiResponse == "" {
			log.Printf("ERROR: ProcessChat: Empty response from AI service for User ID %d", userID)
			return nil, status.Error(codes.Internal, "received empty response from AI service")
		}

		response = &pb.ProcessChatResponse{
			Success:  true,
			Msg:      "Successfully processed AI chat request",
			Query:    req.GetQuery(),
			Response: aiResponse,
		}
	}

	// Ensure query is set in response
	response.Query = req.GetQuery()

	// Save chat entry to database (save the text response, files are handled separately)
	if err := s.SaveChatEntry(ctx, userID, req.GetQuery(), response.GetResponse()); err != nil {
		log.Printf("ERROR: ProcessChat: Failed to save chat entry to database for User ID %d: %v", userID, err)
		// Continue even if save fails, as we want to return the AI response to the user
	}

	log.Printf("INFO: ProcessChat: Successfully processed request for User ID %d", userID)
	return response, nil
}

// --- Helper Functions --- //

// SaveChatEntry saves a single chat interaction to the database.
func (s *AIChatService) SaveChatEntry(ctx context.Context, userID uint, query, response string) error {

	chatEntry := models.AIChatHistory{
		UserID:    userID,
		Query:     query,
		Response:  response,
		CreatedAt: time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(&chatEntry).Error; err != nil {
		log.Printf("ERROR: SaveChatEntry: Failed to save chat history to DB for User ID %d: %v", userID, err)
		return fmt.Errorf("db error saving chat entry: %w", err)
	}

	log.Printf("INFO: SaveChatEntry: Saved chat history entry for User ID: %d", userID)

	return nil
}

// UpdateChatHistoryFile fetches history, formats it, uploads to GCS, and updates DB record with Public URL.
func (s *AIChatService) UpdateChatHistoryFile(ctx context.Context, userID uint) error {
	log.Printf("INFO: UpdateChatHistoryFile: Starting update for User ID: %d", userID)

	// 1. Fetch History
	var history []models.AIChatHistory
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at asc").Find(&history).Error; err != nil {
		log.Printf("ERROR: UpdateChatHistoryFile: Failed to fetch chat history for User ID %d: %v", userID, err)
		return fmt.Errorf("db error fetching history: %w", err)
	}

	// 2. Format History
	historyData, err := s.formatChatHistory(history)
	if err != nil {
		log.Printf("ERROR: UpdateChatHistoryFile: Failed to format chat history for User ID %d: %v", userID, err)
		return fmt.Errorf("formatting error: %w", err)
	}

	// 3. Upload to GCS (returns objectName)
	objectName, err := s.uploadOrOverwriteChatHistoryFile(ctx, userID, historyData)
	if err != nil {
		log.Printf("ERROR: UpdateChatHistoryFile: Failed to upload chat history file for User ID %d: %v", userID, err)
		return fmt.Errorf("gcs upload error: %w", err)
	}

	// 4. Construct Public URL
	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.config.GCSBucketName, objectName)
	log.Printf("INFO: UpdateChatHistoryFile: Constructed public URL for User ID %d: %s", userID, publicURL)

	// 5. Update DB Record with Public URL
	fileRecord := models.UserChatHistoryFile{
		UserID:   userID,
		FilePath: publicURL, // Store the public URL
	}

	if errDb := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"file_path", "updated_at"}),
	}).Create(&fileRecord).Error; errDb != nil {
		log.Printf("ERROR: UpdateChatHistoryFile: Failed to save/update user chat history file public URL in DB for User ID %d: %v", userID, errDb)
		return fmt.Errorf("db error updating file path: %w", errDb)
	}

	log.Printf("INFO: UpdateChatHistoryFile: Completed file update and saved public URL for User ID: %d", userID)

	return nil
}

// formatChatHistory converts chat history records to CSV format.
func (s *AIChatService) formatChatHistory(history []models.AIChatHistory) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	csvWriter := csv.NewWriter(&buf)

	// Write header
	header := []string{"Timestamp", "Role", "Content"}
	if err := csvWriter.Write(header); err != nil {
		return nil, fmt.Errorf("failed writing CSV header: %w", err)
	}

	// Write records
	for _, entry := range history {
		// Write User query
		userRecord := []string{
			entry.CreatedAt.Format(time.RFC3339), // Use a standard timestamp format
			"User",
			entry.Query,
		}
		if err := csvWriter.Write(userRecord); err != nil {
			return nil, fmt.Errorf("failed writing user entry to CSV for query '%s': %w", entry.Query, err)
		}

		// Write AI response
		aiRecord := []string{
			entry.CreatedAt.Format(time.RFC3339), // Can use the same timestamp or fetch response time if stored
			"AI",
			entry.Response,
		}
		if err := csvWriter.Write(aiRecord); err != nil {
			return nil, fmt.Errorf("failed writing AI entry to CSV for query '%s': %w", entry.Query, err)
		}
	}

	// Flush ensures all data is written to the buffer
	csvWriter.Flush()

	if err := csvWriter.Error(); err != nil {
		return nil, fmt.Errorf("error during CSV writing: %w", err)
	}

	return &buf, nil
}

// uploadOrOverwriteChatHistoryFile uploads chat history data to GCS and returns the object name.
func (s *AIChatService) uploadOrOverwriteChatHistoryFile(ctx context.Context, userID uint, data *bytes.Buffer) (string, error) {
	userIDStr := strconv.FormatUint(uint64(userID), 10)
	objectName := fmt.Sprintf("user-chat-history/%s/history.csv", userIDStr)
	bucketName := s.config.GCSBucketName

	client, err := s.getGCSClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get GCS client: %w", err)
	}

	bucket := client.Bucket(bucketName)
	obj := bucket.Object(objectName)

	wc := obj.NewWriter(ctx)
	wc.ContentType = "text/csv"

	if _, err := wc.Write(data.Bytes()); err != nil {
		_ = wc.Close() // Attempt to close even on write error
		return "", fmt.Errorf("GCS wc.Write failed: %w", err)
	}
	if err := wc.Close(); err != nil {
		return "", fmt.Errorf("GCS wc.Close failed: %w", err)
	}

	log.Printf("INFO: Chat history file uploaded/overwritten at gs://%s/%s", bucketName, objectName)
	// Return only the objectName, not the full gs:// path
	return objectName, nil
}

// TriggerChatHistoryIndexing sends the stored chat history public URL to the AI service.
func (s *AIChatService) TriggerChatHistoryIndexing(ctx context.Context, userID uint) error {
	userIDStr := strconv.FormatUint(uint64(userID), 10)
	log.Printf("INFO: TriggerChatHistoryIndexing: Starting for User ID: %s", userIDStr)

	// 1. Fetch Chat History File Path (which is now the Public URL)
	var chatFile models.UserChatHistoryFile
	chatErr := s.db.WithContext(ctx).Where("user_id = ?", userID).Select("file_path").First(&chatFile).Error
	if chatErr != nil {
		if errors.Is(chatErr, gorm.ErrRecordNotFound) {
			log.Printf("WARN: TriggerChatHistoryIndexing: Chat history file record not found for User ID %s. Skipping indexing.", userIDStr)
			return nil
		}
		log.Printf("ERROR: TriggerChatHistoryIndexing: Failed to fetch chat history file path for User ID %s: %v", userIDStr, chatErr)
		return fmt.Errorf("db error fetching chat file path: %w", chatErr)
	}

	// 2. Prepare Indexing Payload with the stored Public URL
	indexPayload := aiIndexChatPayload{
		FilePath: chatFile.FilePath, // Directly use the stored public URL
		UserID:   userIDStr,
	}

	// 3. Make HTTP POST Request
	indexEndpointURL := s.config.AiServiceURL + "/api/index_chat_history"
	log.Printf("INFO: TriggerChatHistoryIndexing: Sending public URL %s for User ID %s", chatFile.FilePath, userIDStr)
	return s.callAIIndexEndpoint(ctx, indexEndpointURL, indexPayload, "Chat History", userIDStr)
}

// TriggerTransactionFileIndexing sends the stored transaction public URL to the AI service.
func (s *AIChatService) TriggerTransactionFileIndexing(ctx context.Context, userID uint) error {
	userIDStr := strconv.FormatUint(uint64(userID), 10)
	log.Printf("INFO: TriggerTransactionFileIndexing: Starting for User ID: %s", userIDStr)

	// 1. Fetch Tx File Path (which is now the Public URL)
	var txFile models.UserTransactionFile
	txErr := s.db.WithContext(ctx).Where("user_id = ?", userID).Select("file_path").First(&txFile).Error
	if txErr != nil {
		if errors.Is(txErr, gorm.ErrRecordNotFound) {
			log.Printf("WARN: TriggerTransactionFileIndexing: Transaction file record not found for User ID %s. Skipping indexing.", userIDStr)
			return nil
		}
		log.Printf("ERROR: TriggerTransactionFileIndexing: Failed to fetch transaction file path for User ID %s: %v", userIDStr, txErr)
		return fmt.Errorf("db error fetching tx file path: %w", txErr)
	}

	// 2. Prepare Indexing Payload with the stored Public URL
	indexPayload := aiIndexTransactionsPayload{
		FilePath: txFile.FilePath, // Directly use the stored public URL
		UserID:   userIDStr,
	}

	// 3. Make HTTP POST Request
	indexEndpointURL := s.config.AiServiceURL + "/api/index_transactions"
	log.Printf("INFO: TriggerTransactionFileIndexing: Sending public URL %s for User ID %s", txFile.FilePath, userIDStr)
	return s.callAIIndexEndpoint(ctx, indexEndpointURL, indexPayload, "Transaction File", userIDStr)
}

// callAIIndexEndpoint is a helper to make the POST request to a specific AI indexing endpoint.
func (s *AIChatService) callAIIndexEndpoint(ctx context.Context, endpointURL string, payload interface{}, indexType string, userIDStr string) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ERROR: callAIIndexEndpoint [%s]: Failed to marshal payload for User ID %s: %v", indexType, userIDStr, err)
		return fmt.Errorf("failed to marshal index payload: %w", err)
	}

	log.Printf("INFO: callAIIndexEndpoint [%s]: Sending request to %s for User ID %s", indexType, endpointURL, userIDStr)
	log.Printf("INFO: callAIIndexEndpoint [%s]: Payload: %s", indexType, string(payloadBytes))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpointURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("ERROR: callAIIndexEndpoint [%s]: Failed to create HTTP request for User ID %s: %v", indexType, userIDStr, err)
		return fmt.Errorf("failed to create index request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Add Auth if needed
	// httpReq.Header.Set("Authorization", "Bearer <your-api-key>")

	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("ERROR: callAIIndexEndpoint [%s]: HTTP request failed for User ID %s to %s: %v", indexType, userIDStr, endpointURL, err)
		if os.IsTimeout(err) {
			return status.Error(codes.DeadlineExceeded, fmt.Sprintf("AI %s indexing request timed out", indexType))
		}
		if strings.Contains(err.Error(), "connection refused") {
			return status.Error(codes.Unavailable, fmt.Sprintf("AI %s indexing service is not running or not accessible at %s", indexType, endpointURL))
		}
		return ErrAIIndexingFailed // General comms failure
	}
	defer httpResp.Body.Close()

	log.Printf("INFO: callAIIndexEndpoint [%s]: Received response from %s - Status: %d", indexType, endpointURL, httpResp.StatusCode)

	// Allow 200 OK or 202 Accepted
	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		bodyBytes, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			log.Printf("ERROR: callAIIndexEndpoint [%s]: Request failed with status %d, and failed to read response body: %v", indexType, httpResp.StatusCode, readErr)
		} else {
			log.Printf("ERROR: callAIIndexEndpoint [%s]: Request failed with status %d. Body: %s", indexType, httpResp.StatusCode, string(bodyBytes))
		}

		switch httpResp.StatusCode {
		case http.StatusBadRequest:
			return status.Errorf(codes.InvalidArgument, "AI %s indexing rejected request (status %d)", indexType, httpResp.StatusCode)
		case http.StatusUnauthorized, http.StatusForbidden:
			return status.Errorf(codes.Unauthenticated, "Authentication failed with AI %s indexing (status %d)", indexType, httpResp.StatusCode)
		case http.StatusNotFound:
			return status.Errorf(codes.NotFound, "AI %s indexing endpoint not found (%s, status %d)", indexType, endpointURL, httpResp.StatusCode)
		default:
			return ErrAIIndexingFailed // General failure
		}
	}

	log.Printf("INFO: callAIIndexEndpoint [%s]: Successfully triggered indexing for User ID: %s", indexType, userIDStr)
	return nil
}

// getGCSClient provides a GCS client instance.
// TODO: Consider making the client a singleton or part of the service struct if created frequently.
func (s *AIChatService) getGCSClient(ctx context.Context) (*storage.Client, error) {
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

// GetAIChatHistory retrieves the AI chat history for a given user.
func (s *AIChatService) GetAIChatHistory(ctx context.Context, userID uint) (*pb.GetAIChatHistoryResponse, error) {
	log.Printf("INFO: GetAIChatHistory: Fetching AI history for User ID: %d", userID)

	var historyRecords []models.AIChatHistory
	// Fetch history ordered by creation time (oldest first)
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at asc").Find(&historyRecords).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No history is not an error, return empty list
			log.Printf("INFO: GetAIChatHistory: No AI history found for User ID: %d", userID)
			return &pb.GetAIChatHistoryResponse{History: []*pb.AIChatHistoryEntry{}}, nil
		}
		// Database error
		log.Printf("ERROR: GetAIChatHistory: Failed to query AI chat history for User ID %d: %v", userID, err)
		return nil, status.Error(codes.Internal, "failed to retrieve AI chat history")
	}

	// Convert DB records to protobuf response format
	pbHistory := make([]*pb.AIChatHistoryEntry, len(historyRecords))
	for i, record := range historyRecords {
		pbHistory[i] = &pb.AIChatHistoryEntry{
			Query:     record.Query,
			Response:  record.Response,
			Timestamp: record.CreatedAt.Format(time.RFC3339), // Format timestamp
		}
	}

	log.Printf("INFO: GetAIChatHistory: Retrieved %d AI history entries for User ID: %d", len(pbHistory), userID)
	return &pb.GetAIChatHistoryResponse{
		History: pbHistory,
	}, nil
}

// --- File Handling Functions ---

// generateFileID creates a unique file identifier
func (s *AIChatService) generateFileID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// uploadFileToGCS uploads a file to Google Cloud Storage and returns the public URL
func (s *AIChatService) uploadFileToGCS(ctx context.Context, userID uint, fileContent []byte, filename, contentType string) (string, error) {
	client, err := s.getGCSClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get GCS client: %w", err)
	}
	defer client.Close()

	// Generate unique object name
	fileID := s.generateFileID()
	extension := filepath.Ext(filename)
	objectName := fmt.Sprintf("chat-files/user_%d/%s_%s%s", userID, fileID, time.Now().Format("20060102_150405"), extension)

	bucket := client.Bucket(s.config.GCSBucketName)
	obj := bucket.Object(objectName)
	writer := obj.NewWriter(ctx)
	writer.ContentType = contentType

	if _, err := writer.Write(fileContent); err != nil {
		writer.Close()
		return "", fmt.Errorf("failed to write file to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close GCS writer: %w", err)
	}

	// Make the object publicly readable
	if err := obj.ACL().Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
		log.Printf("WARN: Failed to make object public: %v", err)
	}

	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.config.GCSBucketName, objectName)
	return publicURL, nil
}

// convertProtoChatFileToMap converts a protobuf ChatFile to a map for AI service
func (s *AIChatService) convertProtoChatFileToMap(file *pb.ChatFile) map[string]interface{} {
	return map[string]interface{}{
		"file_id":          file.GetFileId(),
		"filename":         file.GetFilename(),
		"content_type":     file.GetContentType(),
		"file_size":        file.GetFileSize(),
		"file_url":         file.GetFileUrl(),
		"upload_timestamp": file.GetUploadTimestamp(),
	}
}

// processUploadedFiles handles file uploads and preparation for AI service
func (s *AIChatService) processUploadedFiles(ctx context.Context, userID uint, files []*pb.ChatFile) ([]*pb.ChatFile, error) {
	var processedFiles []*pb.ChatFile

	for _, file := range files {
		// Generate file ID if not provided
		fileID := file.GetFileId()
		if fileID == "" {
			fileID = s.generateFileID()
		}

		// Upload file to GCS if file content is provided
		var fileURL string
		if len(file.GetFileContent()) > 0 {
			url, err := s.uploadFileToGCS(ctx, userID, file.GetFileContent(), file.GetFilename(), file.GetContentType())
			if err != nil {
				log.Printf("ERROR: Failed to upload file %s to GCS: %v", file.GetFilename(), err)
				return nil, fmt.Errorf("failed to upload file %s: %w", file.GetFilename(), err)
			}
			fileURL = url
		} else {
			fileURL = file.GetFileUrl()
		}

		processedFile := &pb.ChatFile{
			FileId:          fileID,
			Filename:        file.GetFilename(),
			ContentType:     file.GetContentType(),
			FileSize:        file.GetFileSize(),
			FileUrl:         fileURL,
			UploadTimestamp: time.Now().Format(time.RFC3339),
		}

		processedFiles = append(processedFiles, processedFile)
	}

	return processedFiles, nil
}

// sendFilesToAIService sends files and query to AI microservice with multipart form data
func (s *AIChatService) sendFilesToAIService(ctx context.Context, endpointURL string, accessToken string, payload map[string]interface{}, files []*pb.ChatFile) (*http.Response, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add form fields exactly as expected by AI service
	if err := writer.WriteField("query", payload["query"].(string)); err != nil {
		return nil, fmt.Errorf("failed to write query field: %w", err)
	}
	if err := writer.WriteField("user_id", payload["user_id"].(string)); err != nil {
		return nil, fmt.Errorf("failed to write user_id field: %w", err)
	}
	if err := writer.WriteField("tx_history", payload["tx_history"].(string)); err != nil {
		return nil, fmt.Errorf("failed to write tx_history field: %w", err)
	}

	// Add file (expecting single file named "file")
	for _, file := range files {
		if len(file.GetFileContent()) > 0 {
			// Add file content to the multipart form
			part, err := writer.CreateFormFile("file", file.GetFilename())
			if err != nil {
				return nil, fmt.Errorf("failed to create form file for %s: %w", file.GetFilename(), err)
			}

			if _, err := part.Write(file.GetFileContent()); err != nil {
				return nil, fmt.Errorf("failed to write file content for %s: %w", file.GetFilename(), err)
			}
		}
		break // Only handle first file to match single file expectation
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create request using the provided endpoint URL
	req, err := http.NewRequestWithContext(ctx, "POST", endpointURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	return s.httpClient.Do(req)
}

// parseAIServiceResponse parses the response from AI service that may include files
func (s *AIChatService) parseAIServiceResponse(responseBody []byte) (*pb.ProcessChatResponse, error) {
	var aiResponse struct {
		Response       string                   `json:"response"`
		GeneratedFiles []map[string]interface{} `json:"generated_files,omitempty"`
		FileAnalysis   map[string]interface{}   `json:"file_analysis,omitempty"`
	}

	if err := json.Unmarshal(responseBody, &aiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal AI response: %w", err)
	}

	response := &pb.ProcessChatResponse{
		Success:  true,
		Msg:      "Successfully processed AI chat request with files",
		Response: aiResponse.Response,
	}

	// Process generated files
	if len(aiResponse.GeneratedFiles) > 0 {
		for _, fileData := range aiResponse.GeneratedFiles {
			chatFile := &pb.ChatFile{
				FileId:      getStringFromMap(fileData, "file_id"),
				Filename:    getStringFromMap(fileData, "filename"),
				ContentType: getStringFromMap(fileData, "content_type"),
				FileUrl:     getStringFromMap(fileData, "file_url"),
			}
			if sizeFloat, ok := fileData["file_size"].(float64); ok {
				chatFile.FileSize = int64(sizeFloat)
			}
			response.GeneratedFiles = append(response.GeneratedFiles, chatFile)
		}
	}

	// Process file analysis
	if aiResponse.FileAnalysis != nil {
		fileAnalysis := &pb.FileAnalysis{
			Summary: getStringFromMap(aiResponse.FileAnalysis, "summary"),
		}

		if results, ok := aiResponse.FileAnalysis["results"].([]interface{}); ok {
			for _, resultData := range results {
				if resultMap, ok := resultData.(map[string]interface{}); ok {
					analysisResult := &pb.FileAnalysisResult{
						FileId:            getStringFromMap(resultMap, "file_id"),
						Filename:          getStringFromMap(resultMap, "filename"),
						AnalysisType:      getStringFromMap(resultMap, "analysis_type"),
						AnalysisResult:    getStringFromMap(resultMap, "analysis_result"),
						ProcessingSuccess: getBoolFromMap(resultMap, "processing_success"),
						ErrorMessage:      getStringFromMap(resultMap, "error_message"),
					}

					// Process metadata
					if metadata, ok := resultMap["metadata"].(map[string]interface{}); ok {
						analysisResult.Metadata = make(map[string]string)
						for key, value := range metadata {
							if strValue, ok := value.(string); ok {
								analysisResult.Metadata[key] = strValue
							}
						}
					}

					fileAnalysis.Results = append(fileAnalysis.Results, analysisResult)
				}
			}
		}

		response.FileAnalysis = fileAnalysis
	}

	return response, nil
}

// Helper functions for extracting values from maps
func getStringFromMap(m map[string]interface{}, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
}

func getBoolFromMap(m map[string]interface{}, key string) bool {
	if value, ok := m[key].(bool); ok {
		return value
	}
	return false
}

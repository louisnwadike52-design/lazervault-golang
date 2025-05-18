package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/tasks"
	"log" // Use standard Go log package
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage" // GCS Client
	"github.com/hibiken/asynq"
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

// getRecentTransactions fetches the last 5 transactions for a user
func (s *AIChatService) getRecentTransactions(ctx context.Context, userID uint) (string, error) {
	// Fetch transfers
	var transfers []models.Transfer
	if err := s.db.WithContext(ctx).
		Where("from_user_id = ? OR to_user_id = ?", userID, userID).
		Order("created_at desc").
		Limit(5).
		Find(&transfers).Error; err != nil {
		return "", fmt.Errorf("failed to fetch transfers: %w", err)
	}

	// Fetch deposits
	var deposits []models.Deposit
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc").
		Limit(5).
		Find(&deposits).Error; err != nil {
		return "", fmt.Errorf("failed to fetch deposits: %w", err)
	}

	// Fetch withdrawals
	var withdrawals []models.Withdrawal
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc").
		Limit(5).
		Find(&withdrawals).Error; err != nil {
		return "", fmt.Errorf("failed to fetch withdrawals: %w", err)
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

	// Convert to JSON
	txHistoryJSON, err := json.Marshal(allTransactions)
	if err != nil {
		return "", fmt.Errorf("failed to marshal transaction history: %w", err)
	}

	return string(txHistoryJSON), nil
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

// ProcessChat handles the primary AI chat request.
func (s *AIChatService) ProcessChat(ctx context.Context, userID uint, req *pb.ProcessChatRequest) (*pb.ProcessChatResponse, error) {
	log.Printf("INFO: ProcessChat Service: Received request for User ID: %d", userID)

	if req.GetQuery() == "" {
		log.Println("WARN: ProcessChat Service: Received empty query")
		return nil, status.Error(codes.InvalidArgument, "query cannot be empty")
	}
	if s.config.AiServiceURL == "" {
		log.Println("ERROR: ProcessChat: AI Chatbot URL is not configured")
		return nil, ErrAIChatbotURLMissing
	}

	// Fetch recent transactions
	txHistory, err := s.getRecentTransactions(ctx, userID)
	if err != nil {
		log.Printf("WARN: ProcessChat: Failed to fetch transaction history for User ID %d: %v", userID, err)
		// Continue without transaction history rather than failing the request
		txHistory = "[]"
	}

	// --- Call External AI /api/chat endpoint --- //
	userIDStr := strconv.FormatUint(uint64(userID), 10) // Convert uint userID to string
	chatbotReqPayload := map[string]string{
		"query":      req.GetQuery(),
		"user_id":    userIDStr,
		"tx_history": txHistory, // Add transaction history to the payload
	}
	payloadBytes, err := json.Marshal(chatbotReqPayload)
	if err != nil {
		log.Printf("ERROR: ProcessChat: Failed to marshal request payload for /api/chat: %v", err)
		return nil, status.Error(codes.Internal, "failed to prepare request for AI service")
	}

	log.Printf("INFO: ProcessChat: Sending query to AI Chatbot at %s/api/chat", s.config.AiServiceURL)
	chatEndpointURL := s.config.AiServiceURL + "/api/chat" // Use specific /api/chat path
	httpReq, err := http.NewRequestWithContext(ctx, "POST", chatEndpointURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("ERROR: ProcessChat: Failed to create HTTP request for /api/chat: %v", err)
		return nil, status.Error(codes.Internal, "failed to prepare request for AI service")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Add API Key or Auth if needed for /chats
	// httpReq.Header.Set("Authorization", "Bearer <your-api-key>")

	log.Printf("INFO: ProcessChat: Request payload to /api/chat: %s", string(payloadBytes)) // Log will now show user_id and tx_history
	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("ERROR: ProcessChat: HTTP request to /api/chat failed. err: %v, URL: %s", err, chatEndpointURL)
		if os.IsTimeout(err) {
			log.Printf("ERROR: ProcessChat: Request to /api/chat timed out after %v", s.httpClient.Timeout)
			return nil, status.Error(codes.DeadlineExceeded, "AI service request timed out")
		}
		if strings.Contains(err.Error(), "connection refused") {
			log.Printf("ERROR: ProcessChat: Connection refused for /api/chat at %s", chatEndpointURL)
			return nil, status.Error(codes.Unavailable, "AI service is not running or not accessible")
		}
		return nil, ErrAIChatbotRequestFailed
	}
	defer httpResp.Body.Close()

	log.Printf("INFO: ProcessChat: Received response from /api/chat - Status: %d", httpResp.StatusCode)

	// --- Handle Chatbot Response --- //
	bodyBytes, readErr := io.ReadAll(httpResp.Body)
	if readErr != nil {
		log.Printf("WARN: ProcessChat: Failed to read response body from /api/chat (Status %d). err: %v", httpResp.StatusCode, readErr)
		// Continue processing if possible, but response might be incomplete
	}

	if httpResp.StatusCode != http.StatusOK {
		log.Printf("ERROR: ProcessChat: Chatbot request to /api/chat failed with status %d. Body: %s", httpResp.StatusCode, string(bodyBytes))
		// Map common errors
		switch httpResp.StatusCode {
		case http.StatusBadRequest:
			return nil, status.Errorf(codes.InvalidArgument, "AI chatbot rejected request (status %d)", httpResp.StatusCode)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, status.Errorf(codes.Unauthenticated, "Authentication failed with AI chatbot (status %d)", httpResp.StatusCode)
		case http.StatusNotFound:
			return nil, status.Errorf(codes.NotFound, "AI chatbot endpoint '/api/chat' not found (status %d)", httpResp.StatusCode)
		default:
			return nil, ErrAIChatbotRequestFailed
		}
	}

	// --- Parse Successful Response --- //
	var chatbotRespPayload map[string]string
	if err := json.Unmarshal(bodyBytes, &chatbotRespPayload); err != nil { // Use bodyBytes
		log.Printf("ERROR: ProcessChat: Failed to decode JSON response from /api/chat: %v. Body: %s", err, string(bodyBytes))
		return nil, status.Error(codes.Internal, "failed to parse response from AI service")
	}

	aiResponse, ok := chatbotRespPayload["response"]
	if !ok || aiResponse == "" {
		log.Println("WARN: ProcessChat: Chatbot response from /api/chat missing 'response' field or field is empty")
		return nil, status.Error(codes.Internal, "invalid response format from AI service")
	}

	log.Printf("INFO: ProcessChat: Successfully processed query for User ID: %d", userID)

	// --- Enqueue Background Task for History Saving & Indexing --- //
	taskPayload, err := tasks.NewUpdateChatHistoryAndIndexTask(userID, req.GetQuery(), aiResponse)
	if err != nil {
		log.Printf("ERROR: ProcessChat: Failed to create task payload for User ID %d: %v", userID, err)
		// Log the error, but still return the response to the user.
		// The history won't be saved/indexed for this interaction.
		// Consider more robust error handling if this is critical.
	} else {
		taskOpts := []asynq.Option{
			asynq.Queue(tasks.QueueLow), // Use low priority queue
			asynq.MaxRetry(3),
			asynq.Timeout(5 * time.Minute),
		}
		if err := s.taskDistributor.DistributeTask(ctx, tasks.TypeUpdateChatHistoryAndIndex, taskPayload, taskOpts...); err != nil {
			log.Printf("ERROR: ProcessChat: Failed to enqueue chat history update task for User ID %d: %v", userID, err)
			// Log error, still return response to user.
		}
		log.Printf("INFO: ProcessChat: Enqueued chat history update task for User ID: %d", userID)
	}

	// --- Return Response to User Immediately --- //
	return &pb.ProcessChatResponse{
		Success:  true,
		Msg:      "Query processed successfully",
		Query:    req.GetQuery(),
		Response: aiResponse,
	}, nil
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
	return &pb.GetAIChatHistoryResponse{History: pbHistory}, nil
}

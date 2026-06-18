package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	pb "lazervaultGo/pb"
	"github.com/rs/zerolog/log"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// isDevMode checks if DEV_MODE environment variable is set
func isDevMode() bool {
	return os.Getenv("DEV_MODE") == "true"
}

// AIChatServiceProxy proxies AIChatService gRPC requests to the Python chat-agent-gateway via HTTP
type AIChatServiceProxy struct {
	pb.UnimplementedAIChatServiceServer
	chatGatewayURL string
	httpClient     *http.Client
}

// NewAIChatServiceProxy creates a new AIChatServiceProxy
func NewAIChatServiceProxy(chatGatewayURL string) *AIChatServiceProxy {
	return &AIChatServiceProxy{
		chatGatewayURL: chatGatewayURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// chatGatewayRequest is the JSON payload sent to the Python chat-agent-gateway
type chatGatewayRequest struct {
	Message       string            `json:"message"`
	SessionID     string            `json:"session_id"`
	UserID        string            `json:"user_id"`
	AccessToken   string            `json:"access_token"`
	SourceContext string            `json:"source_context"`
	Language      string            `json:"language"`
	AccountID     string            `json:"account_id"`
	UserCountry   string            `json:"user_country"`
	Currency      string            `json:"currency"`
	Metadata      map[string]string `json:"metadata"`
}

// chatGatewayResponse is the JSON response from the Python chat-agent-gateway
type chatGatewayResponse struct {
	MessageID       string                 `json:"message_id"`
	Response        string                 `json:"response"`
	ServiceRoutedTo string                 `json:"service_routed_to"`
	Timestamp       string                 `json:"timestamp"`
	Metadata        map[string]interface{} `json:"metadata"`
	Sentiment       map[string]interface{} `json:"sentiment"`
	// Enhanced fields from Phase 2+
	Intent              string                 `json:"intent,omitempty"`
	Entities            map[string]string      `json:"entities,omitempty"`
	RequiresConfirmation bool                  `json:"requires_confirmation,omitempty"`
	ActionButtons       []actionButton         `json:"action_buttons,omitempty"`
	ConfirmationData    *confirmationData      `json:"confirmation_data,omitempty"`
	SessionID           string                 `json:"session_id,omitempty"`
	ConversationState   string                 `json:"conversation_state,omitempty"`
}

type actionButton struct {
	Label      string `json:"label"`
	ActionType string `json:"action_type"`
	Payload    string `json:"payload"`
	Icon       string `json:"icon"`
}

type confirmationData struct {
	ActionType    string            `json:"action_type"`
	Amount        string            `json:"amount"`
	Currency      string            `json:"currency"`
	RecipientName string            `json:"recipient_name"`
	RecipientID   string            `json:"recipient_id"`
	Description   string            `json:"description"`
	Extra         map[string]string `json:"extra"`
}

// chatHistoryResponse is the JSON response from the chat history endpoint
type chatHistoryResponse struct {
	History []chatHistoryEntry `json:"history"`
}

type chatHistoryEntry struct {
	Query     string `json:"query"`
	Response  string `json:"response"`
	Timestamp string `json:"timestamp"`
}

// extractAuthToken extracts the JWT access token from gRPC metadata
func extractAuthToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if auth := md.Get("authorization"); len(auth) > 0 {
		token := auth[0]
		// Strip "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			return token[7:]
		}
		return token
	}
	return ""
}

// extractSessionID gets or generates a session ID from metadata
func extractSessionID(ctx context.Context, userID string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if sid := md.Get("x-session-id"); len(sid) > 0 {
			return sid[0]
		}
	}
	// Generate deterministic session ID from user ID + date
	now := time.Now().Format("2006-01-02")
	return fmt.Sprintf("chat_%s_%s", userID, now)
}

// extractLanguage gets user language preference from metadata
func extractLanguage(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if locale := md.Get("x-locale"); len(locale) > 0 {
			return locale[0]
		}
	}
	return "en"
}

// extractSourceContext gets the source context from metadata
func extractSourceContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if src := md.Get("x-source-context"); len(src) > 0 {
			return src[0]
		}
	}
	return "dashboard"
}

// extractAccountID gets the user's active account ID from metadata
func extractAccountID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if aid := md.Get("x-account-id"); len(aid) > 0 {
			return aid[0]
		}
	}
	return ""
}

// extractUserCountry gets the user's country from metadata
func extractUserCountry(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if country := md.Get("x-user-country"); len(country) > 0 {
			return country[0]
		}
	}
	return ""
}

// extractCurrency gets the user's currency from metadata
func extractCurrency(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if cur := md.Get("x-currency"); len(cur) > 0 {
			return cur[0]
		}
	}
	return ""
}

// ProcessChat proxies chat messages to the Python chat-agent-gateway
func (p *AIChatServiceProxy) ProcessChat(ctx context.Context, req *pb.ProcessChatRequest) (*pb.ProcessChatResponse, error) {
	// Extract user ID from JWT context
	userID, err := authinterceptor.GetUserID(ctx)
	if err != nil || userID == "" {
		log.Warn().Err(err).Msg("Failed to extract user ID from context for AI chat")
		return &pb.ProcessChatResponse{
			Success:  false,
			Msg:      "Authentication required",
			Response: "Please log in to use the AI assistant.",
		}, nil
	}

	// Extract auth token for forwarding
	accessToken := extractAuthToken(ctx)
	sessionID := extractSessionID(ctx, userID)
	language := extractLanguage(ctx)
	sourceContext := extractSourceContext(ctx)
	accountID := extractAccountID(ctx)
	userCountry := extractUserCountry(ctx)
	currency := extractCurrency(ctx)

	// Prefer proto request fields over metadata extraction when non-empty
	if req.SessionId != "" {
		sessionID = req.SessionId
	}
	if req.Language != "" {
		language = req.Language
	}
	if req.SourceContext != "" {
		sourceContext = req.SourceContext
	}
	if req.AccountId != "" {
		accountID = req.AccountId
	}
	if req.UserCountry != "" {
		userCountry = req.UserCountry
	}
	if req.Currency != "" {
		currency = req.Currency
	}

	queryPreview := req.Query
	if len(queryPreview) > 50 {
		queryPreview = queryPreview[:50]
	}

	log.Info().
		Str("user_id", userID).
		Str("session_id", sessionID).
		Str("query", queryPreview).
		Msg("Processing AI chat request")

	// Build request payload for chat-agent-gateway
	gatewayReq := chatGatewayRequest{
		Message:       req.Query,
		SessionID:     sessionID,
		UserID:        userID,
		AccessToken:   accessToken,
		SourceContext: sourceContext,
		Language:      language,
		AccountID:     accountID,
		UserCountry:   userCountry,
		Currency:      currency,
		Metadata:      map[string]string{},
	}

	// Handle file upload - encode as base64 in metadata
	if req.UploadedFile != nil {
		gatewayReq.Metadata["has_file"] = "true"
		gatewayReq.Metadata["file_name"] = req.UploadedFile.Filename
		gatewayReq.Metadata["content_type"] = req.UploadedFile.ContentType
		if len(req.UploadedFile.FileContent) > 0 {
			gatewayReq.Metadata["file_content_b64"] = base64.StdEncoding.EncodeToString(req.UploadedFile.FileContent)
		}
	}

	// Marshal and send to chat-agent-gateway
	jsonBody, err := json.Marshal(gatewayReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal chat gateway request")
		return nil, status.Errorf(codes.Internal, "failed to prepare chat request: %v", err)
	}

	chatURL := fmt.Sprintf("%s/chat", p.chatGatewayURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", chatURL, bytes.NewReader(jsonBody))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create HTTP request to chat gateway")
		return nil, status.Errorf(codes.Internal, "failed to create request: %v", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	}

	// Execute request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", chatURL).Msg("Failed to call chat-agent-gateway")
		errResponse := "I'm having trouble connecting right now. Please try again in a moment."
		if isDevMode() {
			errResponse = fmt.Sprintf("[DEV] Chat gateway connection failed: %v", err)
		}
		return &pb.ProcessChatResponse{
			Success:  false,
			Msg:      "AI service temporarily unavailable",
			Response: errResponse,
		}, nil
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read chat gateway response")
		return nil, status.Errorf(codes.Internal, "failed to read response: %v", err)
	}

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		log.Warn().
			Int("status", resp.StatusCode).
			Str("body", string(body[:min(len(body), 200)])).
			Msg("Chat gateway returned non-200 status")

		errResponse := "I'm experiencing some difficulties. Please try again."
		if isDevMode() {
			errResponse = fmt.Sprintf("[DEV] Chat gateway returned %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
		}
		return &pb.ProcessChatResponse{
			Success:  false,
			Msg:      fmt.Sprintf("AI service error (status %d)", resp.StatusCode),
			Response: errResponse,
		}, nil
	}

	// Parse response
	var gatewayResp chatGatewayResponse
	if err := json.Unmarshal(body, &gatewayResp); err != nil {
		log.Error().Err(err).Str("body", string(body[:min(len(body), 200)])).Msg("Failed to parse chat gateway response")
		return nil, status.Errorf(codes.Internal, "failed to parse response: %v", err)
	}

	log.Info().
		Str("user_id", userID).
		Str("service_routed_to", gatewayResp.ServiceRoutedTo).
		Bool("requires_confirmation", gatewayResp.RequiresConfirmation).
		Msg("AI chat response received")

	// Build gRPC response with conversation intelligence fields
	grpcResp := &pb.ProcessChatResponse{
		Success:              true,
		Msg:                  "OK",
		Query:                req.Query,
		Response:             gatewayResp.Response,
		Intent:               gatewayResp.Intent,
		Entities:             gatewayResp.Entities,
		RequiresConfirmation: gatewayResp.RequiresConfirmation,
		SessionId:            gatewayResp.SessionID,
		ConversationState:    gatewayResp.ConversationState,
	}

	// Map action buttons
	for _, btn := range gatewayResp.ActionButtons {
		grpcResp.ActionButtons = append(grpcResp.ActionButtons, &pb.ActionButton{
			Label:      btn.Label,
			ActionType: btn.ActionType,
			Payload:    btn.Payload,
			Icon:       btn.Icon,
		})
	}

	// Map confirmation data
	if gatewayResp.ConfirmationData != nil {
		grpcResp.ConfirmationData = &pb.ConfirmationData{
			ActionType:    gatewayResp.ConfirmationData.ActionType,
			Amount:        gatewayResp.ConfirmationData.Amount,
			Currency:      gatewayResp.ConfirmationData.Currency,
			RecipientName: gatewayResp.ConfirmationData.RecipientName,
			RecipientId:   gatewayResp.ConfirmationData.RecipientID,
			Description:   gatewayResp.ConfirmationData.Description,
			Extra:         gatewayResp.ConfirmationData.Extra,
		}
	}

	return grpcResp, nil
}

// GetAIChatHistory retrieves chat history from the chat-agent-gateway
func (p *AIChatServiceProxy) GetAIChatHistory(ctx context.Context, req *pb.GetAIChatHistoryRequest) (*pb.GetAIChatHistoryResponse, error) {
	// Extract user ID from JWT context
	userID, err := authinterceptor.GetUserID(ctx)
	if err != nil || userID == "" {
		log.Warn().Err(err).Msg("Failed to extract user ID for chat history")
		return &pb.GetAIChatHistoryResponse{
			History: []*pb.AIChatHistoryEntry{},
		}, nil
	}

	accessToken := extractAuthToken(ctx)
	sessionID := extractSessionID(ctx, userID)

	// Call chat-agent-gateway history endpoint
	// Include access_token as query parameter (required by chat-agent-gateway)
	// Use url.QueryEscape to properly encode the JWT token which contains dots and other special characters
	historyURL := fmt.Sprintf("%s/chat/history?user_id=%s&session_id=%s&access_token=%s",
		p.chatGatewayURL, url.QueryEscape(userID), url.QueryEscape(sessionID), url.QueryEscape(accessToken))
	httpReq, err := http.NewRequestWithContext(ctx, "GET", historyURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create history request")
		return &pb.GetAIChatHistoryResponse{History: []*pb.AIChatHistoryEntry{}}, nil
	}

	if accessToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch chat history from gateway")
		return &pb.GetAIChatHistoryResponse{History: []*pb.AIChatHistoryEntry{}}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn().Int("status", resp.StatusCode).Msg("Chat history endpoint returned non-200")
		return &pb.GetAIChatHistoryResponse{History: []*pb.AIChatHistoryEntry{}}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to read history response")
		return &pb.GetAIChatHistoryResponse{History: []*pb.AIChatHistoryEntry{}}, nil
	}

	var historyResp chatHistoryResponse
	if err := json.Unmarshal(body, &historyResp); err != nil {
		log.Warn().Err(err).Msg("Failed to parse history response")
		return &pb.GetAIChatHistoryResponse{History: []*pb.AIChatHistoryEntry{}}, nil
	}

	// Convert to gRPC response
	var entries []*pb.AIChatHistoryEntry
	for _, entry := range historyResp.History {
		entries = append(entries, &pb.AIChatHistoryEntry{
			Query:     entry.Query,
			Response:  entry.Response,
			Timestamp: entry.Timestamp,
		})
	}

	return &pb.GetAIChatHistoryResponse{History: entries}, nil
}

// IndexChatHistory triggers chat history indexing (stub - handled by existing async task system)
func (p *AIChatServiceProxy) IndexChatHistory(ctx context.Context, req *pb.IndexChatHistoryRequest) (*pb.IndexChatHistoryResponse, error) {
	return &pb.IndexChatHistoryResponse{
		Success: true,
		Msg:     "Chat history indexing triggered",
	}, nil
}

// IndexTransactionFile triggers transaction file indexing (stub - handled by existing async task system)
func (p *AIChatServiceProxy) IndexTransactionFile(ctx context.Context, req *pb.IndexTransactionFileRequest) (*pb.IndexTransactionFileResponse, error) {
	return &pb.IndexTransactionFileResponse{
		Success: true,
		Msg:     "Transaction file indexing triggered",
	}, nil
}


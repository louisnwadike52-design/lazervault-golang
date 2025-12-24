package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// AiScanService handles all AI scan-to-pay operations
type AiScanService struct {
	pb.UnimplementedAiScanServiceServer
	Db     *gorm.DB
	Config *configs.Config
}

// NewAiScanService creates a new AI scan service
func NewAiScanService(db *gorm.DB, config *configs.Config) *AiScanService {
	return &AiScanService{
		Db:     db,
		Config: config,
	}
}

// StartScanSession creates a new scan session
func (s *AiScanService) StartScanSession(ctx context.Context, req *pb.StartScanSessionRequest) (*pb.StartScanSessionResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	// Create scan session model
	session := &models.ScanSession{
		UserID:    req.UserId,
		ScanType:  req.ScanType.String(),
		Status:    "PENDING",
		CreatedAt: time.Now(),
	}

	// Save to database
	if err := s.Db.Create(session).Error; err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create scan session: %v", err))
	}

	return &pb.StartScanSessionResponse{
		SessionId: session.ID,
		ScanType:  req.ScanType,
		Status:    pb.ScanStatus_PENDING,
		CreatedAt: session.CreatedAt.Unix(),
	}, nil
}

// ProcessImage processes an image and extracts data using AI
func (s *AiScanService) ProcessImage(ctx context.Context, req *pb.ProcessImageRequest) (*pb.ProcessImageResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	if len(req.ImageData) == 0 {
		return nil, status.Error(codes.InvalidArgument, "image_data is required")
	}

	// Update session status to processing
	var session models.ScanSession
	if err := s.Db.Where("id = ?", req.SessionId).First(&session).Error; err != nil {
		return nil, status.Error(codes.NotFound, "scan session not found")
	}

	session.Status = "PROCESSING"
	if err := s.Db.Save(&session).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to update session status")
	}

	// Call AI microservice to process image (Django on port 8000)
	aiServiceURL := s.Config.AI_SCAN_SERVICE_URL
	if aiServiceURL == "" {
		aiServiceURL = "http://localhost:8000" // Default - Django server
	}

	extractedData, aiMessage, err := s.callAiScanService(req.ImageData, req.ScanType.String(), aiServiceURL)
	if err != nil {
		session.Status = "FAILED"
		s.Db.Save(&session)
		return &pb.ProcessImageResponse{
			SessionId:    req.SessionId,
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Update session with extracted data
	session.Status = "COMPLETED"
	extractedDataJSON, _ := json.Marshal(extractedData)
	session.ExtractedData = string(extractedDataJSON)
	if err := s.Db.Save(&session).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to save extracted data")
	}

	// Convert extracted data to protobuf format
	pbExtractedData := s.convertToProtobufData(extractedData)

	return &pb.ProcessImageResponse{
		SessionId:     req.SessionId,
		ExtractedData: pbExtractedData,
		AiMessage:     aiMessage,
		Success:       true,
	}, nil
}

// SendChatMessage sends a chat message and gets AI response
func (s *AiScanService) SendChatMessage(ctx context.Context, req *pb.SendChatMessageRequest) (*pb.SendChatMessageResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	// Save user message to chat history
	chatMessage := &models.ScanChatMessage{
		SessionID: req.SessionId,
		Content:   req.UserMessage,
		IsUser:    true,
		Timestamp: time.Now(),
	}

	if err := s.Db.Create(chatMessage).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to save chat message")
	}

	// Generate AI response (simplified - you can make this more sophisticated)
	aiResponse := s.generateAiChatResponse(req.UserMessage, req.ContextData)

	// Save AI response
	aiMessage := &models.ScanChatMessage{
		SessionID: req.SessionId,
		Content:   aiResponse,
		IsUser:    false,
		Timestamp: time.Now(),
	}

	if err := s.Db.Create(aiMessage).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to save AI response")
	}

	return &pb.SendChatMessageResponse{
		MessageId:  aiMessage.ID,
		AiResponse: aiResponse,
		Timestamp:  aiMessage.Timestamp.Unix(),
	}, nil
}

// GeneratePaymentInstruction generates a payment instruction from extracted data
func (s *AiScanService) GeneratePaymentInstruction(ctx context.Context, req *pb.GeneratePaymentInstructionRequest) (*pb.GeneratePaymentInstructionResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	if req.ExtractedData == nil {
		return nil, status.Error(codes.InvalidArgument, "extracted_data is required")
	}

	// Generate unique instruction ID
	instructionID := fmt.Sprintf("pay_%d", time.Now().UnixNano())

	// Create payment instruction
	instruction := &pb.PaymentInstruction{
		InstructionId: instructionID,
		Recipient:     req.ExtractedData.Recipient,
		Amount:        req.ExtractedData.Amount,
		Currency:      req.ExtractedData.Currency,
		Reference:     req.ExtractedData.Reference,
		Description:   fmt.Sprintf("AI Scan Payment - %s", req.ScanType.String()),
		Metadata: map[string]string{
			"session_id": req.SessionId,
			"scan_type":  req.ScanType.String(),
		},
	}

	return &pb.GeneratePaymentInstructionResponse{
		Instruction: instruction,
		Success:     true,
	}, nil
}

// ProcessPayment processes the payment instruction
func (s *AiScanService) ProcessPayment(ctx context.Context, req *pb.ScanProcessPaymentRequest) (*pb.ScanProcessPaymentResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	if req.Instruction == nil {
		return nil, status.Error(codes.InvalidArgument, "instruction is required")
	}

	// Convert string user ID to uint64
	userID, err := strconv.ParseUint(req.UserId, 10, 64)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid user_id: %v", err))
	}

	// Here you would integrate with your actual payment service
	// For now, we'll create a transaction record
	now := time.Now()
	transaction := &models.Transaction{
		UserID:      userID,
		Type:        "SCAN_PAYMENT",
		Amount:      req.Instruction.Amount,
		Currency:    req.Instruction.Currency,
		Description: req.Instruction.Description,
		Reference:   req.Instruction.Reference,
		Status:      "COMPLETED",
		Timestamp:   now,
		CompletedAt: &now,
	}

	if err := s.Db.Create(transaction).Error; err != nil {
		return &pb.ScanProcessPaymentResponse{
			Success:      false,
			Status:       "FAILED",
			ErrorMessage: err.Error(),
		}, nil
	}

	// Convert transaction ID (uint) to string
	transactionIDStr := strconv.FormatUint(uint64(transaction.ID), 10)

	// Update scan session with transaction ID
	if req.SessionId != "" {
		s.Db.Model(&models.ScanSession{}).
			Where("id = ?", req.SessionId).
			Update("transaction_id", transactionIDStr)
	}

	return &pb.ScanProcessPaymentResponse{
		TransactionId: transactionIDStr,
		Success:       true,
		Status:        "COMPLETED",
		Timestamp:     transaction.Timestamp.Unix(),
	}, nil
}

// GetScanHistory retrieves scan history for a user
func (s *AiScanService) GetScanHistory(ctx context.Context, req *pb.GetScanHistoryRequest) (*pb.GetScanHistoryResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	page := req.Page
	if page < 1 {
		page = 1
	}

	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	var sessions []models.ScanSession
	var totalCount int64

	// Get total count
	if err := s.Db.Model(&models.ScanSession{}).
		Where("user_id = ?", req.UserId).
		Count(&totalCount).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to count sessions")
	}

	// Get paginated sessions
	if err := s.Db.Where("user_id = ?", req.UserId).
		Order("created_at DESC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&sessions).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to retrieve scan history")
	}

	// Convert to protobuf format
	pbSessions := make([]*pb.ScanSessionHistory, len(sessions))
	for i, session := range sessions {
		var extractedData map[string]interface{}
		json.Unmarshal([]byte(session.ExtractedData), &extractedData)

		pbSessions[i] = &pb.ScanSessionHistory{
			SessionId:     session.ID,
			ScanType:      s.parseScanType(session.ScanType),
			Status:        s.parseScanStatus(session.Status),
			CreatedAt:     session.CreatedAt.Unix(),
			ExtractedData: s.convertToProtobufData(extractedData),
			TransactionId: session.TransactionID,
		}
	}

	return &pb.GetScanHistoryResponse{
		Sessions:   pbSessions,
		TotalCount: int32(totalCount),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

// Helper functions

func (s *AiScanService) callAiScanService(imageData []byte, scanType string, serviceURL string) (map[string]interface{}, string, error) {
	// Encode image to base64
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	// Prepare request
	requestBody := map[string]interface{}{
		"image_data": imageBase64,
		"scan_type":  scanType,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Call AI service (Django Ninja API)
	resp, err := http.Post(
		fmt.Sprintf("%s/api/scan/process-image", serviceURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to call AI service: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("AI service error: %s", string(body))
	}

	// Parse response
	var result struct {
		Success        bool                   `json:"success"`
		ExtractedData  map[string]interface{} `json:"extracted_data"`
		AiMessage      string                 `json:"ai_message"`
		ErrorMessage   string                 `json:"error_message"`
		ConfidenceScore float64               `json:"confidence_score"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("failed to decode response: %v", err)
	}

	if !result.Success {
		return nil, "", fmt.Errorf("AI processing failed: %s", result.ErrorMessage)
	}

	return result.ExtractedData, result.AiMessage, nil
}

func (s *AiScanService) convertToProtobufData(data map[string]interface{}) *pb.ExtractedData {
	if data == nil {
		return &pb.ExtractedData{}
	}

	return &pb.ExtractedData{
		Recipient:     getStringValue(data, "recipient"),
		Amount:        getFloatValue(data, "amount"),
		Currency:      getStringValue(data, "currency"),
		Reference:     getStringValue(data, "reference"),
		DueDate:       getStringValue(data, "due_date"),
		Description:   getStringValue(data, "description"),
		AccountNumber: getStringValue(data, "account_number"),
		RoutingNumber: getStringValue(data, "routing_number"),
		BankName:      getStringValue(data, "bank_name"),
		ConfidenceScore: float32(getFloatValue(data, "confidence_score")),
	}
}

func (s *AiScanService) generateAiChatResponse(userMessage string, contextData *pb.ExtractedData) string {
	// Simple AI response generation based on keywords
	lowerMessage := strings.ToLower(userMessage)

	if strings.Contains(lowerMessage, "yes") || strings.Contains(lowerMessage, "proceed") || strings.Contains(lowerMessage, "confirm") {
		return "Great! I'll process this payment for you. Please review the details once more before we proceed."
	}

	if strings.Contains(lowerMessage, "no") || strings.Contains(lowerMessage, "cancel") {
		return "No problem! The payment has been cancelled. Is there anything else I can help you with?"
	}

	if strings.Contains(lowerMessage, "help") {
		return "I can help you with:\n• Reviewing payment details\n• Processing payments\n• Answering questions about the scanned document\n\nWhat would you like to know?"
	}

	return "I understand. If you're ready to proceed with the payment, just let me know!"
}

func (s *AiScanService) parseScanType(scanType string) pb.ScanType {
	switch scanType {
	case "INVOICE":
		return pb.ScanType_INVOICE
	case "UTILITY_BILL":
		return pb.ScanType_UTILITY_BILL
	case "QR_CODE":
		return pb.ScanType_QR_CODE
	case "BARCODE":
		return pb.ScanType_BARCODE
	case "ACCOUNT_DETAILS":
		return pb.ScanType_ACCOUNT_DETAILS
	case "GIFT_CARD":
		return pb.ScanType_GIFT_CARD
	case "RECEIPT":
		return pb.ScanType_RECEIPT
	case "BANK_DETAILS":
		return pb.ScanType_BANK_DETAILS
	default:
		return pb.ScanType_SCAN_TYPE_UNSPECIFIED
	}
}

func (s *AiScanService) parseScanStatus(status string) pb.ScanStatus {
	switch status {
	case "PENDING":
		return pb.ScanStatus_PENDING
	case "PROCESSING":
		return pb.ScanStatus_PROCESSING
	case "COMPLETED":
		return pb.ScanStatus_COMPLETED
	case "FAILED":
		return pb.ScanStatus_FAILED
	default:
		return pb.ScanStatus_SCAN_STATUS_UNSPECIFIED
	}
}

// Helper functions for type conversion
func getStringValue(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getFloatValue(data map[string]interface{}, key string) float64 {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0.0
}

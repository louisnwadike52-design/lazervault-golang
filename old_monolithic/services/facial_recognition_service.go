package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"lazervaultGo/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type FacialRecognitionService struct {
	pb.UnimplementedFacialRecognitionServiceServer
	client             *http.Client
	serviceURL         string
	maxRetries         int
	retryDelay         time.Duration
	requestTimeout     time.Duration
	circuitBreakerOpen bool
}

// NewFacialRecognitionService creates a new facial recognition service
func NewFacialRecognitionService(serviceURL string) *FacialRecognitionService {
	return &FacialRecognitionService{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		serviceURL:     serviceURL,
		maxRetries:     3,
		retryDelay:     time.Second * 2,
		requestTimeout: 30 * time.Second,
	}
}

// RegisterFace registers a new face for a user
func (s *FacialRecognitionService) RegisterFace(ctx context.Context, req *pb.RegisterFaceRequest) (*pb.RegisterFaceResponse, error) {
	if s.circuitBreakerOpen {
		return nil, status.Error(codes.Unavailable, "facial recognition service is currently unavailable")
	}

	// Validate input
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if len(req.ImageData) == 0 {
		return nil, status.Error(codes.InvalidArgument, "image data is required")
	}

	// Create multipart request
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add form fields
	_ = writer.WriteField("user_id", req.UserId)
	if req.FaceId != "" {
		_ = writer.WriteField("face_id", req.FaceId)
	}
	_ = writer.WriteField("allow_duplicates", strconv.FormatBool(req.AllowDuplicates))
	if req.DuplicateThreshold > 0 {
		_ = writer.WriteField("duplicate_threshold", fmt.Sprintf("%.4f", req.DuplicateThreshold))
	}

	// Add image file
	part, err := writer.CreateFormFile("image", req.ImageFilename)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create form file")
	}
	_, err = part.Write(req.ImageData)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to write image data")
	}
	writer.Close()

	// Make request with retries
	var resp *http.Response
	var lastErr error

	for i := 0; i < s.maxRetries; i++ {
		resp, lastErr = s.makeRequest(ctx, "POST", "/api/facials/register", &buf, writer.FormDataContentType())
		if lastErr == nil {
			break
		}
		if i < s.maxRetries-1 {
			time.Sleep(s.retryDelay * time.Duration(i+1)) // Exponential backoff
		}
	}

	if lastErr != nil {
		s.circuitBreakerOpen = true
		go s.resetCircuitBreaker()
		return nil, status.Error(codes.Unavailable, fmt.Sprintf("failed to communicate with facial recognition service: %v", lastErr))
	}

	defer resp.Body.Close()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read response body")
	}

	if resp.StatusCode != http.StatusOK {
		return s.handleErrorResponse(resp.StatusCode, body)
	}

	// Parse JSON response
	var djangoResp struct {
		Success          bool                   `json:"success"`
		FaceID           string                 `json:"face_id"`
		Message          string                 `json:"message"`
		Error            string                 `json:"error"`
		NumFacesDetected int32                  `json:"num_faces_detected"`
		DuplicateDetails map[string]interface{} `json:"duplicate_details"`
	}

	if err := json.Unmarshal(body, &djangoResp); err != nil {
		return nil, status.Error(codes.Internal, "failed to parse response")
	}

	// Convert to gRPC response
	response := &pb.RegisterFaceResponse{
		Success:          djangoResp.Success,
		FaceId:           djangoResp.FaceID,
		Message:          djangoResp.Message,
		Error:            djangoResp.Error,
		NumFacesDetected: djangoResp.NumFacesDetected,
	}

	// Parse duplicate details if present
	if djangoResp.DuplicateDetails != nil {
		response.DuplicateDetails = s.parseDuplicateDetails(djangoResp.DuplicateDetails)
	}

	return response, nil
}

// VerifyFace verifies a face against registered faces
func (s *FacialRecognitionService) VerifyFace(ctx context.Context, req *pb.VerifyFaceRequest) (*pb.VerifyFaceResponse, error) {
	if s.circuitBreakerOpen {
		return nil, status.Error(codes.Unavailable, "facial recognition service is currently unavailable")
	}

	// Validate input
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if len(req.ImageData) == 0 {
		return nil, status.Error(codes.InvalidArgument, "image data is required")
	}

	// Create multipart request
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add form fields
	_ = writer.WriteField("user_id", req.UserId)
	if req.Threshold > 0 {
		_ = writer.WriteField("threshold", fmt.Sprintf("%.4f", req.Threshold))
	}

	// Add image file
	part, err := writer.CreateFormFile("image", req.ImageFilename)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create form file")
	}
	_, err = part.Write(req.ImageData)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to write image data")
	}
	writer.Close()

	// Make request with retries
	var resp *http.Response
	var lastErr error

	for i := 0; i < s.maxRetries; i++ {
		resp, lastErr = s.makeRequest(ctx, "POST", "/api/facials/verify", &buf, writer.FormDataContentType())
		if lastErr == nil {
			break
		}
		if i < s.maxRetries-1 {
			time.Sleep(s.retryDelay * time.Duration(i+1))
		}
	}

	if lastErr != nil {
		s.circuitBreakerOpen = true
		go s.resetCircuitBreaker()
		return nil, status.Error(codes.Unavailable, fmt.Sprintf("failed to communicate with facial recognition service: %v", lastErr))
	}

	defer resp.Body.Close()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read response body")
	}

	if resp.StatusCode != http.StatusOK {
		return s.handleVerifyErrorResponse(resp.StatusCode, body)
	}

	// Parse JSON response
	var djangoResp struct {
		Success       bool    `json:"success"`
		Verified      bool    `json:"verified"`
		Confidence    float64 `json:"confidence"`
		MatchedFaceID string  `json:"matched_face_id"`
		Threshold     float64 `json:"threshold"`
		Distance      float64 `json:"distance"`
		Message       string  `json:"message"`
		Error         string  `json:"error"`
	}

	if err := json.Unmarshal(body, &djangoResp); err != nil {
		return nil, status.Error(codes.Internal, "failed to parse response")
	}

	// Convert to gRPC response
	response := &pb.VerifyFaceResponse{
		Success:       djangoResp.Success,
		Verified:      djangoResp.Verified,
		Confidence:    djangoResp.Confidence,
		MatchedFaceId: djangoResp.MatchedFaceID,
		Threshold:     djangoResp.Threshold,
		Distance:      djangoResp.Distance,
		Message:       djangoResp.Message,
		Error:         djangoResp.Error,
	}

	return response, nil
}

// HealthCheck checks the health of the facial recognition service
func (s *FacialRecognitionService) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	// Make a simple health check request to the facial recognition endpoint
	resp, err := s.makeRequest(ctx, "GET", "/api/facials/health", nil, "application/json")
	if err != nil {
		return &pb.HealthCheckResponse{
			Healthy:        false,
			Message:        fmt.Sprintf("Facial recognition service unavailable: %v", err),
			ServiceVersion: "unknown",
			Timestamp:      timestamppb.Now(),
		}, nil
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode == http.StatusOK
	message := "Facial recognition service is healthy"
	if !healthy {
		message = fmt.Sprintf("Facial recognition service returned status: %d", resp.StatusCode)
	}

	return &pb.HealthCheckResponse{
		Healthy:        healthy,
		Message:        message,
		ServiceVersion: "1.0.0",
		Timestamp:      timestamppb.Now(),
	}, nil
}

// Helper methods

func (s *FacialRecognitionService) makeRequest(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	url := s.serviceURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return s.client.Do(req)
}

func (s *FacialRecognitionService) handleErrorResponse(statusCode int, body []byte) (*pb.RegisterFaceResponse, error) {
	var errorResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	json.Unmarshal(body, &errorResp)

	response := &pb.RegisterFaceResponse{
		Success: false,
		Error:   errorResp.Error,
		Message: errorResp.Message,
	}

	switch statusCode {
	case http.StatusBadRequest:
		return response, status.Error(codes.InvalidArgument, errorResp.Error)
	case http.StatusConflict:
		return response, status.Error(codes.AlreadyExists, errorResp.Error)
	case http.StatusUnprocessableEntity:
		return response, status.Error(codes.FailedPrecondition, errorResp.Error)
	case http.StatusTooManyRequests:
		return response, status.Error(codes.ResourceExhausted, errorResp.Error)
	default:
		return response, status.Error(codes.Internal, errorResp.Error)
	}
}

func (s *FacialRecognitionService) handleVerifyErrorResponse(statusCode int, body []byte) (*pb.VerifyFaceResponse, error) {
	var errorResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	json.Unmarshal(body, &errorResp)

	response := &pb.VerifyFaceResponse{
		Success: false,
		Error:   errorResp.Error,
		Message: errorResp.Message,
	}

	switch statusCode {
	case http.StatusBadRequest:
		return response, status.Error(codes.InvalidArgument, errorResp.Error)
	case http.StatusUnprocessableEntity:
		return response, status.Error(codes.FailedPrecondition, errorResp.Error)
	case http.StatusTooManyRequests:
		return response, status.Error(codes.ResourceExhausted, errorResp.Error)
	default:
		return response, status.Error(codes.Internal, errorResp.Error)
	}
}

func (s *FacialRecognitionService) parseDuplicateDetails(data map[string]interface{}) *pb.DuplicateDetails {
	details := &pb.DuplicateDetails{}

	if isDuplicate, ok := data["is_duplicate"].(bool); ok {
		details.IsDuplicate = isDuplicate
	}

	if threshold, ok := data["threshold"].(float64); ok {
		details.Threshold = threshold
	}

	if totalMatches, ok := data["total_matches"].(float64); ok {
		details.TotalMatches = int32(totalMatches)
	}

	if message, ok := data["message"].(string); ok {
		details.Message = message
	}

	if securityNote, ok := data["security_note"].(string); ok {
		details.SecurityNote = securityNote
	}

	// Parse primary match
	if primaryMatch, ok := data["primary_match"].(map[string]interface{}); ok {
		details.PrimaryMatch = s.parsePrimaryMatch(primaryMatch)
	}

	// Parse all matches
	if allMatches, ok := data["all_matches"].([]interface{}); ok {
		for _, match := range allMatches {
			if matchMap, ok := match.(map[string]interface{}); ok {
				details.AllMatches = append(details.AllMatches, s.parseMatch(matchMap))
			}
		}
	}

	return details
}

func (s *FacialRecognitionService) parseMatch(data map[string]interface{}) *pb.Match {
	match := &pb.Match{}

	if userID, ok := data["user_id"].(string); ok {
		match.UserId = userID
	}

	if faceID, ok := data["face_id"].(string); ok {
		match.FaceId = faceID
	}

	if confidence, ok := data["confidence"].(float64); ok {
		match.Confidence = confidence
	}

	if registeredAt, ok := data["registered_at"].(string); ok {
		match.RegisteredAt = registeredAt
	}

	return match
}

func (s *FacialRecognitionService) parsePrimaryMatch(data map[string]interface{}) *pb.PrimaryMatch {
	match := &pb.PrimaryMatch{}

	if userID, ok := data["user_id"].(string); ok {
		match.UserId = userID
	}

	if faceID, ok := data["face_id"].(string); ok {
		match.FaceId = faceID
	}

	if confidence, ok := data["confidence"].(float64); ok {
		match.Confidence = confidence
	}

	if registeredAt, ok := data["registered_at"].(string); ok {
		match.RegisteredAt = registeredAt
	}

	return match
}

func (s *FacialRecognitionService) resetCircuitBreaker() {
	time.Sleep(30 * time.Second) // Wait 30 seconds before allowing requests again
	s.circuitBreakerOpen = false
}

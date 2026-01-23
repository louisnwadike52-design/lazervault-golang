package grpcApi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

// AI microservice response structures
type DocumentOCRResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Data    map[string]string `json:"data"`
	Error   string            `json:"error"`
}

type DocumentVerifyResponse struct {
	Success       bool              `json:"success"`
	Message       string            `json:"message"`
	IsAuthentic   bool              `json:"is_authentic"`
	Confidence    float64           `json:"confidence"`
	ExtractedData map[string]string `json:"extracted_data"`
}

type FacialRegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	FaceID  string `json:"face_id"`
}

type FacialVerifyResponse struct {
	Success    bool    `json:"success"`
	Message    string  `json:"message"`
	Verified   bool    `json:"verified"`
	Confidence float64 `json:"confidence"`
}

// getAIMicroserviceURL returns the base URL for the AI microservice
func getAIMicroserviceURL() string {
	url := os.Getenv("AI_MICROSERVICE_URL")
	if url == "" {
		return "http://localhost:8000"
	}
	return url
}

// getHTTPClient returns a configured HTTP client for AI microservice calls
func getHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
	}
}

// callDocumentOCR calls the AI microservice to process a document
func callDocumentOCR(ctx context.Context, imageBytes []byte, documentType string) (*DocumentOCRResponse, error) {
	baseURL := getAIMicroserviceURL()
	endpoint := fmt.Sprintf("%s/api/ocr/process", baseURL)

	// Create multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add document_type field
	if err := writer.WriteField("document_type", documentType); err != nil {
		return nil, fmt.Errorf("failed to write document_type field: %w", err)
	}

	// Add image file
	part, err := writer.CreateFormFile("image", "document.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(imageBytes); err != nil {
		return nil, fmt.Errorf("failed to write image data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request
	client := getHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI service: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var ocrResponse DocumentOCRResponse
	if err := json.NewDecoder(resp.Body).Decode(&ocrResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &ocrResponse, nil
}

// callDocumentVerify calls the AI microservice to verify a document's authenticity
func callDocumentVerify(ctx context.Context, frontImageBytes []byte, backImageBytes []byte, documentType string) (*DocumentVerifyResponse, error) {
	baseURL := getAIMicroserviceURL()
	endpoint := fmt.Sprintf("%s/api/ocr/verify", baseURL)

	// Create multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add document_type field
	if err := writer.WriteField("document_type", documentType); err != nil {
		return nil, fmt.Errorf("failed to write document_type field: %w", err)
	}

	// Add front image file
	frontPart, err := writer.CreateFormFile("front_image", "front.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create front image form file: %w", err)
	}
	if _, err := frontPart.Write(frontImageBytes); err != nil {
		return nil, fmt.Errorf("failed to write front image data: %w", err)
	}

	// Add back image file if provided
	if len(backImageBytes) > 0 {
		backPart, err := writer.CreateFormFile("back_image", "back.jpg")
		if err != nil {
			return nil, fmt.Errorf("failed to create back image form file: %w", err)
		}
		if _, err := backPart.Write(backImageBytes); err != nil {
			return nil, fmt.Errorf("failed to write back image data: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request
	client := getHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI service: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var verifyResponse DocumentVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &verifyResponse, nil
}

// callFacialRegister calls the AI microservice to register a face
func callFacialRegister(ctx context.Context, faceImageBytes []byte, userID string) (*FacialRegisterResponse, error) {
	baseURL := getAIMicroserviceURL()
	endpoint := fmt.Sprintf("%s/api/facials/register", baseURL)

	// Create multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add user_id field
	if err := writer.WriteField("user_id", userID); err != nil {
		return nil, fmt.Errorf("failed to write user_id field: %w", err)
	}

	// Add face image file
	part, err := writer.CreateFormFile("face_image", "face.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(faceImageBytes); err != nil {
		return nil, fmt.Errorf("failed to write image data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request
	client := getHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI service: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse response
	var registerResponse FacialRegisterResponse
	if err := json.Unmarshal(bodyBytes, &registerResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w (body: %s)", err, string(bodyBytes))
	}

	return &registerResponse, nil
}

// callFacialVerify calls the AI microservice to verify a face
func callFacialVerify(ctx context.Context, faceImageBytes []byte, userID string) (*FacialVerifyResponse, error) {
	baseURL := getAIMicroserviceURL()
	endpoint := fmt.Sprintf("%s/api/facials/verify", baseURL)

	// Create multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add user_id field
	if err := writer.WriteField("user_id", userID); err != nil {
		return nil, fmt.Errorf("failed to write user_id field: %w", err)
	}

	// Add face image file
	part, err := writer.CreateFormFile("face_image", "face.jpg")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(faceImageBytes); err != nil {
		return nil, fmt.Errorf("failed to write image data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request
	client := getHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI service: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse response
	var verifyResponse FacialVerifyResponse
	if err := json.Unmarshal(bodyBytes, &verifyResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w (body: %s)", err, string(bodyBytes))
	}

	return &verifyResponse, nil
}

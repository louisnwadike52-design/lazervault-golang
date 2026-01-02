package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PaystackConfig holds Paystack API configuration
type PaystackConfig struct {
	SecretKey string
	PublicKey string
	BaseURL   string // https://api.paystack.co
	Enabled   bool
}

// PaystackBillClient handles Paystack bill payment API interactions
type PaystackBillClient struct {
	config     PaystackConfig
	httpClient *http.Client
}

// NewPaystackBillClient creates a new Paystack bill payment client
func NewPaystackBillClient(config PaystackConfig) *PaystackBillClient {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.paystack.co"
	}

	return &PaystackBillClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetProviderName returns the provider name
func (c *PaystackBillClient) GetProviderName() string {
	return "paystack"
}

// GetProviders fetches available electricity providers from Paystack
func (c *PaystackBillClient) GetProviders(ctx context.Context, country string) ([]ProviderInfo, error) {
	url := fmt.Sprintf("%s/bill/services", c.config.BaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.SecretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    []struct {
			ID            int     `json:"id"`
			ServiceName   string  `json:"service_name"`
			ServiceSlug   string  `json:"service_slug"`
			Category      string  `json:"category"`
			MinAmount     float64 `json:"minimum_amount"`
			MaxAmount     float64 `json:"maximum_amount"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter for electricity providers
	providers := make([]ProviderInfo, 0)
	for _, item := range result.Data {
		// Only include electricity/power services
		if item.Category != "electricity" && item.Category != "power" {
			continue
		}

		providers = append(providers, ProviderInfo{
			Code:       item.ServiceSlug,
			Name:       item.ServiceName,
			MinAmount:  item.MinAmount / 100, // Paystack amounts are in kobo
			MaxAmount:  item.MaxAmount / 100,
			ServiceFee: 100, // Default fee, update based on actual
			FeeType:    "fixed",
		})
	}

	return providers, nil
}

// ValidateMeter validates a meter number with Paystack
func (c *PaystackBillClient) ValidateMeter(ctx context.Context, req MeterValidationRequest) (*MeterValidationResponse, error) {
	url := fmt.Sprintf("%s/bill/verify", c.config.BaseURL)

	payload := map[string]string{
		"service_id": req.ProviderCode,
		"customer":   req.MeterNumber,
		"type":       req.MeterType,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.config.SecretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("validation failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			CustomerName string  `json:"customer_name"`
			Address      string  `json:"address"`
			Minimum      float64 `json:"minimum"`
			Maximum      float64 `json:"maximum"`
			Outstanding  float64 `json:"outstanding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &MeterValidationResponse{
		IsValid:         result.Status,
		CustomerName:    result.Data.CustomerName,
		CustomerAddress: result.Data.Address,
		MeterType:       req.MeterType,
		OutstandingDebt: result.Data.Outstanding / 100, // Convert from kobo
		Message:         result.Message,
	}, nil
}

// InitiatePayment processes a bill payment via Paystack
func (c *PaystackBillClient) InitiatePayment(ctx context.Context, req BillPaymentRequest) (*BillPaymentResponse, error) {
	url := c.config.BaseURL + "/bill/pay"

	payload := map[string]interface{}{
		"service_id": req.ProviderCode,
		"customer":   req.MeterNumber,
		"amount":     req.Amount * 100, // Convert to kobo
		"reference":  req.Reference,
		"type":       req.MeterType,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.config.SecretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("payment failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			Reference       string  `json:"reference"`
			TransactionRef  string  `json:"transaction_reference"`
			Token           string  `json:"token"`
			Units           float64 `json:"units"`
			Amount          float64 `json:"amount"`
			Fee             float64 `json:"fee"`
			Status          string  `json:"status"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &BillPaymentResponse{
		Success:          result.Status,
		Reference:        req.Reference,
		GatewayReference: result.Data.TransactionRef,
		Token:            result.Data.Token,
		Units:            result.Data.Units,
		Amount:           result.Data.Amount / 100, // Convert from kobo
		ServiceFee:       result.Data.Fee / 100,
		TotalAmount:      (result.Data.Amount + result.Data.Fee) / 100,
		Status:           result.Data.Status,
		Message:          result.Message,
	}, nil
}

// VerifyPayment verifies a payment transaction
func (c *PaystackBillClient) VerifyPayment(ctx context.Context, reference string) (*BillPaymentResponse, error) {
	url := fmt.Sprintf("%s/transaction/verify/%s", c.config.BaseURL, reference)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.SecretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("verification failed (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			Reference     string  `json:"reference"`
			Amount        float64 `json:"amount"`
			Status        string  `json:"status"`
			Fees          float64 `json:"fees"`
			PaidAt        string  `json:"paid_at"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	success := result.Status && result.Data.Status == "success"
	status := "failed"
	if success {
		status = "completed"
	}

	return &BillPaymentResponse{
		Success:    success,
		Reference:  result.Data.Reference,
		Amount:     result.Data.Amount / 100, // Convert from kobo
		ServiceFee: result.Data.Fees / 100,
		TotalAmount: (result.Data.Amount + result.Data.Fees) / 100,
		Status:     status,
		Message:    result.Message,
	}, nil
}

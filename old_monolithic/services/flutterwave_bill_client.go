package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// FlutterwaveConfig holds Flutterwave API configuration
type FlutterwaveConfig struct {
	SecretKey string
	PublicKey string
	BaseURL   string // https://api.flutterwave.com
	Enabled   bool
}

// FlutterwaveBillClient handles Flutterwave bill payment API interactions
type FlutterwaveBillClient struct {
	config     FlutterwaveConfig
	httpClient *http.Client
}

// NewFlutterwaveBillClient creates a new Flutterwave bill payment client
func NewFlutterwaveBillClient(config FlutterwaveConfig) *FlutterwaveBillClient {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.flutterwave.com"
	}

	return &FlutterwaveBillClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetProviderName returns the provider name
func (c *FlutterwaveBillClient) GetProviderName() string {
	return "flutterwave"
}

// GetProviders fetches available electricity providers from Flutterwave
func (c *FlutterwaveBillClient) GetProviders(ctx context.Context, country string) ([]ProviderInfo, error) {
	url := fmt.Sprintf("%s/v3/bill-categories", c.config.BaseURL)

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
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    []struct {
			ID         int    `json:"id"`
			BillerCode string `json:"biller_code"`
			Name       string `json:"name"`
			Amount     string `json:"default_commission"`
			Country    string `json:"country"`
			IsAirtime  bool   `json:"is_airtime"`
			BillerName string `json:"biller_name"`
			ItemCode   string `json:"item_code"`
			ShortName  string `json:"short_name"`
			Fee        int    `json:"fee"`
			LabelName  string `json:"label_name"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter for electricity providers only
	providers := make([]ProviderInfo, 0)
	for _, item := range result.Data {
		// Check if it's an electricity provider (not airtime)
		if item.IsAirtime {
			continue
		}

		// Filter by country if specified
		if country != "" && item.Country != country {
			continue
		}

		// Only include power/electricity providers
		if !isElectricityProvider(item.BillerName, item.ShortName) {
			continue
		}

		providers = append(providers, ProviderInfo{
			Code:       item.BillerCode,
			Name:       item.BillerName,
			MinAmount:  100,    // Default minimum
			MaxAmount:  500000, // Default maximum
			ServiceFee: float64(item.Fee) / 100,
			FeeType:    "fixed",
		})
	}

	return providers, nil
}

// isElectricityProvider checks if a provider is electricity-related
func isElectricityProvider(billerName, shortName string) bool {
	electricityKeywords := []string{
		"ELECTRIC", "POWER", "DISCO", "EKEDC", "IKEDC", "AEDC", "PHED",
		"EEDC", "IBEDC", "JED", "KAEDCO", "KEDCO", "NEPA",
	}

	nameUpper := stringToUpper(billerName + " " + shortName)
	for _, keyword := range electricityKeywords {
		if containsString(nameUpper, keyword) {
			return true
		}
	}
	return false
}

// ValidateMeter validates a meter number with Flutterwave
func (c *FlutterwaveBillClient) ValidateMeter(ctx context.Context, req MeterValidationRequest) (*MeterValidationResponse, error) {
	url := fmt.Sprintf("%s/v3/bill-items/%s/validate", c.config.BaseURL, req.ProviderCode)

	payload := map[string]string{
		"code":     req.MeterNumber,
		"customer": req.MeterNumber,
		"type":     req.MeterType,
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
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    struct {
			ResponseCode    string  `json:"response_code"`
			Address         string  `json:"address"`
			ResponseMessage string  `json:"response_message"`
			Name            string  `json:"name"`
			BillerCode      string  `json:"biller_code"`
			Customer        string  `json:"customer"`
			ProductCode     string  `json:"product_code"`
			Email           string  `json:"email"`
			Fee             float64 `json:"fee"`
			Maximum         float64 `json:"maximum"`
			Minimum         float64 `json:"minimum"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	isValid := result.Data.ResponseCode == "00" || result.Data.ResponseCode == "0"

	return &MeterValidationResponse{
		IsValid:         isValid,
		CustomerName:    result.Data.Name,
		CustomerAddress: result.Data.Address,
		MeterType:       req.MeterType,
		Message:         result.Data.ResponseMessage,
	}, nil
}

// InitiatePayment processes a bill payment via Flutterwave
func (c *FlutterwaveBillClient) InitiatePayment(ctx context.Context, req BillPaymentRequest) (*BillPaymentResponse, error) {
	url := c.config.BaseURL + "/v3/bills"

	payload := map[string]interface{}{
		"country":     "NG",
		"customer":    req.MeterNumber,
		"amount":      req.Amount,
		"recurrence":  "ONCE",
		"type":        req.ProviderCode,
		"reference":   req.Reference,
		"biller_name": req.ProviderCode,
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
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    struct {
			PhoneNumber string  `json:"phone_number"`
			Amount      float64 `json:"amount"`
			Network     string  `json:"network"`
			FlwRef      string  `json:"flw_ref"`
			TxRef       string  `json:"tx_ref"`
			Reference   string  `json:"reference"`
			Token       string  `json:"token"`
			Units       string  `json:"units"`
			Fee         float64 `json:"fee"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse units as float
	units := 0.0
	if result.Data.Units != "" {
		units, _ = strconv.ParseFloat(result.Data.Units, 64)
	}

	success := result.Status == "success"

	return &BillPaymentResponse{
		Success:          success,
		Reference:        req.Reference,
		GatewayReference: result.Data.FlwRef,
		Token:            result.Data.Token,
		Units:            units,
		Amount:           req.Amount,
		ServiceFee:       result.Data.Fee,
		TotalAmount:      req.Amount + result.Data.Fee,
		Status:           "completed",
		Message:          result.Message,
	}, nil
}

// VerifyPayment verifies a payment transaction
func (c *FlutterwaveBillClient) VerifyPayment(ctx context.Context, reference string) (*BillPaymentResponse, error) {
	url := fmt.Sprintf("%s/v3/transactions/%s/verify", c.config.BaseURL, reference)

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
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    struct {
			ID                int     `json:"id"`
			TxRef             string  `json:"tx_ref"`
			FlwRef            string  `json:"flw_ref"`
			Amount            float64 `json:"amount"`
			Currency          string  `json:"currency"`
			ChargedAmount     float64 `json:"charged_amount"`
			AppFee            float64 `json:"app_fee"`
			MerchantFee       float64 `json:"merchant_fee"`
			ProcessorResponse string  `json:"processor_response"`
			AuthModel         string  `json:"auth_model"`
			IP                string  `json:"ip"`
			Narration         string  `json:"narration"`
			Status            string  `json:"status"`
			PaymentType       string  `json:"payment_type"`
			CreatedAt         string  `json:"created_at"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	success := result.Status == "success" && result.Data.Status == "successful"
	status := "failed"
	if success {
		status = "completed"
	}

	return &BillPaymentResponse{
		Success:          success,
		Reference:        result.Data.TxRef,
		GatewayReference: result.Data.FlwRef,
		Amount:           result.Data.Amount,
		ServiceFee:       result.Data.AppFee,
		TotalAmount:      result.Data.ChargedAmount,
		Status:           status,
		Message:          result.Data.ProcessorResponse,
	}, nil
}

// Helper functions
func stringToUpper(s string) string {
	result := ""
	for _, c := range s {
		if c >= 'a' && c <= 'z' {
			result += string(c - 32)
		} else {
			result += string(c)
		}
	}
	return result
}

func containsString(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

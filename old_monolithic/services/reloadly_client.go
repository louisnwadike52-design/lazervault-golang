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

// ReloadlyConfig holds Reloadly API configuration
type ReloadlyConfig struct {
	ClientID     string
	ClientSecret string
	BaseURL      string // https://giftcards.reloadly.com or https://giftcards-sandbox.reloadly.com
	AuthURL      string // https://auth.reloadly.com/oauth/token
	Enabled      bool   // Set to true when credentials are configured
}

// ReloadlyClient handles Reloadly API interactions
type ReloadlyClient struct {
	config      ReloadlyConfig
	httpClient  *http.Client
	accessToken string
	tokenExpiry time.Time
}

// NewReloadlyClient creates a new Reloadly API client
func NewReloadlyClient(config ReloadlyConfig) *ReloadlyClient {
	return &ReloadlyClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// authenticate gets a new access token from Reloadly
func (c *ReloadlyClient) authenticate(ctx context.Context) error {
	if !c.config.Enabled {
		return fmt.Errorf("Reloadly integration not enabled")
	}

	// Check if token is still valid
	if time.Now().Before(c.tokenExpiry) {
		return nil
	}

	authReq := map[string]string{
		"client_id":     c.config.ClientID,
		"client_secret": c.config.ClientSecret,
		"grant_type":    "client_credentials",
		"audience":      "https://giftcards.reloadly.com",
	}

	jsonData, err := json.Marshal(authReq)
	if err != nil {
		return fmt.Errorf("failed to marshal auth request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.AuthURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth failed with status %d: %s", resp.StatusCode, string(body))
	}

	var authResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	c.accessToken = authResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(authResp.ExpiresIn) * time.Second)

	return nil
}

// makeRequest makes an authenticated request to Reloadly API
func (c *ReloadlyClient) makeRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	// Ensure we have a valid token
	if err := c.authenticate(ctx); err != nil {
		return err
	}

	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonData)
	}

	url := c.config.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// ReloadlyProduct represents a gift card product from Reloadly
type ReloadlyProduct struct {
	ProductID                   int                       `json:"productId"`
	ProductName                 string                    `json:"productName"`
	CountryCode                 string                    `json:"countryCode"`
	ProductCategoryID           int                       `json:"productCategoryId"`
	SenderFee                   float64                   `json:"senderFee"`
	DiscountPercentage          float64                   `json:"discountPercentage"`
	DenominationType            string                    `json:"denominationType"` // FIXED or RANGE
	RecipientCurrencyCode       string                    `json:"recipientCurrencyCode"`
	MinRecipientDenomination    float64                   `json:"minRecipientDenomination"`
	MaxRecipientDenomination    float64                   `json:"maxRecipientDenomination"`
	SenderCurrencyCode          string                    `json:"senderCurrencyCode"`
	MinSenderDenomination       float64                   `json:"minSenderDenomination"`
	MaxSenderDenomination       float64                   `json:"maxSenderDenomination"`
	FixedRecipientDenominations []float64                 `json:"fixedRecipientDenominations"`
	LogoURLs                    []string                  `json:"logoUrls"`
	Brand                       ReloadlyBrand             `json:"brand"`
	RedeemInstruction           ReloadlyRedeemInstruction `json:"redeemInstruction"`
}

type ReloadlyBrand struct {
	BrandID   int    `json:"brandId"`
	BrandName string `json:"brandName"`
}

type ReloadlyRedeemInstruction struct {
	Concise string `json:"concise"`
	Verbose string `json:"verbose"`
}

// ReloadlyOrder represents an order request
type ReloadlyOrder struct {
	ProductID             int             `json:"productId"`
	CountryCode           string          `json:"countryCode"`
	Quantity              int             `json:"quantity"`
	UnitPrice             float64         `json:"unitPrice"`
	CustomIdentifier      string          `json:"customIdentifier"`
	SenderName            string          `json:"senderName,omitempty"`
	RecipientEmail        string          `json:"recipientEmail,omitempty"`
	RecipientPhoneDetails *RecipientPhone `json:"recipientPhoneDetails,omitempty"`
}

type RecipientPhone struct {
	CountryCode string `json:"countryCode"`
	PhoneNumber string `json:"phoneNumber"`
}

// ReloadlyOrderResponse represents the order response
type ReloadlyOrderResponse struct {
	TransactionID          int64           `json:"transactionId"`
	Amount                 float64         `json:"amount"`
	Discount               float64         `json:"discount"`
	CurrencyCode           string          `json:"currencyCode"`
	Fee                    float64         `json:"fee"`
	RecipientEmail         string          `json:"recipientEmail"`
	CustomIdentifier       string          `json:"customIdentifier"`
	Status                 string          `json:"status"`
	Product                ReloadlyProduct `json:"product"`
	TransactionCreatedTime string          `json:"transactionCreatedTime"`
	PinDetail              *PinDetail      `json:"pinDetail,omitempty"`
}

type PinDetail struct {
	Serial1 string `json:"serial1"`
	Serial2 string `json:"serial2"`
	Serial3 string `json:"serial3"`
	Code1   string `json:"code1"`
	Code2   string `json:"code2"`
	Code3   string `json:"code3"`
}

// GetProducts retrieves all available gift card products
func (c *ReloadlyClient) GetProducts(ctx context.Context, countryCode string, page, size int) ([]ReloadlyProduct, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("Reloadly integration not enabled - configure API credentials")
	}

	path := fmt.Sprintf("/products?countryCode=%s&page=%d&size=%d", countryCode, page, size)

	var result struct {
		Content []ReloadlyProduct `json:"content"`
	}

	if err := c.makeRequest(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}

	return result.Content, nil
}

// GetProductByID retrieves a specific product by ID
func (c *ReloadlyClient) GetProductByID(ctx context.Context, productID int) (*ReloadlyProduct, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("Reloadly integration not enabled - configure API credentials")
	}

	path := fmt.Sprintf("/products/%d", productID)

	var product ReloadlyProduct
	if err := c.makeRequest(ctx, "GET", path, nil, &product); err != nil {
		return nil, err
	}

	return &product, nil
}

// PlaceOrder places a gift card order
func (c *ReloadlyClient) PlaceOrder(ctx context.Context, order ReloadlyOrder) (*ReloadlyOrderResponse, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("Reloadly integration not enabled - configure API credentials")
	}

	var orderResp ReloadlyOrderResponse
	if err := c.makeRequest(ctx, "POST", "/orders", order, &orderResp); err != nil {
		return nil, err
	}

	return &orderResp, nil
}

// GetOrderByTransactionID retrieves order details by transaction ID
func (c *ReloadlyClient) GetOrderByTransactionID(ctx context.Context, transactionID int64) (*ReloadlyOrderResponse, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("Reloadly integration not enabled - configure API credentials")
	}

	path := fmt.Sprintf("/orders/transactions/%d", transactionID)

	var orderResp ReloadlyOrderResponse
	if err := c.makeRequest(ctx, "GET", path, nil, &orderResp); err != nil {
		return nil, err
	}

	return &orderResp, nil
}

// GetBalance retrieves account balance
func (c *ReloadlyClient) GetBalance(ctx context.Context) (float64, string, error) {
	if !c.config.Enabled {
		return 0, "", fmt.Errorf("Reloadly integration not enabled - configure API credentials")
	}

	var result struct {
		Balance      float64 `json:"balance"`
		CurrencyCode string  `json:"currencyCode"`
	}

	if err := c.makeRequest(ctx, "GET", "/accounts/balance", nil, &result); err != nil {
		return 0, "", err
	}

	return result.Balance, result.CurrencyCode, nil
}

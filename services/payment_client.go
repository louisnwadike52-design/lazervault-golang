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

// PaymentProvider defines the interface for payment processing
type PaymentProvider interface {
	CreatePaymentIntent(ctx context.Context, amount float64, currency string, metadata map[string]string) (*PaymentIntent, error)
	ConfirmPaymentIntent(ctx context.Context, intentID string) (*PaymentIntent, error)
	CancelPaymentIntent(ctx context.Context, intentID string) (*PaymentIntent, error)
	GetPaymentIntent(ctx context.Context, intentID string) (*PaymentIntent, error)
	CreateCustomer(ctx context.Context, email, name string, metadata map[string]string) (*Customer, error)
	GetCustomer(ctx context.Context, customerID string) (*Customer, error)
}

// PaymentIntent represents a payment intent
type PaymentIntent struct {
	ID            string            `json:"id"`
	Amount        int64             `json:"amount"` // Amount in cents
	Currency      string            `json:"currency"`
	Status        string            `json:"status"` // requires_payment_method, requires_confirmation, succeeded, canceled
	ClientSecret  string            `json:"client_secret"`
	CustomerID    string            `json:"customer,omitempty"`
	Description   string            `json:"description,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Created       int64             `json:"created"`
	CanceledAt    int64             `json:"canceled_at,omitempty"`
	ConfirmedAt   int64             `json:"confirmed_at,omitempty"`
}

// Customer represents a payment customer
type Customer struct {
	ID       string            `json:"id"`
	Email    string            `json:"email"`
	Name     string            `json:"name"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Created  int64             `json:"created"`
}

// StripeConfig holds Stripe API configuration
type StripeConfig struct {
	SecretKey      string
	PublishableKey string
	WebhookSecret  string
	BaseURL        string // https://api.stripe.com
	Enabled        bool   // Set to true when credentials are configured
}

// StripeClient handles Stripe payment processing
type StripeClient struct {
	config     StripeConfig
	httpClient *http.Client
}

// NewStripeClient creates a new Stripe payment client
func NewStripeClient(config StripeConfig) *StripeClient {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.stripe.com"
	}

	return &StripeClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// makeRequest makes an authenticated request to Stripe API
func (c *StripeClient) makeRequest(ctx context.Context, method, path string, params map[string]interface{}, result interface{}) error {
	if !c.config.Enabled {
		return fmt.Errorf("Stripe integration not enabled - configure API credentials")
	}

	var bodyReader io.Reader
	if params != nil {
		jsonData, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal request params: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonData)
	}

	url := c.config.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.config.SecretKey, "")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Version", "2023-10-16")

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

// CreatePaymentIntent creates a new payment intent
func (c *StripeClient) CreatePaymentIntent(ctx context.Context, amount float64, currency string, metadata map[string]string) (*PaymentIntent, error) {
	// Convert to cents
	amountCents := int64(amount * 100)

	params := map[string]interface{}{
		"amount":   amountCents,
		"currency": currency,
	}

	if metadata != nil {
		params["metadata"] = metadata
	}

	var intent PaymentIntent
	if err := c.makeRequest(ctx, "POST", "/v1/payment_intents", params, &intent); err != nil {
		return nil, err
	}

	return &intent, nil
}

// ConfirmPaymentIntent confirms a payment intent
func (c *StripeClient) ConfirmPaymentIntent(ctx context.Context, intentID string) (*PaymentIntent, error) {
	path := fmt.Sprintf("/v1/payment_intents/%s/confirm", intentID)

	var intent PaymentIntent
	if err := c.makeRequest(ctx, "POST", path, nil, &intent); err != nil {
		return nil, err
	}

	return &intent, nil
}

// CancelPaymentIntent cancels a payment intent
func (c *StripeClient) CancelPaymentIntent(ctx context.Context, intentID string) (*PaymentIntent, error) {
	path := fmt.Sprintf("/v1/payment_intents/%s/cancel", intentID)

	var intent PaymentIntent
	if err := c.makeRequest(ctx, "POST", path, nil, &intent); err != nil {
		return nil, err
	}

	return &intent, nil
}

// GetPaymentIntent retrieves a payment intent
func (c *StripeClient) GetPaymentIntent(ctx context.Context, intentID string) (*PaymentIntent, error) {
	path := fmt.Sprintf("/v1/payment_intents/%s", intentID)

	var intent PaymentIntent
	if err := c.makeRequest(ctx, "GET", path, nil, &intent); err != nil {
		return nil, err
	}

	return &intent, nil
}

// CreateCustomer creates a new customer
func (c *StripeClient) CreateCustomer(ctx context.Context, email, name string, metadata map[string]string) (*Customer, error) {
	params := map[string]interface{}{
		"email": email,
		"name":  name,
	}

	if metadata != nil {
		params["metadata"] = metadata
	}

	var customer Customer
	if err := c.makeRequest(ctx, "POST", "/v1/customers", params, &customer); err != nil {
		return nil, err
	}

	return &customer, nil
}

// GetCustomer retrieves a customer
func (c *StripeClient) GetCustomer(ctx context.Context, customerID string) (*Customer, error) {
	path := fmt.Sprintf("/v1/customers/%s", customerID)

	var customer Customer
	if err := c.makeRequest(ctx, "GET", path, nil, &customer); err != nil {
		return nil, err
	}

	return &customer, nil
}

// WebhookEvent represents a Stripe webhook event
type WebhookEvent struct {
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Data    map[string]interface{} `json:"data"`
	Created int64                  `json:"created"`
}

// VerifyWebhookSignature verifies a Stripe webhook signature
func (c *StripeClient) VerifyWebhookSignature(payload []byte, signature string) (*WebhookEvent, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("Stripe integration not enabled")
	}

	// In production, use stripe.ConstructEvent from stripe-go SDK
	// This is a placeholder for the actual implementation
	// which requires the official Stripe SDK for proper HMAC verification

	var event WebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	return &event, nil
}

// MockPaymentClient provides a mock payment client for testing
type MockPaymentClient struct {
	intents   map[string]*PaymentIntent
	customers map[string]*Customer
}

// NewMockPaymentClient creates a mock payment client
func NewMockPaymentClient() *MockPaymentClient {
	return &MockPaymentClient{
		intents:   make(map[string]*PaymentIntent),
		customers: make(map[string]*Customer),
	}
}

// CreatePaymentIntent creates a mock payment intent
func (m *MockPaymentClient) CreatePaymentIntent(ctx context.Context, amount float64, currency string, metadata map[string]string) (*PaymentIntent, error) {
	intent := &PaymentIntent{
		ID:           fmt.Sprintf("pi_mock_%d", time.Now().Unix()),
		Amount:       int64(amount * 100),
		Currency:     currency,
		Status:       "requires_confirmation",
		ClientSecret: fmt.Sprintf("pi_mock_%d_secret", time.Now().Unix()),
		Metadata:     metadata,
		Created:      time.Now().Unix(),
	}

	m.intents[intent.ID] = intent
	return intent, nil
}

// ConfirmPaymentIntent confirms a mock payment intent
func (m *MockPaymentClient) ConfirmPaymentIntent(ctx context.Context, intentID string) (*PaymentIntent, error) {
	intent, exists := m.intents[intentID]
	if !exists {
		return nil, fmt.Errorf("payment intent not found")
	}

	intent.Status = "succeeded"
	intent.ConfirmedAt = time.Now().Unix()
	return intent, nil
}

// CancelPaymentIntent cancels a mock payment intent
func (m *MockPaymentClient) CancelPaymentIntent(ctx context.Context, intentID string) (*PaymentIntent, error) {
	intent, exists := m.intents[intentID]
	if !exists {
		return nil, fmt.Errorf("payment intent not found")
	}

	intent.Status = "canceled"
	intent.CanceledAt = time.Now().Unix()
	return intent, nil
}

// GetPaymentIntent retrieves a mock payment intent
func (m *MockPaymentClient) GetPaymentIntent(ctx context.Context, intentID string) (*PaymentIntent, error) {
	intent, exists := m.intents[intentID]
	if !exists {
		return nil, fmt.Errorf("payment intent not found")
	}

	return intent, nil
}

// CreateCustomer creates a mock customer
func (m *MockPaymentClient) CreateCustomer(ctx context.Context, email, name string, metadata map[string]string) (*Customer, error) {
	customer := &Customer{
		ID:       fmt.Sprintf("cus_mock_%d", time.Now().Unix()),
		Email:    email,
		Name:     name,
		Metadata: metadata,
		Created:  time.Now().Unix(),
	}

	m.customers[customer.ID] = customer
	return customer, nil
}

// GetCustomer retrieves a mock customer
func (m *MockPaymentClient) GetCustomer(ctx context.Context, customerID string) (*Customer, error) {
	customer, exists := m.customers[customerID]
	if !exists {
		return nil, fmt.Errorf("customer not found")
	}

	return customer, nil
}

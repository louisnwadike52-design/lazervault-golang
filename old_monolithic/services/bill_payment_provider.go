package services

import (
	"context"
)

// BillPaymentProvider defines the interface for bill payment gateways
type BillPaymentProvider interface {
	// Provider management
	GetProviders(ctx context.Context, country string) ([]ProviderInfo, error)

	// Meter validation
	ValidateMeter(ctx context.Context, req MeterValidationRequest) (*MeterValidationResponse, error)

	// Payment processing
	InitiatePayment(ctx context.Context, req BillPaymentRequest) (*BillPaymentResponse, error)
	VerifyPayment(ctx context.Context, reference string) (*BillPaymentResponse, error)

	// Provider name
	GetProviderName() string
}

// ProviderInfo represents information about an electricity provider
type ProviderInfo struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	LogoURL    string  `json:"logo_url"`
	MinAmount  float64 `json:"min_amount"`
	MaxAmount  float64 `json:"max_amount"`
	ServiceFee float64 `json:"service_fee"`
	FeeType    string  `json:"fee_type"` // fixed, percentage
}

// MeterValidationRequest represents a meter validation request
type MeterValidationRequest struct {
	ProviderCode string `json:"provider_code"`
	MeterNumber  string `json:"meter_number"`
	MeterType    string `json:"meter_type"` // prepaid, postpaid
}

// MeterValidationResponse represents the result of meter validation
type MeterValidationResponse struct {
	IsValid         bool    `json:"is_valid"`
	CustomerName    string  `json:"customer_name"`
	CustomerAddress string  `json:"customer_address"`
	MeterType       string  `json:"meter_type"`
	OutstandingDebt float64 `json:"outstanding_debt"`
	Message         string  `json:"message"`
}

// BillPaymentRequest represents a payment request
type BillPaymentRequest struct {
	ProviderCode  string  `json:"provider_code"`
	MeterNumber   string  `json:"meter_number"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	MeterType     string  `json:"meter_type"`
	CustomerName  string  `json:"customer_name"`
	CustomerEmail string  `json:"customer_email"`
	CustomerPhone string  `json:"customer_phone"`
	Reference     string  `json:"reference"`
}

// BillPaymentResponse represents the result of a payment
type BillPaymentResponse struct {
	Success          bool    `json:"success"`
	Reference        string  `json:"reference"`
	GatewayReference string  `json:"gateway_reference"`
	Token            string  `json:"token"` // Electricity token
	Units            float64 `json:"units"` // kWh purchased
	Amount           float64 `json:"amount"`
	ServiceFee       float64 `json:"service_fee"`
	TotalAmount      float64 `json:"total_amount"`
	Status           string  `json:"status"`
	Message          string  `json:"message"`
}

// BillPaymentProviderFactory creates providers based on configuration
type BillPaymentProviderFactory struct {
	flutterwaveClient *FlutterwaveBillClient
	paystackClient    *PaystackBillClient
}

// NewBillPaymentProviderFactory creates a new factory
func NewBillPaymentProviderFactory(flutterwave *FlutterwaveBillClient, paystack *PaystackBillClient) *BillPaymentProviderFactory {
	return &BillPaymentProviderFactory{
		flutterwaveClient: flutterwave,
		paystackClient:    paystack,
	}
}

// GetProvider returns the appropriate provider based on gateway name
func (f *BillPaymentProviderFactory) GetProvider(gateway string) BillPaymentProvider {
	switch gateway {
	case "flutterwave":
		return f.flutterwaveClient
	case "paystack":
		return f.paystackClient
	default:
		return f.flutterwaveClient // Default to Flutterwave
	}
}

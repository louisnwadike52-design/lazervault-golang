# Production Deployment Guide

This guide covers deploying the LazerVault backend integrations to production.

---

## Table of Contents

1. [Overview](#overview)
2. [Crypto Service (CoinGecko)](#1-crypto-service-coingecko)
3. [Gift Cards Service (Reloadly)](#2-gift-cards-service-reloadly)
4. [Stocks Service (Alpaca)](#3-stocks-service-alpaca)
5. [Payment Processing (Stripe)](#4-payment-processing-stripe)
6. [Configuration Management](#configuration-management)
7. [Testing](#testing)
8. [Security Best Practices](#security-best-practices)
9. [Troubleshooting](#troubleshooting)

---

## Overview

The LazerVault backend integrates with three external services:
- **CoinGecko**: Cryptocurrency market data (FREE, no API key required)
- **Reloadly**: Gift card API (requires account & OAuth2 credentials)
- **Alpaca**: Stock trading API (requires account & API keys)
- **Stripe**: Payment processing (requires account & API keys)

**Current Status:**
- Crypto: Production ready (live API integrated)
- Gift Cards: Mock data (structure ready for Reloadly)
- Stocks: Mock data (structure ready for Alpaca)
- Payments: Mock client (structure ready for Stripe)

---

## 1. Crypto Service (CoinGecko)

### Status: ✅ Production Ready

The crypto service uses the free CoinGecko API with no authentication required.

### Current Implementation
- Live API integration at `services/crypto_service.go:600`
- Real-time cryptocurrency market data
- No API key needed
- 8 public endpoints operational

### Endpoints
```
GET /v1/cryptos                  - List cryptocurrencies
GET /v1/cryptos/{id}             - Get crypto details
GET /v1/cryptos/search           - Search cryptos
GET /v1/cryptos/{id}/history     - Price history
GET /v1/cryptos/trending         - Trending coins
GET /v1/cryptos/top              - Top by market cap
GET /v1/cryptos/{id}/chart       - Chart data
GET /v1/cryptos/global           - Global market data
```

### Rate Limits
- Free tier: 10-50 requests/minute
- Consider caching for production:
  - Redis for frequently accessed data
  - Cache TTL: 60 seconds for price data

### Recommended Enhancements
1. Add Redis caching layer
2. Implement rate limiting middleware
3. Monitor API usage
4. Add fallback error handling

---

## 2. Gift Cards Service (Reloadly)

### Status: ⚠️ Mock Data (Production Structure Ready)

### Timeline: 1-2 weeks to production

---

### Step 1: Reloadly Account Setup

1. **Sign up for Reloadly**
   - Visit: https://www.reloadly.com/
   - Create account (business email required)
   - Verify email and complete KYC

2. **Get API Credentials**
   - Login to dashboard
   - Navigate to "Developers" > "API Credentials"
   - Copy Client ID and Client Secret
   - Start with **Sandbox** environment

3. **Account Funding** (Production only)
   - Add funds via bank transfer or card
   - Minimum balance varies by region
   - Test thoroughly in sandbox first

---

### Step 2: Configure Environment

Edit `app.env`:

```bash
# Reloadly Gift Cards API (Sandbox)
RELOADLY_CLIENT_ID="your_sandbox_client_id"
RELOADLY_CLIENT_SECRET="your_sandbox_client_secret"
RELOADLY_BASE_URL="https://giftcards-sandbox.reloadly.com"
RELOADLY_AUTH_URL="https://auth.reloadly.com/oauth/token"
RELOADLY_ENABLED=true
```

---

### Step 3: Update Service to Use Real API

Edit `services/gift_card_service.go`:

```go
// Replace this line in NewGiftCardService()
func NewGiftCardService(config configs.Config) *GiftCardService {
	// Get Reloadly configuration
	clientID, clientSecret, baseURL, authURL, enabled := config.GetReloadlyConfig()

	// Create Reloadly client
	reloadlyConfig := ReloadlyConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		BaseURL:      baseURL,
		AuthURL:      authURL,
		Enabled:      enabled,
	}
	reloadlyClient := NewReloadlyClient(reloadlyConfig)

	return &GiftCardService{
		reloadlyClient: reloadlyClient,
		// Remove mock data fields when using real API
	}
}
```

Update method implementations to call `s.reloadlyClient` instead of returning mock data.

---

### Step 4: Test in Sandbox

```bash
# Start server
go run main.go

# Test product retrieval
curl "http://localhost:7878/v1/gift-cards/brands?page=1&per_page=5"

# Test specific product
curl "http://localhost:7878/v1/gift-cards/brands/amazon"
```

---

### Step 5: Switch to Production

When ready for production:

```bash
# Update app.env
RELOADLY_BASE_URL="https://giftcards.reloadly.com"  # Remove -sandbox
RELOADLY_ENABLED=true

# Use production credentials
RELOADLY_CLIENT_ID="your_production_client_id"
RELOADLY_CLIENT_SECRET="your_production_client_secret"
```

---

### API Reference

**Reloadly Client:** `services/reloadly_client.go:297`

Key methods:
- `GetProducts(ctx, countryCode, page, size)` - List products
- `GetProductByID(ctx, productID)` - Get product details
- `PlaceOrder(ctx, order)` - Purchase gift card
- `GetOrderByTransactionID(ctx, transactionID)` - Get order status
- `GetBalance(ctx)` - Check account balance

**Documentation:** https://developers.reloadly.com/

---

## 3. Stocks Service (Alpaca)

### Status: ⚠️ Mock Data (Production Structure Ready)

### Timeline: 2-4 weeks to production

---

### Step 1: Alpaca Account Setup

1. **Sign up for Alpaca**
   - Visit: https://alpaca.markets/
   - Create account
   - Start with **Paper Trading** (free, no real money)

2. **Get API Keys**
   - Login to dashboard
   - Navigate to "Your API Keys" (Paper Trading)
   - Generate new key pair
   - Copy API Key ID and Secret Key

3. **Account Approval** (Live Trading only)
   - Complete identity verification
   - Link bank account
   - Wait for approval (1-3 business days)
   - Fund account (minimum $0, recommended $2000+)

---

### Step 2: Configure Environment

Edit `app.env`:

```bash
# Alpaca Stock Trading API (Paper Trading)
ALPACA_API_KEY="your_paper_api_key_id"
ALPACA_API_SECRET="your_paper_secret_key"
ALPACA_BASE_URL="https://paper-api.alpaca.markets"
ALPACA_DATA_URL="https://data.alpaca.markets"
ALPACA_ENABLED=true
ALPACA_IS_PAPER=true
```

---

### Step 3: Update Service to Use Real API

Edit `services/stock_service.go`:

```go
// Replace this line in NewStockService()
func NewStockService(config configs.Config) *StockService {
	// Get Alpaca configuration
	apiKey, apiSecret, baseURL, dataURL, enabled, isPaper := config.GetAlpacaConfig()

	// Create Alpaca client
	alpacaConfig := AlpacaConfig{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   baseURL,
		DataURL:   dataURL,
		Enabled:   enabled,
		IsPaper:   isPaper,
	}
	alpacaClient := NewAlpacaClient(alpacaConfig)

	return &StockService{
		alpacaClient: alpacaClient,
		// Remove mock data fields when using real API
	}
}
```

Update method implementations to call `s.alpacaClient` instead of returning mock data.

---

### Step 4: Test in Paper Trading

```bash
# Start server
go run main.go

# Test stock data
curl "http://localhost:7878/v1/stocks/AAPL"

# Test quote
curl "http://localhost:7878/v1/stocks/TSLA/quote"

# Test order placement (authenticated)
curl -X POST "http://localhost:7878/v1/stocks/orders" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "symbol": "AAPL",
    "quantity": 1,
    "side": "buy",
    "type": "market",
    "time_in_force": "day"
  }'
```

---

### Step 5: Switch to Live Trading

**⚠️ CRITICAL: Only switch to live trading after extensive testing in paper mode!**

```bash
# Update app.env
ALPACA_BASE_URL="https://api.alpaca.markets"  # Remove paper-
ALPACA_IS_PAPER=false

# Use live trading credentials
ALPACA_API_KEY="your_live_api_key_id"
ALPACA_API_SECRET="your_live_secret_key"
```

---

### Step 6: Implement Safety Features

Before live trading, add these safeguards in `services/stock_service.go`:

```go
// Add to PlaceOrder method
func (s *StockService) PlaceOrder(ctx context.Context, userID uint, req *pb.PlaceOrderRequest) (*pb.PlaceOrderResponse, error) {
	// 1. Validate order amount
	if req.Quantity * stock.CurrentPrice > MAX_ORDER_VALUE {
		return nil, status.Errorf(codes.InvalidArgument, "order exceeds maximum value")
	}

	// 2. Check account balance
	account, err := s.alpacaClient.GetAccount(ctx)
	if err != nil {
		return nil, err
	}

	// 3. Verify buying power
	cost := req.Quantity * stock.CurrentPrice
	if cost > account.BuyingPower {
		return nil, status.Errorf(codes.FailedPrecondition, "insufficient buying power")
	}

	// 4. Implement rate limiting per user
	// 5. Add order confirmation for large trades
	// 6. Log all trades for audit

	// ... proceed with order
}
```

---

### API Reference

**Alpaca Client:** `services/alpaca_client.go:376`

Key methods:
- `GetAccount(ctx)` - Account information
- `GetAsset(ctx, symbol)` - Asset details
- `GetSnapshot(ctx, symbol)` - Current market snapshot
- `GetBars(ctx, symbol, timeframe, start, end, limit)` - Historical OHLC data
- `CreateOrder(ctx, request)` - Place order
- `GetPositions(ctx)` - Current positions
- `CancelOrder(ctx, orderID)` - Cancel order

**Documentation:** https://alpaca.markets/docs/

---

## 4. Payment Processing (Stripe)

### Status: ⚠️ Mock Client (Production Structure Ready)

### Timeline: 1-2 weeks to production

---

### Step 1: Stripe Account Setup

1. **Sign up for Stripe**
   - Visit: https://stripe.com/
   - Create account
   - Complete business information
   - Verify email and phone

2. **Get API Keys**
   - Login to dashboard
   - Navigate to "Developers" > "API keys"
   - Copy **Test** keys first (for sandbox)
   - Copy **Publishable key** and **Secret key**

3. **Set up Webhooks**
   - Go to "Developers" > "Webhooks"
   - Add endpoint: `https://your-domain.com/webhooks/stripe`
   - Select events:
     - `payment_intent.succeeded`
     - `payment_intent.payment_failed`
     - `charge.refunded`
   - Copy **Webhook signing secret**

---

### Step 2: Configure Environment

Edit `app.env`:

```bash
# Stripe Payment API (Test Mode)
STRIPE_SECRET_KEY="sk_test_your_secret_key"
STRIPE_PUBLISHABLE_KEY="pk_test_your_publishable_key"
STRIPE_WEBHOOK_SECRET="whsec_your_webhook_secret"
STRIPE_BASE_URL="https://api.stripe.com"
STRIPE_ENABLED=true
```

---

### Step 3: Update Gift Card Service to Use Real Payments

Edit `services/gift_card_service.go`:

```go
func NewGiftCardService(config configs.Config) *GiftCardService {
	// ... existing Reloadly setup ...

	// Get Stripe configuration
	secretKey, publishableKey, webhookSecret, baseURL, enabled := config.GetStripeConfig()

	// Create Stripe client
	stripeConfig := StripeConfig{
		SecretKey:      secretKey,
		PublishableKey: publishableKey,
		WebhookSecret:  webhookSecret,
		BaseURL:        baseURL,
		Enabled:        enabled,
	}
	paymentClient := NewStripeClient(stripeConfig)

	return &GiftCardService{
		reloadlyClient: reloadlyClient,
		paymentClient:  paymentClient,
	}
}
```

Update `PurchaseGiftCard` method:

```go
func (s *GiftCardService) PurchaseGiftCard(ctx context.Context, userID uint, req *pb.PurchaseGiftCardRequest) (*pb.PurchaseGiftCardResponse, error) {
	// 1. Get brand and validate
	brand := s.getBrandByID(req.BrandId)

	// 2. Calculate price
	finalPrice := req.Amount * (1 - brand.DiscountPercentage/100)

	// 3. Create payment intent
	metadata := map[string]string{
		"user_id":   fmt.Sprintf("%d", userID),
		"brand_id":  req.BrandId,
		"amount":    fmt.Sprintf("%.2f", req.Amount),
	}

	paymentIntent, err := s.paymentClient.CreatePaymentIntent(ctx, finalPrice, "usd", metadata)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create payment: %v", err)
	}

	// 4. Wait for payment confirmation (or return client_secret for frontend)
	// 5. Once confirmed, purchase from Reloadly
	// 6. Return gift card details

	return &pb.PurchaseGiftCardResponse{
		GiftCard: giftCard,
		Transaction: transaction,
	}, nil
}
```

---

### Step 4: Implement Webhook Handler

Create `grpcApi/webhooks.go`:

```go
package grpcApi

import (
	"io"
	"net/http"
	"lazervaultGo/services"
)

type WebhookHandler struct {
	paymentClient *services.StripeClient
	giftCardService *services.GiftCardService
}

func (h *WebhookHandler) HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}

	signature := r.Header.Get("Stripe-Signature")

	event, err := h.paymentClient.VerifyWebhookSignature(payload, signature)
	if err != nil {
		http.Error(w, "Invalid signature", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "payment_intent.succeeded":
		// Handle successful payment
		// Complete gift card purchase from Reloadly

	case "payment_intent.payment_failed":
		// Handle failed payment
		// Update order status

	case "charge.refunded":
		// Handle refund
	}

	w.WriteHeader(http.StatusOK)
}
```

Register webhook route in `grpcApi/server.go`:

```go
// Add to HTTP server setup
webhookHandler := &WebhookHandler{
	paymentClient: paymentClient,
	giftCardService: giftCardService,
}

mux.HandleFunc("/webhooks/stripe", webhookHandler.HandleStripeWebhook)
```

---

### Step 5: Test in Test Mode

```bash
# Use Stripe test cards
# Success: 4242 4242 4242 4242
# Decline: 4000 0000 0000 0002

# Test webhook locally using Stripe CLI
stripe listen --forward-to localhost:7878/webhooks/stripe
```

---

### Step 6: Switch to Live Mode

**⚠️ CRITICAL: Only activate live mode after thorough testing!**

```bash
# Update app.env with live keys
STRIPE_SECRET_KEY="sk_live_your_secret_key"
STRIPE_PUBLISHABLE_KEY="pk_live_your_publishable_key"
STRIPE_WEBHOOK_SECRET="whsec_your_live_webhook_secret"
STRIPE_ENABLED=true
```

---

### API Reference

**Stripe Client:** `services/payment_client.go:323`

Key methods:
- `CreatePaymentIntent(ctx, amount, currency, metadata)` - Create payment
- `ConfirmPaymentIntent(ctx, intentID)` - Confirm payment
- `CancelPaymentIntent(ctx, intentID)` - Cancel payment
- `GetPaymentIntent(ctx, intentID)` - Get payment status
- `CreateCustomer(ctx, email, name, metadata)` - Create customer
- `VerifyWebhookSignature(payload, signature)` - Verify webhook

**Documentation:** https://stripe.com/docs/api

---

## Configuration Management

### Environment Variables

All API credentials are managed in `app.env` and loaded via `configs/config.go:161`.

### Configuration Structure

```go
type Config struct {
	// ... existing fields ...

	// Reloadly (configs/config.go:62-66)
	ReloadlyClientID     string
	ReloadlyClientSecret string
	ReloadlyBaseURL      string
	ReloadlyAuthURL      string
	ReloadlyEnabled      bool

	// Alpaca (configs/config.go:68-74)
	AlpacaAPIKey    string
	AlpacaAPISecret string
	AlpacaBaseURL   string
	AlpacaDataURL   string
	AlpacaEnabled   bool
	AlpacaIsPaper   bool

	// Stripe (configs/config.go:76-81)
	StripeSecretKey      string
	StripePublishableKey string
	StripeWebhookSecret  string
	StripeBaseURL        string
	StripeEnabled        bool
}
```

### Helper Methods

```go
// configs/config.go:113
config.GetReloadlyConfig()

// configs/config.go:129
config.GetAlpacaConfig()

// configs/config.go:150
config.GetStripeConfig()
```

---

## Testing

### Unit Tests

Create test files for each client:

```bash
# Test Reloadly client
go test -v ./services/reloadly_client_test.go

# Test Alpaca client
go test -v ./services/alpaca_client_test.go

# Test Stripe client
go test -v ./services/payment_client_test.go
```

### Integration Tests

```bash
# Test complete gift card purchase flow
go test -v ./services/gift_card_service_test.go

# Test stock order flow
go test -v ./services/stock_service_test.go
```

### Manual Testing Script

Use the test script at `/tmp/test_all_integrations.sh:58`:

```bash
chmod +x /tmp/test_all_integrations.sh
./tmp/test_all_integrations.sh
```

---

## Security Best Practices

### 1. API Key Management

**Never commit API keys to version control!**

```bash
# Add to .gitignore
app.env
*.env
*.key
*.pem
```

### 2. Use Environment-Specific Keys

- Development: Sandbox/test keys
- Staging: Sandbox/test keys
- Production: Live keys (separate from dev)

### 3. Rotate Keys Regularly

- Reloadly: Every 90 days
- Alpaca: Every 90 days
- Stripe: Every 90 days

### 4. Implement Rate Limiting

```go
// Add to grpcApi/middleware/rate_limit.go
func RateLimitMiddleware(limit int, window time.Duration) grpc.UnaryServerInterceptor {
	// Implement rate limiting logic
}
```

### 5. Monitor API Usage

- Log all external API calls
- Track response times
- Monitor error rates
- Set up alerts for:
  - High error rates (> 5%)
  - Slow responses (> 2s)
  - Unusual order volumes

### 6. Validate All Inputs

```go
// Example validation in stock orders
if req.Quantity <= 0 || req.Quantity > MAX_SHARES {
	return nil, status.Errorf(codes.InvalidArgument, "invalid quantity")
}

if req.Type != "market" && req.Type != "limit" {
	return nil, status.Errorf(codes.InvalidArgument, "invalid order type")
}
```

### 7. Use HTTPS in Production

- Never expose API keys over HTTP
- Use TLS certificates for all endpoints
- Implement HSTS headers

### 8. Database Encryption

- Encrypt sensitive data at rest
- Use prepared statements (prevent SQL injection)
- Hash and salt user passwords

---

## Troubleshooting

### Common Issues

#### 1. Reloadly Authentication Failed

**Error:** `"auth failed with status 401"`

**Solutions:**
- Verify Client ID and Secret are correct
- Check if using sandbox vs production credentials
- Ensure RELOADLY_ENABLED=true in app.env
- Check token hasn't expired (auto-refresh at `services/reloadly_client.go:41`)

#### 2. Alpaca API Key Invalid

**Error:** `"request failed with status 401"`

**Solutions:**
- Verify API Key ID and Secret Key are correct
- Check if using paper vs live credentials
- Ensure account is approved (for live trading)
- Verify market hours (stocks only trade 9:30am-4pm ET)

#### 3. Stripe Payment Failed

**Error:** `"Your card was declined"`

**Solutions:**
- Use test card numbers in test mode
- Verify webhook endpoint is accessible
- Check Stripe dashboard for detailed error
- Ensure 3D Secure is handled if required

#### 4. Service Not Enabled

**Error:** `"integration not enabled - configure API credentials"`

**Solutions:**
- Set `*_ENABLED=true` in app.env
- Restart server after config changes
- Check config loading: `configs/config.go:84`

#### 5. Rate Limit Exceeded

**Error:** `"rate limit exceeded"`

**Solutions:**
- Implement caching (Redis)
- Add request throttling
- Upgrade API plan if needed
- Use batch requests where possible

---

## Deployment Checklist

### Pre-Production

- [ ] All tests passing
- [ ] Sandbox/paper trading tested thoroughly
- [ ] Error handling implemented
- [ ] Rate limiting configured
- [ ] Logging and monitoring set up
- [ ] API keys secured (not in code)
- [ ] Webhooks tested (Stripe)
- [ ] Database backups configured

### Production Deployment

- [ ] Use production API credentials
- [ ] HTTPS enabled on all endpoints
- [ ] Environment variables set correctly
- [ ] Database migrations applied
- [ ] Redis cache configured
- [ ] Load balancer configured
- [ ] Health checks enabled
- [ ] Monitoring alerts configured
- [ ] Error tracking (Sentry/similar)
- [ ] API documentation updated

### Post-Deployment

- [ ] Monitor error rates
- [ ] Verify all endpoints working
- [ ] Check API usage/costs
- [ ] Test payment flows
- [ ] Verify webhooks receiving events
- [ ] Review logs for issues
- [ ] Performance testing
- [ ] User acceptance testing

---

## Support & Resources

### Documentation
- **Reloadly:** https://developers.reloadly.com/
- **Alpaca:** https://alpaca.markets/docs/
- **Stripe:** https://stripe.com/docs/api
- **CoinGecko:** https://www.coingecko.com/en/api/documentation

### Code References
- **Reloadly Client:** `services/reloadly_client.go:297`
- **Alpaca Client:** `services/alpaca_client.go:376`
- **Stripe Client:** `services/payment_client.go:323`
- **Config Management:** `configs/config.go:161`
- **Backend Integration Docs:** `BACKEND_INTEGRATIONS.md:246`

### Support Channels
- Reloadly: support@reloadly.com
- Alpaca: support@alpaca.markets
- Stripe: https://support.stripe.com/

---

**Last Updated:** December 8, 2025
**Version:** 1.0.0

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

// AlpacaConfig holds Alpaca API configuration
type AlpacaConfig struct {
	APIKey    string
	APISecret string
	BaseURL   string // https://api.alpaca.markets (live) or https://paper-api.alpaca.markets (paper)
	DataURL   string // https://data.alpaca.markets
	Enabled   bool   // Set to true when credentials are configured
	IsPaper   bool   // true for paper trading, false for live
}

// AlpacaClient handles Alpaca API interactions
type AlpacaClient struct {
	config     AlpacaConfig
	httpClient *http.Client
}

// NewAlpacaClient creates a new Alpaca API client
func NewAlpacaClient(config AlpacaConfig) *AlpacaClient {
	return &AlpacaClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// makeRequest makes an authenticated request to Alpaca API
func (c *AlpacaClient) makeRequest(ctx context.Context, baseURL, method, path string, body interface{}, result interface{}) error {
	if !c.config.Enabled {
		return fmt.Errorf("Alpaca integration not enabled - configure API credentials")
	}

	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonData)
	}

	url := baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("APCA-API-KEY-ID", c.config.APIKey)
	req.Header.Set("APCA-API-SECRET-KEY", c.config.APISecret)
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

// AlpacaAsset represents a stock asset
type AlpacaAsset struct {
	ID           string `json:"id"`
	Class        string `json:"class"`
	Exchange     string `json:"exchange"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Tradable     bool   `json:"tradable"`
	Marginable   bool   `json:"marginable"`
	Shortable    bool   `json:"shortable"`
	EasyToBorrow bool   `json:"easy_to_borrow"`
	Fractionable bool   `json:"fractionable"`
}

// AlpacaQuote represents a real-time quote
type AlpacaQuote struct {
	Symbol    string    `json:"symbol"`
	AskPrice  float64   `json:"ap"`
	AskSize   int       `json:"as"`
	BidPrice  float64   `json:"bp"`
	BidSize   int       `json:"bs"`
	Timestamp time.Time `json:"t"`
}

// AlpacaBar represents OHLCV bar data
type AlpacaBar struct {
	Timestamp  time.Time `json:"t"`
	Open       float64   `json:"o"`
	High       float64   `json:"h"`
	Low        float64   `json:"l"`
	Close      float64   `json:"c"`
	Volume     uint64    `json:"v"`
	VWAP       float64   `json:"vw"`
	TradeCount uint64    `json:"n"`
}

// AlpacaSnapshot represents a snapshot of current market data
type AlpacaSnapshot struct {
	Symbol       string       `json:"symbol"`
	LatestTrade  *AlpacaTrade `json:"latestTrade"`
	LatestQuote  *AlpacaQuote `json:"latestQuote"`
	MinuteBar    *AlpacaBar   `json:"minuteBar"`
	DailyBar     *AlpacaBar   `json:"dailyBar"`
	PrevDailyBar *AlpacaBar   `json:"prevDailyBar"`
}

// AlpacaTrade represents a trade
type AlpacaTrade struct {
	Timestamp  time.Time `json:"t"`
	Price      float64   `json:"p"`
	Size       int       `json:"s"`
	Exchange   string    `json:"x"`
	Conditions []string  `json:"c"`
}

// AlpacaAccount represents account information
type AlpacaAccount struct {
	ID                    string  `json:"id"`
	AccountNumber         string  `json:"account_number"`
	Status                string  `json:"status"`
	Currency              string  `json:"currency"`
	Cash                  float64 `json:"cash,string"`
	BuyingPower           float64 `json:"buying_power,string"`
	PortfolioValue        float64 `json:"portfolio_value,string"`
	PatternDayTrader      bool    `json:"pattern_day_trader"`
	TradingBlocked        bool    `json:"trading_blocked"`
	TransfersBlocked      bool    `json:"transfers_blocked"`
	AccountBlocked        bool    `json:"account_blocked"`
	Equity                float64 `json:"equity,string"`
	LastEquity            float64 `json:"last_equity,string"`
	LongMarketValue       float64 `json:"long_market_value,string"`
	ShortMarketValue      float64 `json:"short_market_value,string"`
	InitialMargin         float64 `json:"initial_margin,string"`
	MaintenanceMargin     float64 `json:"maintenance_margin,string"`
	DaytradingBuyingPower float64 `json:"daytrading_buying_power,string"`
}

// AlpacaPosition represents a position
type AlpacaPosition struct {
	AssetID        string  `json:"asset_id"`
	Symbol         string  `json:"symbol"`
	Exchange       string  `json:"exchange"`
	AssetClass     string  `json:"asset_class"`
	AvgEntryPrice  float64 `json:"avg_entry_price,string"`
	Qty            float64 `json:"qty,string"`
	Side           string  `json:"side"` // long or short
	MarketValue    float64 `json:"market_value,string"`
	CostBasis      float64 `json:"cost_basis,string"`
	UnrealizedPL   float64 `json:"unrealized_pl,string"`
	UnrealizedPLPC float64 `json:"unrealized_plpc,string"`
	CurrentPrice   float64 `json:"current_price,string"`
	LastdayPrice   float64 `json:"lastday_price,string"`
	ChangeToday    float64 `json:"change_today,string"`
}

// AlpacaOrder represents an order
type AlpacaOrder struct {
	ID             string        `json:"id"`
	ClientOrderID  string        `json:"client_order_id"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	SubmittedAt    time.Time     `json:"submitted_at"`
	FilledAt       *time.Time    `json:"filled_at"`
	ExpiredAt      *time.Time    `json:"expired_at"`
	CanceledAt     *time.Time    `json:"canceled_at"`
	FailedAt       *time.Time    `json:"failed_at"`
	AssetID        string        `json:"asset_id"`
	Symbol         string        `json:"symbol"`
	AssetClass     string        `json:"asset_class"`
	Qty            float64       `json:"qty,string"`
	FilledQty      float64       `json:"filled_qty,string"`
	OrderType      string        `json:"order_type"` // market, limit, stop, stop_limit
	Type           string        `json:"type"`       // market, limit, stop, stop_limit
	Side           string        `json:"side"`       // buy or sell
	TimeInForce    string        `json:"time_in_force"`
	LimitPrice     *float64      `json:"limit_price,string"`
	StopPrice      *float64      `json:"stop_price,string"`
	FilledAvgPrice *float64      `json:"filled_avg_price,string"`
	Status         string        `json:"status"`
	ExtendedHours  bool          `json:"extended_hours"`
	Legs           []AlpacaOrder `json:"legs"`
}

// CreateOrderRequest represents an order creation request
type CreateOrderRequest struct {
	Symbol        string   `json:"symbol"`
	Qty           float64  `json:"qty,omitempty"`
	Notional      float64  `json:"notional,omitempty"` // Dollar amount for fractional shares
	Side          string   `json:"side"`               // buy or sell
	Type          string   `json:"type"`               // market, limit, stop, stop_limit
	TimeInForce   string   `json:"time_in_force"`      // day, gtc, ioc, fok
	LimitPrice    *float64 `json:"limit_price,omitempty"`
	StopPrice     *float64 `json:"stop_price,omitempty"`
	ExtendedHours bool     `json:"extended_hours,omitempty"`
	ClientOrderID string   `json:"client_order_id,omitempty"`
}

// GetAccount retrieves account information
func (c *AlpacaClient) GetAccount(ctx context.Context) (*AlpacaAccount, error) {
	var account AlpacaAccount
	if err := c.makeRequest(ctx, c.config.BaseURL, "GET", "/v2/account", nil, &account); err != nil {
		return nil, err
	}
	return &account, nil
}

// GetAsset retrieves asset information
func (c *AlpacaClient) GetAsset(ctx context.Context, symbol string) (*AlpacaAsset, error) {
	var asset AlpacaAsset
	path := fmt.Sprintf("/v2/assets/%s", symbol)
	if err := c.makeRequest(ctx, c.config.BaseURL, "GET", path, nil, &asset); err != nil {
		return nil, err
	}
	return &asset, nil
}

// GetAssets retrieves all assets
func (c *AlpacaClient) GetAssets(ctx context.Context, status, assetClass string) ([]AlpacaAsset, error) {
	path := "/v2/assets"
	if status != "" || assetClass != "" {
		path += "?"
		if status != "" {
			path += fmt.Sprintf("status=%s&", status)
		}
		if assetClass != "" {
			path += fmt.Sprintf("asset_class=%s", assetClass)
		}
	}

	var assets []AlpacaAsset
	if err := c.makeRequest(ctx, c.config.BaseURL, "GET", path, nil, &assets); err != nil {
		return nil, err
	}
	return assets, nil
}

// GetLatestQuote retrieves the latest quote for a symbol
func (c *AlpacaClient) GetLatestQuote(ctx context.Context, symbol string) (*AlpacaQuote, error) {
	path := fmt.Sprintf("/v2/stocks/%s/quotes/latest", symbol)

	var result struct {
		Quote AlpacaQuote `json:"quote"`
	}

	if err := c.makeRequest(ctx, c.config.DataURL, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result.Quote, nil
}

// GetSnapshot retrieves current snapshot for a symbol
func (c *AlpacaClient) GetSnapshot(ctx context.Context, symbol string) (*AlpacaSnapshot, error) {
	path := fmt.Sprintf("/v2/stocks/%s/snapshot", symbol)

	var snapshot AlpacaSnapshot
	if err := c.makeRequest(ctx, c.config.DataURL, "GET", path, nil, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// GetBars retrieves historical bars for a symbol
func (c *AlpacaClient) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]AlpacaBar, error) {
	path := fmt.Sprintf("/v2/stocks/%s/bars?timeframe=%s", symbol, timeframe)

	if start != "" {
		path += fmt.Sprintf("&start=%s", start)
	}
	if end != "" {
		path += fmt.Sprintf("&end=%s", end)
	}
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}

	var result struct {
		Bars []AlpacaBar `json:"bars"`
	}

	if err := c.makeRequest(ctx, c.config.DataURL, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result.Bars, nil
}

// GetPositions retrieves all positions
func (c *AlpacaClient) GetPositions(ctx context.Context) ([]AlpacaPosition, error) {
	var positions []AlpacaPosition
	if err := c.makeRequest(ctx, c.config.BaseURL, "GET", "/v2/positions", nil, &positions); err != nil {
		return nil, err
	}
	return positions, nil
}

// GetPosition retrieves a specific position
func (c *AlpacaClient) GetPosition(ctx context.Context, symbol string) (*AlpacaPosition, error) {
	var position AlpacaPosition
	path := fmt.Sprintf("/v2/positions/%s", symbol)
	if err := c.makeRequest(ctx, c.config.BaseURL, "GET", path, nil, &position); err != nil {
		return nil, err
	}
	return &position, nil
}

// CreateOrder creates a new order
func (c *AlpacaClient) CreateOrder(ctx context.Context, req CreateOrderRequest) (*AlpacaOrder, error) {
	var order AlpacaOrder
	if err := c.makeRequest(ctx, c.config.BaseURL, "POST", "/v2/orders", req, &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// GetOrders retrieves orders
func (c *AlpacaClient) GetOrders(ctx context.Context, status string, limit int) ([]AlpacaOrder, error) {
	path := "/v2/orders?"
	if status != "" {
		path += fmt.Sprintf("status=%s&", status)
	}
	if limit > 0 {
		path += fmt.Sprintf("limit=%d", limit)
	}

	var orders []AlpacaOrder
	if err := c.makeRequest(ctx, c.config.BaseURL, "GET", path, nil, &orders); err != nil {
		return nil, err
	}
	return orders, nil
}

// GetOrder retrieves a specific order
func (c *AlpacaClient) GetOrder(ctx context.Context, orderID string) (*AlpacaOrder, error) {
	var order AlpacaOrder
	path := fmt.Sprintf("/v2/orders/%s", orderID)
	if err := c.makeRequest(ctx, c.config.BaseURL, "GET", path, nil, &order); err != nil {
		return nil, err
	}
	return &order, nil
}

// CancelOrder cancels an order
func (c *AlpacaClient) CancelOrder(ctx context.Context, orderID string) error {
	path := fmt.Sprintf("/v2/orders/%s", orderID)
	return c.makeRequest(ctx, c.config.BaseURL, "DELETE", path, nil, nil)
}

// CancelAllOrders cancels all open orders
func (c *AlpacaClient) CancelAllOrders(ctx context.Context) error {
	return c.makeRequest(ctx, c.config.BaseURL, "DELETE", "/v2/orders", nil, nil)
}

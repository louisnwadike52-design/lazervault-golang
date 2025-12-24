package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"lazervaultGo/pb"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// CoinGecko API base URL
const coinGeckoBaseURL = "https://api.coingecko.com/api/v3"

// ICryptoService defines the interface for crypto services
type ICryptoService interface {
	GetCryptos(ctx context.Context, req *pb.GetCryptosRequest) (*pb.GetCryptosResponse, error)
	GetCryptoById(ctx context.Context, req *pb.GetCryptoByIdRequest) (*pb.GetCryptoByIdResponse, error)
	SearchCryptos(ctx context.Context, req *pb.SearchCryptosRequest) (*pb.SearchCryptosResponse, error)
	GetCryptoPriceHistory(ctx context.Context, req *pb.GetCryptoPriceHistoryRequest) (*pb.GetCryptoPriceHistoryResponse, error)
	GetTrendingCryptos(ctx context.Context, req *pb.GetTrendingCryptosRequest) (*pb.GetTrendingCryptosResponse, error)
	GetTopCryptos(ctx context.Context, req *pb.GetTopCryptosRequest) (*pb.GetTopCryptosResponse, error)
	GetMarketChart(ctx context.Context, req *pb.GetMarketChartRequest) (*pb.GetMarketChartResponse, error)
	GetGlobalMarketData(ctx context.Context, req *pb.GetGlobalMarketDataRequest) (*pb.GetGlobalMarketDataResponse, error)
}

// CryptoService implements ICryptoService
type CryptoService struct {
	httpClient *http.Client
}

// NewCryptoService creates a new crypto service
func NewCryptoService() ICryptoService {
	return &CryptoService{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetCryptos retrieves a list of cryptocurrencies with market data
func (s *CryptoService) GetCryptos(ctx context.Context, req *pb.GetCryptosRequest) (*pb.GetCryptosResponse, error) {
	// Set defaults
	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 || perPage > 250 {
		perPage = 100
	}
	vsCurrency := req.VsCurrency
	if vsCurrency == "" {
		vsCurrency = "usd"
	}
	order := req.Order
	if order == "" {
		order = "market_cap_desc"
	}

	// Build URL
	params := url.Values{}
	params.Add("vs_currency", vsCurrency)
	params.Add("order", order)
	params.Add("per_page", strconv.Itoa(int(perPage)))
	params.Add("page", strconv.Itoa(int(page)))
	params.Add("sparkline", "false")
	params.Add("price_change_percentage", "1h,24h,7d,30d,1y")

	if len(req.Ids) > 0 {
		params.Add("ids", strings.Join(req.Ids, ","))
	}
	if req.Category != "" {
		params.Add("category", req.Category)
	}

	apiURL := fmt.Sprintf("%s/coins/markets?%s", coinGeckoBaseURL, params.Encode())

	// Make API request
	var coinsData []map[string]interface{}
	if err := s.makeRequest(ctx, apiURL, &coinsData); err != nil {
		return nil, err
	}

	// Convert to proto messages
	cryptos := make([]*pb.CryptoMessage, 0, len(coinsData))
	for _, coinData := range coinsData {
		crypto := s.coinDataToProto(coinData, vsCurrency)
		cryptos = append(cryptos, crypto)
	}

	// Calculate pagination info
	totalItems := int32(len(cryptos))
	if len(req.Ids) > 0 {
		totalItems = int32(len(req.Ids))
	} else {
		// For general listing, estimate total
		totalItems = 10000 // CoinGecko has ~10k+ coins
	}
	totalPages := (totalItems + perPage - 1) / perPage

	pagination := &pb.CryptoPaginationInfo{
		CurrentPage:  page,
		TotalPages:   totalPages,
		TotalItems:   totalItems,
		ItemsPerPage: perPage,
		HasNext:      page < totalPages,
		HasPrev:      page > 1,
	}

	return &pb.GetCryptosResponse{
		Cryptos:    cryptos,
		Pagination: pagination,
	}, nil
}

// GetCryptoById retrieves detailed information about a specific cryptocurrency
func (s *CryptoService) GetCryptoById(ctx context.Context, req *pb.GetCryptoByIdRequest) (*pb.GetCryptoByIdResponse, error) {
	vsCurrency := req.VsCurrency
	if vsCurrency == "" {
		vsCurrency = "usd"
	}

	// Build URL
	params := url.Values{}
	params.Add("localization", "false")
	params.Add("tickers", "false")
	params.Add("market_data", strconv.FormatBool(req.IncludeMarketData))
	params.Add("community_data", strconv.FormatBool(req.IncludeCommunityData))
	params.Add("developer_data", strconv.FormatBool(req.IncludeDeveloperData))
	params.Add("sparkline", "false")

	apiURL := fmt.Sprintf("%s/coins/%s?%s", coinGeckoBaseURL, req.Id, params.Encode())

	// Make API request
	var coinData map[string]interface{}
	if err := s.makeRequest(ctx, apiURL, &coinData); err != nil {
		return nil, err
	}

	// Convert to proto message
	crypto := s.coinDetailToProto(coinData, vsCurrency)

	return &pb.GetCryptoByIdResponse{
		Crypto: crypto,
	}, nil
}

// SearchCryptos searches for cryptocurrencies by name or symbol
func (s *CryptoService) SearchCryptos(ctx context.Context, req *pb.SearchCryptosRequest) (*pb.SearchCryptosResponse, error) {
	apiURL := fmt.Sprintf("%s/search?query=%s", coinGeckoBaseURL, url.QueryEscape(req.Query))

	var searchData map[string]interface{}
	if err := s.makeRequest(ctx, apiURL, &searchData); err != nil {
		return nil, err
	}

	// Extract coins from search results
	coinsInterface, ok := searchData["coins"].([]interface{})
	if !ok {
		return &pb.SearchCryptosResponse{Cryptos: []*pb.CryptoMessage{}}, nil
	}

	// Convert to proto messages
	cryptos := make([]*pb.CryptoMessage, 0, len(coinsInterface))
	for _, coinInterface := range coinsInterface {
		coinData, ok := coinInterface.(map[string]interface{})
		if !ok {
			continue
		}

		crypto := &pb.CryptoMessage{
			Id:     getString(coinData, "id"),
			Symbol: getString(coinData, "symbol"),
			Name:   getString(coinData, "name"),
			Image:  getString(coinData, "large"),
		}
		if marketCapRank := getFloat64(coinData, "market_cap_rank"); marketCapRank > 0 {
			crypto.MarketCapRank = int32(marketCapRank)
		}
		cryptos = append(cryptos, crypto)
	}

	return &pb.SearchCryptosResponse{
		Cryptos: cryptos,
	}, nil
}

// GetCryptoPriceHistory retrieves historical price data for a cryptocurrency
func (s *CryptoService) GetCryptoPriceHistory(ctx context.Context, req *pb.GetCryptoPriceHistoryRequest) (*pb.GetCryptoPriceHistoryResponse, error) {
	vsCurrency := req.VsCurrency
	if vsCurrency == "" {
		vsCurrency = "usd"
	}

	// Convert range to days
	days := s.rangeToDays(req.Range)

	// Build URL
	params := url.Values{}
	params.Add("vs_currency", vsCurrency)
	params.Add("days", days)
	if req.Interval > 0 {
		params.Add("interval", strconv.Itoa(int(req.Interval)))
	}

	apiURL := fmt.Sprintf("%s/coins/%s/market_chart?%s", coinGeckoBaseURL, req.Id, params.Encode())

	var chartData map[string]interface{}
	if err := s.makeRequest(ctx, apiURL, &chartData); err != nil {
		return nil, err
	}

	// Extract price history
	priceHistory := s.extractPriceHistory(chartData)

	return &pb.GetCryptoPriceHistoryResponse{
		PriceHistory: priceHistory,
		CryptoId:     req.Id,
		Range:        req.Range,
	}, nil
}

// GetTrendingCryptos retrieves currently trending cryptocurrencies
func (s *CryptoService) GetTrendingCryptos(ctx context.Context, req *pb.GetTrendingCryptosRequest) (*pb.GetTrendingCryptosResponse, error) {
	apiURL := fmt.Sprintf("%s/search/trending", coinGeckoBaseURL)

	var trendingData map[string]interface{}
	if err := s.makeRequest(ctx, apiURL, &trendingData); err != nil {
		return nil, err
	}

	// Extract coins from trending results
	coinsInterface, ok := trendingData["coins"].([]interface{})
	if !ok {
		return &pb.GetTrendingCryptosResponse{Cryptos: []*pb.CryptoMessage{}}, nil
	}

	limit := int(req.Limit)
	if limit < 1 {
		limit = 10
	}

	// Convert to proto messages
	cryptos := make([]*pb.CryptoMessage, 0, len(coinsInterface))
	for i, coinInterface := range coinsInterface {
		if i >= limit {
			break
		}

		coinWrapper, ok := coinInterface.(map[string]interface{})
		if !ok {
			continue
		}
		coinData, ok := coinWrapper["item"].(map[string]interface{})
		if !ok {
			continue
		}

		crypto := &pb.CryptoMessage{
			Id:     getString(coinData, "id"),
			Symbol: getString(coinData, "symbol"),
			Name:   getString(coinData, "name"),
			Image:  getString(coinData, "large"),
		}
		if marketCapRank := getFloat64(coinData, "market_cap_rank"); marketCapRank > 0 {
			crypto.MarketCapRank = int32(marketCapRank)
		}
		cryptos = append(cryptos, crypto)
	}

	return &pb.GetTrendingCryptosResponse{
		Cryptos: cryptos,
	}, nil
}

// GetTopCryptos retrieves top cryptocurrencies by market cap
func (s *CryptoService) GetTopCryptos(ctx context.Context, req *pb.GetTopCryptosRequest) (*pb.GetTopCryptosResponse, error) {
	limit := req.Limit
	if limit < 1 || limit > 250 {
		limit = 100
	}
	vsCurrency := req.VsCurrency
	if vsCurrency == "" {
		vsCurrency = "usd"
	}

	// Use GetCryptos with specific parameters
	cryptosReq := &pb.GetCryptosRequest{
		Page:       1,
		PerPage:    limit,
		VsCurrency: vsCurrency,
		Order:      "market_cap_desc",
	}

	cryptosResp, err := s.GetCryptos(ctx, cryptosReq)
	if err != nil {
		return nil, err
	}

	return &pb.GetTopCryptosResponse{
		Cryptos: cryptosResp.Cryptos,
	}, nil
}

// GetMarketChart retrieves market chart data (prices, market caps, volumes)
func (s *CryptoService) GetMarketChart(ctx context.Context, req *pb.GetMarketChartRequest) (*pb.GetMarketChartResponse, error) {
	vsCurrency := req.VsCurrency
	if vsCurrency == "" {
		vsCurrency = "usd"
	}

	// Build URL
	params := url.Values{}
	params.Add("vs_currency", vsCurrency)
	params.Add("days", strconv.Itoa(int(req.Days)))
	if req.Interval != "" {
		params.Add("interval", req.Interval)
	}

	apiURL := fmt.Sprintf("%s/coins/%s/market_chart?%s", coinGeckoBaseURL, req.Id, params.Encode())

	var chartData map[string]interface{}
	if err := s.makeRequest(ctx, apiURL, &chartData); err != nil {
		return nil, err
	}

	// Extract different data types
	prices := s.extractPricePoints(chartData, "prices")
	marketCaps := s.extractPricePoints(chartData, "market_caps")
	totalVolumes := s.extractPricePoints(chartData, "total_volumes")

	return &pb.GetMarketChartResponse{
		Prices:       prices,
		MarketCaps:   marketCaps,
		TotalVolumes: totalVolumes,
	}, nil
}

// GetGlobalMarketData retrieves global cryptocurrency market data
func (s *CryptoService) GetGlobalMarketData(ctx context.Context, req *pb.GetGlobalMarketDataRequest) (*pb.GetGlobalMarketDataResponse, error) {
	apiURL := fmt.Sprintf("%s/global", coinGeckoBaseURL)

	var globalData map[string]interface{}
	if err := s.makeRequest(ctx, apiURL, &globalData); err != nil {
		return nil, err
	}

	data, ok := globalData["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid global market data response")
	}

	// Extract market cap data
	marketCapData, _ := data["total_market_cap"].(map[string]interface{})
	totalMarketCap := getFloat64(marketCapData, "usd")

	volumeData, _ := data["total_volume"].(map[string]interface{})
	totalVolume24h := getFloat64(volumeData, "usd")

	marketCapPercentage, _ := data["market_cap_percentage"].(map[string]interface{})
	btcPercentage := getFloat64(marketCapPercentage, "btc")
	ethPercentage := getFloat64(marketCapPercentage, "eth")

	activeCryptos := int32(getFloat64(data, "active_cryptocurrencies"))
	markets := int32(getFloat64(data, "markets"))

	// Parse updated_at timestamp
	updatedAt := time.Now()
	if updatedAtFloat := getFloat64(data, "updated_at"); updatedAtFloat > 0 {
		updatedAt = time.Unix(int64(updatedAtFloat), 0)
	}

	return &pb.GetGlobalMarketDataResponse{
		TotalMarketCap:          totalMarketCap,
		TotalVolume_24H:         totalVolume24h,
		MarketCapPercentageBtc:  btcPercentage,
		MarketCapPercentageEth:  ethPercentage,
		ActiveCryptocurrencies:  activeCryptos,
		Markets:                 markets,
		UpdatedAt:               timestamppb.New(updatedAt),
	}, nil
}

// Helper functions

func (s *CryptoService) makeRequest(ctx context.Context, url string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}

func (s *CryptoService) coinDataToProto(data map[string]interface{}, vsCurrency string) *pb.CryptoMessage {
	crypto := &pb.CryptoMessage{
		Id:                         getString(data, "id"),
		Symbol:                     getString(data, "symbol"),
		Name:                       getString(data, "name"),
		Image:                      getString(data, "image"),
		CurrentPrice:               getFloat64(data, "current_price"),
		MarketCap:                  uint64(getFloat64(data, "market_cap")),
		MarketCapRank:              int32(getFloat64(data, "market_cap_rank")),
		TotalVolume:                uint64(getFloat64(data, "total_volume")),
		High_24H:                   getFloat64(data, "high_24h"),
		Low_24H:                    getFloat64(data, "low_24h"),
		PriceChange_24H:            getFloat64(data, "price_change_24h"),
		PriceChangePercentage_24H:  getFloat64(data, "price_change_percentage_24h"),
		PriceChangePercentage_7D:   getFloat64(data, "price_change_percentage_7d_in_currency"),
		PriceChangePercentage_30D:  getFloat64(data, "price_change_percentage_30d_in_currency"),
		PriceChangePercentage_1Y:   getFloat64(data, "price_change_percentage_1y_in_currency"),
		CirculatingSupply:          getFloat64(data, "circulating_supply"),
		TotalSupply:                getFloat64(data, "total_supply"),
		MaxSupply:                  getFloat64(data, "max_supply"),
		Ath:                        getFloat64(data, "ath"),
		AthChangePercentage:        getFloat64(data, "ath_change_percentage"),
		Atl:                        getFloat64(data, "atl"),
		AtlChangePercentage:        getFloat64(data, "atl_change_percentage"),
		FullyDilutedValuation:      getFloat64(data, "fully_diluted_valuation"),
		MarketCapChange_24H:        getFloat64(data, "market_cap_change_24h"),
		MarketCapChangePercentage_24H: getFloat64(data, "market_cap_change_percentage_24h"),
	}

	// Parse timestamps
	if athDate := getString(data, "ath_date"); athDate != "" {
		if t, err := time.Parse(time.RFC3339, athDate); err == nil {
			crypto.AthDate = timestamppb.New(t)
		}
	}
	if atlDate := getString(data, "atl_date"); atlDate != "" {
		if t, err := time.Parse(time.RFC3339, atlDate); err == nil {
			crypto.AtlDate = timestamppb.New(t)
		}
	}
	if lastUpdated := getString(data, "last_updated"); lastUpdated != "" {
		if t, err := time.Parse(time.RFC3339, lastUpdated); err == nil {
			crypto.LastUpdated = timestamppb.New(t)
		}
	}

	return crypto
}

func (s *CryptoService) coinDetailToProto(data map[string]interface{}, vsCurrency string) *pb.CryptoMessage {
	crypto := &pb.CryptoMessage{
		Id:     getString(data, "id"),
		Symbol: getString(data, "symbol"),
		Name:   getString(data, "name"),
	}

	// Extract image
	if imageData, ok := data["image"].(map[string]interface{}); ok {
		crypto.Image = getString(imageData, "large")
	}

	// Extract description
	if descData, ok := data["description"].(map[string]interface{}); ok {
		crypto.Description = getString(descData, "en")
	}

	// Extract categories
	if categoriesData, ok := data["categories"].([]interface{}); ok {
		categories := make([]string, 0, len(categoriesData))
		for _, cat := range categoriesData {
			if catStr, ok := cat.(string); ok {
				categories = append(categories, catStr)
			}
		}
		crypto.Categories = categories
	}

	// Extract links
	if linksData, ok := data["links"].(map[string]interface{}); ok {
		links := make(map[string]string)
		if homepage, ok := linksData["homepage"].([]interface{}); ok && len(homepage) > 0 {
			if homepageStr, ok := homepage[0].(string); ok {
				links["homepage"] = homepageStr
			}
		}
		if blockchain, ok := linksData["blockchain_site"].([]interface{}); ok && len(blockchain) > 0 {
			if blockchainStr, ok := blockchain[0].(string); ok {
				links["blockchain_site"] = blockchainStr
			}
		}
		crypto.Links = links
	}

	// Extract market data if available
	if marketData, ok := data["market_data"].(map[string]interface{}); ok {
		if currentPrice, ok := marketData["current_price"].(map[string]interface{}); ok {
			crypto.CurrentPrice = getFloat64(currentPrice, vsCurrency)
		}
		if marketCap, ok := marketData["market_cap"].(map[string]interface{}); ok {
			crypto.MarketCap = uint64(getFloat64(marketCap, vsCurrency))
		}
		crypto.MarketCapRank = int32(getFloat64(marketData, "market_cap_rank"))

		if totalVolume, ok := marketData["total_volume"].(map[string]interface{}); ok {
			crypto.TotalVolume = uint64(getFloat64(totalVolume, vsCurrency))
		}
		if high24h, ok := marketData["high_24h"].(map[string]interface{}); ok {
			crypto.High_24H = getFloat64(high24h, vsCurrency)
		}
		if low24h, ok := marketData["low_24h"].(map[string]interface{}); ok {
			crypto.Low_24H = getFloat64(low24h, vsCurrency)
		}
		crypto.CirculatingSupply = getFloat64(marketData, "circulating_supply")
		crypto.TotalSupply = getFloat64(marketData, "total_supply")
		crypto.MaxSupply = getFloat64(marketData, "max_supply")
	}

	return crypto
}

func (s *CryptoService) extractPriceHistory(data map[string]interface{}) []*pb.PricePoint {
	pricePoints := make([]*pb.PricePoint, 0)

	pricesData, _ := data["prices"].([]interface{})
	volumesData, _ := data["total_volumes"].([]interface{})
	marketCapsData, _ := data["market_caps"].([]interface{})

	maxLen := len(pricesData)
	if len(volumesData) > maxLen {
		maxLen = len(volumesData)
	}
	if len(marketCapsData) > maxLen {
		maxLen = len(marketCapsData)
	}

	for i := 0; i < maxLen; i++ {
		point := &pb.PricePoint{}

		if i < len(pricesData) {
			if priceArr, ok := pricesData[i].([]interface{}); ok && len(priceArr) == 2 {
				if timestamp, ok := priceArr[0].(float64); ok {
					point.Timestamp = timestamppb.New(time.Unix(int64(timestamp)/1000, 0))
				}
				if price, ok := priceArr[1].(float64); ok {
					point.Price = price
				}
			}
		}

		if i < len(volumesData) {
			if volumeArr, ok := volumesData[i].([]interface{}); ok && len(volumeArr) == 2 {
				if volume, ok := volumeArr[1].(float64); ok {
					point.Volume = volume
				}
			}
		}

		if i < len(marketCapsData) {
			if mcArr, ok := marketCapsData[i].([]interface{}); ok && len(mcArr) == 2 {
				if marketCap, ok := mcArr[1].(float64); ok {
					point.MarketCap = marketCap
				}
			}
		}

		pricePoints = append(pricePoints, point)
	}

	return pricePoints
}

func (s *CryptoService) extractPricePoints(data map[string]interface{}, key string) []*pb.PricePoint {
	points := make([]*pb.PricePoint, 0)

	pointsData, ok := data[key].([]interface{})
	if !ok {
		return points
	}

	for _, pointInterface := range pointsData {
		pointArr, ok := pointInterface.([]interface{})
		if !ok || len(pointArr) != 2 {
			continue
		}

		timestamp, ok1 := pointArr[0].(float64)
		value, ok2 := pointArr[1].(float64)
		if !ok1 || !ok2 {
			continue
		}

		point := &pb.PricePoint{
			Timestamp: timestamppb.New(time.Unix(int64(timestamp)/1000, 0)),
		}

		switch key {
		case "prices":
			point.Price = value
		case "market_caps":
			point.MarketCap = value
		case "total_volumes":
			point.Volume = value
		}

		points = append(points, point)
	}

	return points
}

func (s *CryptoService) rangeToDays(rangeStr string) string {
	switch strings.ToLower(rangeStr) {
	case "1d":
		return "1"
	case "7d":
		return "7"
	case "30d":
		return "30"
	case "90d":
		return "90"
	case "1y":
		return "365"
	case "all":
		return "max"
	default:
		return "30"
	}
}

// Utility functions for safe type extraction

func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

func getFloat64(data map[string]interface{}, key string) float64 {
	if val, ok := data[key].(float64); ok {
		return val
	}
	return 0
}

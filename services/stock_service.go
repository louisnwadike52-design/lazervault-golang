package services

import (
	"context"
	"fmt"
	"lazervaultGo/pb"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// IStockService defines the stock service interface
type IStockService interface {
	GetStocks(ctx context.Context, req *pb.GetStocksRequest) (*pb.GetStocksResponse, error)
	GetStockBySymbol(ctx context.Context, req *pb.GetStockBySymbolRequest) (*pb.GetStockBySymbolResponse, error)
	SearchStocks(ctx context.Context, req *pb.SearchStocksRequest) (*pb.SearchStocksResponse, error)
	GetStockPriceHistory(ctx context.Context, req *pb.GetStockPriceHistoryRequest) (*pb.GetStockPriceHistoryResponse, error)
	GetUserPortfolio(ctx context.Context, userID uint, req *pb.GetUserPortfolioRequest) (*pb.GetUserPortfolioResponse, error)
	PlaceOrder(ctx context.Context, userID uint, req *pb.PlaceOrderRequest) (*pb.PlaceOrderResponse, error)
	GetUserOrders(ctx context.Context, userID uint, req *pb.GetUserOrdersRequest) (*pb.GetUserOrdersResponse, error)
	CancelOrder(ctx context.Context, userID uint, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error)
	GetMarketIndices(ctx context.Context, req *pb.GetMarketIndicesRequest) (*pb.GetMarketIndicesResponse, error)
	GetTrendingStocks(ctx context.Context, req *pb.GetTrendingStocksRequest) (*pb.GetTrendingStocksResponse, error)
	GetTopGainers(ctx context.Context, req *pb.GetTopGainersRequest) (*pb.GetTopGainersResponse, error)
	GetTopLosers(ctx context.Context, req *pb.GetTopLosersRequest) (*pb.GetTopLosersResponse, error)
	CreateWatchlist(ctx context.Context, userID uint, req *pb.CreateWatchlistRequest) (*pb.CreateWatchlistResponse, error)
	GetUserWatchlists(ctx context.Context, userID uint, req *pb.GetUserWatchlistsRequest) (*pb.GetUserWatchlistsResponse, error)
	AddToWatchlist(ctx context.Context, userID uint, req *pb.AddToWatchlistRequest) (*pb.AddToWatchlistResponse, error)
	RemoveFromWatchlist(ctx context.Context, userID uint, req *pb.RemoveFromWatchlistRequest) (*pb.RemoveFromWatchlistResponse, error)
}

// StockService implements the stock service with mock data
// Ready for Alpaca API integration
type StockService struct {
	mu               sync.RWMutex
	mockStocks       map[string]*pb.StockMessage
	mockPortfolios   map[uint]*pb.PortfolioMessage
	mockOrders       map[uint][]*pb.StockOrderMessage
	mockWatchlists   map[uint][]*pb.WatchlistMessage
	mockIndices      []*pb.MarketIndexMessage
	orderCounter     int
	watchlistCounter int
}

// NewStockService creates a new stock service with mock data
func NewStockService() IStockService {
	service := &StockService{
		mockStocks:     make(map[string]*pb.StockMessage),
		mockPortfolios: make(map[uint]*pb.PortfolioMessage),
		mockOrders:     make(map[uint][]*pb.StockOrderMessage),
		mockWatchlists: make(map[uint][]*pb.WatchlistMessage),
	}
	service.initMockData()
	return service
}

// initMockData initializes mock stock data
func (s *StockService) initMockData() {
	now := timestamppb.Now()

	// Initialize mock stocks
	stocks := []struct {
		symbol   string
		name     string
		price    float64
		change   float64
		marketCap float64
		sector   string
		industry string
	}{
		{"AAPL", "Apple Inc.", 178.50, 2.35, 2800000000000, "Technology", "Consumer Electronics"},
		{"GOOGL", "Alphabet Inc.", 142.30, -1.20, 1750000000000, "Technology", "Internet Content & Information"},
		{"MSFT", "Microsoft Corporation", 378.90, 3.45, 2820000000000, "Technology", "Software"},
		{"AMZN", "Amazon.com Inc.", 152.80, 1.90, 1580000000000, "Consumer Cyclical", "Internet Retail"},
		{"TSLA", "Tesla Inc.", 242.50, -5.20, 770000000000, "Consumer Cyclical", "Auto Manufacturers"},
		{"META", "Meta Platforms Inc.", 355.20, 4.10, 906000000000, "Technology", "Internet Content & Information"},
		{"NVDA", "NVIDIA Corporation", 495.20, 8.75, 1220000000000, "Technology", "Semiconductors"},
		{"JPM", "JPMorgan Chase & Co.", 152.40, 0.85, 445000000000, "Financial", "Banks"},
		{"V", "Visa Inc.", 245.60, 1.45, 510000000000, "Financial", "Credit Services"},
		{"WMT", "Walmart Inc.", 168.30, 0.65, 445000000000, "Consumer Defensive", "Discount Stores"},
	}

	for _, stock := range stocks {
		changePercent := (stock.change / (stock.price - stock.change)) * 100

		s.mockStocks[stock.symbol] = &pb.StockMessage{
			Symbol:         stock.symbol,
			Name:           stock.name,
			CurrentPrice:   stock.price,
			PreviousClose:  stock.price - stock.change,
			Change:         stock.change,
			ChangePercent:  changePercent,
			DayHigh:        stock.price + 2.5,
			DayLow:         stock.price - 3.2,
			Volume:         45000000 + float64(len(stock.symbol)*1000000),
			MarketCap:      stock.marketCap,
			PeRatio:        25.5 + float64(len(stock.name)),
			DividendYield:  0.5 + (float64(len(stock.symbol)) * 0.1),
			Sector:         stock.sector,
			Industry:       stock.industry,
			LogoUrl:        fmt.Sprintf("https://logo.clearbit.com/%s.com", strings.ToLower(stock.symbol)),
			LastUpdated:    now,
			WeekHigh_52:    stock.price * 1.25,
			WeekLow_52:     stock.price * 0.75,
			AvgVolume:      42000000,
			Beta:           1.15,
			Eps:            stock.price / 28.5,
			Description:    fmt.Sprintf("%s is a leading company in the %s industry.", stock.name, stock.industry),
			Exchange:       "NASDAQ",
			Currency:       "USD",
			PriceHistory:   s.generateMockPriceHistory(stock.price),
		}
	}

	// Initialize market indices
	s.mockIndices = []*pb.MarketIndexMessage{
		{
			Symbol:        "^GSPC",
			Name:          "S&P 500",
			Value:         4567.80,
			Change:        23.45,
			ChangePercent: 0.52,
			LastUpdated:   now,
		},
		{
			Symbol:        "^DJI",
			Name:          "Dow Jones Industrial Average",
			Value:         35482.30,
			Change:        156.78,
			ChangePercent: 0.44,
			LastUpdated:   now,
		},
		{
			Symbol:        "^IXIC",
			Name:          "NASDAQ Composite",
			Value:         14239.88,
			Change:        -45.23,
			ChangePercent: -0.32,
			LastUpdated:   now,
		},
	}
}

// generateMockPriceHistory generates mock price history data
func (s *StockService) generateMockPriceHistory(currentPrice float64) []*pb.PricePoint {
	history := make([]*pb.PricePoint, 30)
	now := time.Now()

	for i := 0; i < 30; i++ {
		// Generate price variation
		dayOffset := float64(30-i) * 0.5
		price := currentPrice - dayOffset + (float64(i%5) * 0.3)

		history[i] = &pb.PricePoint{
			Timestamp: timestamppb.New(now.AddDate(0, 0, -30+i)),
			Price:     price,
			Volume:    40000000 + float64(i*100000),
			Open:      price - 0.5,
			High:      price + 1.2,
			Low:       price - 1.5,
			Close:     price,
		}
	}

	return history
}

// GetStocks retrieves multiple stocks with pagination
func (s *StockService) GetStocks(ctx context.Context, req *pb.GetStocksRequest) (*pb.GetStocksResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// If specific symbols requested
	if len(req.Symbols) > 0 {
		stocks := make([]*pb.StockMessage, 0, len(req.Symbols))
		for _, symbol := range req.Symbols {
			if stock, exists := s.mockStocks[symbol]; exists {
				stocks = append(stocks, stock)
			}
		}
		return &pb.GetStocksResponse{
			Stocks: stocks,
			Pagination: &pb.StockPaginationInfo{
				CurrentPage:  1,
				TotalPages:   1,
				TotalItems:   int32(len(stocks)),
				ItemsPerPage: int32(len(stocks)),
				HasNext:      false,
				HasPrev:      false,
			},
		}, nil
	}

	// Return all stocks with pagination
	allStocks := make([]*pb.StockMessage, 0, len(s.mockStocks))
	for _, stock := range s.mockStocks {
		allStocks = append(allStocks, stock)
	}

	// Apply pagination
	page := req.Page
	if page == 0 {
		page = 1
	}
	perPage := req.PerPage
	if perPage == 0 {
		perPage = 10
	}

	start := int((page - 1) * perPage)
	end := int(page * perPage)
	if start >= len(allStocks) {
		start = len(allStocks)
	}
	if end > len(allStocks) {
		end = len(allStocks)
	}

	paginatedStocks := allStocks[start:end]
	totalPages := (int32(len(allStocks)) + perPage - 1) / perPage

	return &pb.GetStocksResponse{
		Stocks: paginatedStocks,
		Pagination: &pb.StockPaginationInfo{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   int32(len(allStocks)),
			ItemsPerPage: perPage,
			HasNext:      page < totalPages,
			HasPrev:      page > 1,
		},
	}, nil
}

// GetStockBySymbol retrieves a single stock by symbol
func (s *StockService) GetStockBySymbol(ctx context.Context, req *pb.GetStockBySymbolRequest) (*pb.GetStockBySymbolResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stock, exists := s.mockStocks[req.Symbol]
	if !exists {
		return nil, fmt.Errorf("stock with symbol %s not found", req.Symbol)
	}

	return &pb.GetStockBySymbolResponse{
		Stock: stock,
	}, nil
}

// SearchStocks searches for stocks by query
func (s *StockService) SearchStocks(ctx context.Context, req *pb.SearchStocksRequest) (*pb.SearchStocksResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := strings.ToLower(req.Query)
	results := make([]*pb.StockMessage, 0)

	for _, stock := range s.mockStocks {
		if strings.Contains(strings.ToLower(stock.Symbol), query) ||
			strings.Contains(strings.ToLower(stock.Name), query) {
			results = append(results, stock)

			// Apply limit
			if req.Limit > 0 && int32(len(results)) >= req.Limit {
				break
			}
		}
	}

	return &pb.SearchStocksResponse{
		Stocks: results,
	}, nil
}

// GetStockPriceHistory retrieves price history for a stock
func (s *StockService) GetStockPriceHistory(ctx context.Context, req *pb.GetStockPriceHistoryRequest) (*pb.GetStockPriceHistoryResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stock, exists := s.mockStocks[req.Symbol]
	if !exists {
		return nil, fmt.Errorf("stock with symbol %s not found", req.Symbol)
	}

	// For now, return the same mock price history
	// In production, this would filter by timeframe and interval
	return &pb.GetStockPriceHistoryResponse{
		Symbol:       req.Symbol,
		PriceHistory: stock.PriceHistory,
		Timeframe:    req.Timeframe,
	}, nil
}

// GetUserPortfolio retrieves user's portfolio
func (s *StockService) GetUserPortfolio(ctx context.Context, userID uint, req *pb.GetUserPortfolioRequest) (*pb.GetUserPortfolioResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	portfolio, exists := s.mockPortfolios[userID]
	if !exists {
		// Create empty portfolio
		portfolio = &pb.PortfolioMessage{
			Id:               fmt.Sprintf("portfolio-%d", userID),
			TotalValue:       10000.0,
			TotalCost:        10000.0,
			TotalReturn:      0.0,
			TotalReturnPercent: 0.0,
			DayChange:        0.0,
			DayChangePercent: 0.0,
			Holdings:         []*pb.StockHoldingMessage{},
			LastUpdated:      timestamppb.Now(),
			AvailableCash:    10000.0,
			TotalInvested:    0.0,
		}
		s.mockPortfolios[userID] = portfolio
	}

	return &pb.GetUserPortfolioResponse{
		Portfolio: portfolio,
	}, nil
}

// PlaceOrder places a stock order
func (s *StockService) PlaceOrder(ctx context.Context, userID uint, req *pb.PlaceOrderRequest) (*pb.PlaceOrderResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify stock exists
	stock, exists := s.mockStocks[req.Symbol]
	if !exists {
		return &pb.PlaceOrderResponse{
			Success: false,
			Message: fmt.Sprintf("stock with symbol %s not found", req.Symbol),
		}, nil
	}

	// Get or create portfolio
	portfolio, exists := s.mockPortfolios[userID]
	if !exists {
		portfolio = &pb.PortfolioMessage{
			Id:            fmt.Sprintf("portfolio-%d", userID),
			AvailableCash: 10000.0,
			Holdings:      []*pb.StockHoldingMessage{},
		}
		s.mockPortfolios[userID] = portfolio
	}

	// Create order
	s.orderCounter++
	order := &pb.StockOrderMessage{
		Id:              fmt.Sprintf("order-%d", s.orderCounter),
		Symbol:          req.Symbol,
		Type:            req.Type,
		Side:            req.Side,
		Quantity:        req.Quantity,
		Price:           req.Price,
		Status:          pb.OrderStatus_ORDER_STATUS_EXECUTED,
		CreatedAt:       timestamppb.Now(),
		ExecutedAt:      timestamppb.Now(),
		ExecutedPrice:   stock.CurrentPrice,
		ExecutedQuantity: req.Quantity,
		Fees:            0.0,
		Notes:           req.Notes,
	}

	// Store order
	if s.mockOrders[userID] == nil {
		s.mockOrders[userID] = []*pb.StockOrderMessage{}
	}
	s.mockOrders[userID] = append(s.mockOrders[userID], order)

	// Update portfolio
	if req.Side == pb.OrderSide_ORDER_SIDE_BUY {
		cost := stock.CurrentPrice * float64(req.Quantity)
		if cost > portfolio.AvailableCash {
			order.Status = pb.OrderStatus_ORDER_STATUS_REJECTED
			return &pb.PlaceOrderResponse{
				Order:   order,
				Success: false,
				Message: "insufficient funds",
			}, nil
		}

		portfolio.AvailableCash -= cost

		// Add or update holding
		found := false
		for _, holding := range portfolio.Holdings {
			if holding.Symbol == req.Symbol {
				totalShares := holding.Shares + req.Quantity
				totalCost := (holding.AverageCost * float64(holding.Shares)) + cost
				holding.Shares = totalShares
				holding.AverageCost = totalCost / float64(totalShares)
				holding.CurrentPrice = stock.CurrentPrice
				holding.TotalValue = stock.CurrentPrice * float64(totalShares)
				holding.TotalReturn = holding.TotalValue - totalCost
				holding.TotalReturnPercent = (holding.TotalReturn / totalCost) * 100
				found = true
				break
			}
		}

		if !found {
			portfolio.Holdings = append(portfolio.Holdings, &pb.StockHoldingMessage{
				Symbol:             req.Symbol,
				Name:               stock.Name,
				Shares:             req.Quantity,
				AverageCost:        stock.CurrentPrice,
				CurrentPrice:       stock.CurrentPrice,
				TotalValue:         stock.CurrentPrice * float64(req.Quantity),
				TotalReturn:        0.0,
				TotalReturnPercent: 0.0,
				DayChange:          0.0,
				DayChangePercent:   0.0,
				PurchaseDate:       timestamppb.Now(),
				LogoUrl:            stock.LogoUrl,
			})
		}
	}

	// Recalculate portfolio totals
	s.recalculatePortfolio(portfolio)

	return &pb.PlaceOrderResponse{
		Order:   order,
		Success: true,
		Message: "order executed successfully",
	}, nil
}

// recalculatePortfolio updates portfolio totals
func (s *StockService) recalculatePortfolio(portfolio *pb.PortfolioMessage) {
	totalValue := portfolio.AvailableCash
	totalCost := portfolio.AvailableCash
	totalInvested := 0.0

	for _, holding := range portfolio.Holdings {
		totalValue += holding.TotalValue
		cost := holding.AverageCost * float64(holding.Shares)
		totalCost += cost
		totalInvested += cost
	}

	portfolio.TotalValue = totalValue
	portfolio.TotalCost = totalCost
	portfolio.TotalInvested = totalInvested
	portfolio.TotalReturn = totalValue - totalCost
	if totalCost > 0 {
		portfolio.TotalReturnPercent = (portfolio.TotalReturn / totalCost) * 100
	}
	portfolio.LastUpdated = timestamppb.Now()
}

// GetUserOrders retrieves user's orders
func (s *StockService) GetUserOrders(ctx context.Context, userID uint, req *pb.GetUserOrdersRequest) (*pb.GetUserOrdersResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	orders := s.mockOrders[userID]
	if orders == nil {
		orders = []*pb.StockOrderMessage{}
	}

	// Filter by symbol if provided
	if req.Symbol != "" {
		filtered := make([]*pb.StockOrderMessage, 0)
		for _, order := range orders {
			if order.Symbol == req.Symbol {
				filtered = append(filtered, order)
			}
		}
		orders = filtered
	}

	// Filter by status if provided
	if req.Status != pb.OrderStatus_ORDER_STATUS_UNSPECIFIED {
		filtered := make([]*pb.StockOrderMessage, 0)
		for _, order := range orders {
			if order.Status == req.Status {
				filtered = append(filtered, order)
			}
		}
		orders = filtered
	}

	return &pb.GetUserOrdersResponse{
		Orders: orders,
		Pagination: &pb.StockPaginationInfo{
			CurrentPage:  1,
			TotalPages:   1,
			TotalItems:   int32(len(orders)),
			ItemsPerPage: int32(len(orders)),
			HasNext:      false,
			HasPrev:      false,
		},
	}, nil
}

// CancelOrder cancels an order
func (s *StockService) CancelOrder(ctx context.Context, userID uint, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	orders := s.mockOrders[userID]
	if orders == nil {
		return &pb.CancelOrderResponse{
			Success: false,
			Message: "order not found",
		}, nil
	}

	for _, order := range orders {
		if order.Id == req.OrderId {
			if order.Status == pb.OrderStatus_ORDER_STATUS_EXECUTED {
				return &pb.CancelOrderResponse{
					Success: false,
					Message: "cannot cancel executed order",
				}, nil
			}

			order.Status = pb.OrderStatus_ORDER_STATUS_CANCELLED
			return &pb.CancelOrderResponse{
				Success: true,
				Message: "order cancelled successfully",
			}, nil
		}
	}

	return &pb.CancelOrderResponse{
		Success: false,
		Message: "order not found",
	}, nil
}

// GetMarketIndices retrieves market indices
func (s *StockService) GetMarketIndices(ctx context.Context, req *pb.GetMarketIndicesRequest) (*pb.GetMarketIndicesResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &pb.GetMarketIndicesResponse{
		Indices: s.mockIndices,
	}, nil
}

// GetTrendingStocks retrieves trending stocks
func (s *StockService) GetTrendingStocks(ctx context.Context, req *pb.GetTrendingStocksRequest) (*pb.GetTrendingStocksResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return stocks with highest volume
	allStocks := make([]*pb.StockMessage, 0, len(s.mockStocks))
	for _, stock := range s.mockStocks {
		allStocks = append(allStocks, stock)
	}

	// Limit results
	limit := req.Limit
	if limit == 0 || limit > int32(len(allStocks)) {
		limit = int32(len(allStocks))
	}

	return &pb.GetTrendingStocksResponse{
		Stocks: allStocks[:limit],
	}, nil
}

// GetTopGainers retrieves top gaining stocks
func (s *StockService) GetTopGainers(ctx context.Context, req *pb.GetTopGainersRequest) (*pb.GetTopGainersResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return stocks with positive changes
	gainers := make([]*pb.StockMessage, 0)
	for _, stock := range s.mockStocks {
		if stock.Change > 0 {
			gainers = append(gainers, stock)
		}
	}

	// Limit results
	limit := req.Limit
	if limit == 0 || limit > int32(len(gainers)) {
		limit = int32(len(gainers))
	}

	return &pb.GetTopGainersResponse{
		Stocks: gainers[:limit],
	}, nil
}

// GetTopLosers retrieves top losing stocks
func (s *StockService) GetTopLosers(ctx context.Context, req *pb.GetTopLosersRequest) (*pb.GetTopLosersResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return stocks with negative changes
	losers := make([]*pb.StockMessage, 0)
	for _, stock := range s.mockStocks {
		if stock.Change < 0 {
			losers = append(losers, stock)
		}
	}

	// Limit results
	limit := req.Limit
	if limit == 0 || limit > int32(len(losers)) {
		limit = int32(len(losers))
	}

	return &pb.GetTopLosersResponse{
		Stocks: losers[:limit],
	}, nil
}

// CreateWatchlist creates a new watchlist
func (s *StockService) CreateWatchlist(ctx context.Context, userID uint, req *pb.CreateWatchlistRequest) (*pb.CreateWatchlistResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.watchlistCounter++
	watchlist := &pb.WatchlistMessage{
		Id:          fmt.Sprintf("watchlist-%d", s.watchlistCounter),
		Name:        req.Name,
		Symbols:     req.Symbols,
		CreatedAt:   timestamppb.Now(),
		LastUpdated: timestamppb.Now(),
		IsDefault:   false,
	}

	if s.mockWatchlists[userID] == nil {
		s.mockWatchlists[userID] = []*pb.WatchlistMessage{}
		watchlist.IsDefault = true
	}

	s.mockWatchlists[userID] = append(s.mockWatchlists[userID], watchlist)

	return &pb.CreateWatchlistResponse{
		Watchlist: watchlist,
	}, nil
}

// GetUserWatchlists retrieves user's watchlists
func (s *StockService) GetUserWatchlists(ctx context.Context, userID uint, req *pb.GetUserWatchlistsRequest) (*pb.GetUserWatchlistsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	watchlists := s.mockWatchlists[userID]
	if watchlists == nil {
		watchlists = []*pb.WatchlistMessage{}
	}

	return &pb.GetUserWatchlistsResponse{
		Watchlists: watchlists,
	}, nil
}

// AddToWatchlist adds a symbol to watchlist
func (s *StockService) AddToWatchlist(ctx context.Context, userID uint, req *pb.AddToWatchlistRequest) (*pb.AddToWatchlistResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	watchlists := s.mockWatchlists[userID]
	if watchlists == nil {
		return &pb.AddToWatchlistResponse{
			Success: false,
		}, fmt.Errorf("watchlist not found")
	}

	for _, watchlist := range watchlists {
		if watchlist.Id == req.WatchlistId {
			// Check if already exists
			for _, symbol := range watchlist.Symbols {
				if symbol == req.Symbol {
					return &pb.AddToWatchlistResponse{
						Success:   true,
						Watchlist: watchlist,
					}, nil
				}
			}

			// Add symbol
			watchlist.Symbols = append(watchlist.Symbols, req.Symbol)
			watchlist.LastUpdated = timestamppb.Now()

			return &pb.AddToWatchlistResponse{
				Success:   true,
				Watchlist: watchlist,
			}, nil
		}
	}

	return &pb.AddToWatchlistResponse{
		Success: false,
	}, fmt.Errorf("watchlist not found")
}

// RemoveFromWatchlist removes a symbol from watchlist
func (s *StockService) RemoveFromWatchlist(ctx context.Context, userID uint, req *pb.RemoveFromWatchlistRequest) (*pb.RemoveFromWatchlistResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	watchlists := s.mockWatchlists[userID]
	if watchlists == nil {
		return &pb.RemoveFromWatchlistResponse{
			Success: false,
		}, fmt.Errorf("watchlist not found")
	}

	for _, watchlist := range watchlists {
		if watchlist.Id == req.WatchlistId {
			// Find and remove symbol
			for i, symbol := range watchlist.Symbols {
				if symbol == req.Symbol {
					watchlist.Symbols = append(watchlist.Symbols[:i], watchlist.Symbols[i+1:]...)
					watchlist.LastUpdated = timestamppb.Now()

					return &pb.RemoveFromWatchlistResponse{
						Success:   true,
						Watchlist: watchlist,
					}, nil
				}
			}

			return &pb.RemoveFromWatchlistResponse{
				Success: false,
			}, fmt.Errorf("symbol not in watchlist")
		}
	}

	return &pb.RemoveFromWatchlistResponse{
		Success: false,
	}, fmt.Errorf("watchlist not found")
}

package grpcApi

import (
	"context"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type StockController struct {
	pb.UnimplementedStockServiceServer
	stockService services.IStockService
	userService  services.IUserService
}

func NewStockController(stockService services.IStockService, userService services.IUserService) *StockController {
	return &StockController{
		stockService: stockService,
		userService:  userService,
	}
}

// GetStocks retrieves multiple stocks with pagination
func (c *StockController) GetStocks(ctx context.Context, req *pb.GetStocksRequest) (*pb.GetStocksResponse, error) {
	resp, err := c.stockService.GetStocks(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get stocks: %v", err)
	}
	return resp, nil
}

// GetStockBySymbol retrieves a single stock by symbol
func (c *StockController) GetStockBySymbol(ctx context.Context, req *pb.GetStockBySymbolRequest) (*pb.GetStockBySymbolResponse, error) {
	if req.Symbol == "" {
		return nil, status.Errorf(codes.InvalidArgument, "symbol is required")
	}

	resp, err := c.stockService.GetStockBySymbol(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "stock not found: %v", err)
	}
	return resp, nil
}

// SearchStocks searches for stocks by query
func (c *StockController) SearchStocks(ctx context.Context, req *pb.SearchStocksRequest) (*pb.SearchStocksResponse, error) {
	if req.Query == "" {
		return nil, status.Errorf(codes.InvalidArgument, "search query is required")
	}

	resp, err := c.stockService.SearchStocks(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search stocks: %v", err)
	}
	return resp, nil
}

// GetStockPriceHistory retrieves price history for a stock
func (c *StockController) GetStockPriceHistory(ctx context.Context, req *pb.GetStockPriceHistoryRequest) (*pb.GetStockPriceHistoryResponse, error) {
	if req.Symbol == "" {
		return nil, status.Errorf(codes.InvalidArgument, "symbol is required")
	}

	resp, err := c.stockService.GetStockPriceHistory(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "stock not found: %v", err)
	}
	return resp, nil
}

// GetUserPortfolio retrieves user's portfolio
func (c *StockController) GetUserPortfolio(ctx context.Context, req *pb.GetUserPortfolioRequest) (*pb.GetUserPortfolioResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	resp, err := c.stockService.GetUserPortfolio(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get portfolio: %v", err)
	}
	return resp, nil
}

// PlaceOrder places a stock order
func (c *StockController) PlaceOrder(ctx context.Context, req *pb.PlaceOrderRequest) (*pb.PlaceOrderResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.Symbol == "" {
		return nil, status.Errorf(codes.InvalidArgument, "symbol is required")
	}
	if req.Type == pb.OrderType_ORDER_TYPE_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "order type is required")
	}
	if req.Side == pb.OrderSide_ORDER_SIDE_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "order side is required")
	}
	if req.Quantity <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "quantity must be greater than 0")
	}
	if req.Type == pb.OrderType_ORDER_TYPE_LIMIT && req.Price <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "price is required for limit orders")
	}

	resp, err := c.stockService.PlaceOrder(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to place order: %v", err)
	}
	return resp, nil
}

// GetUserOrders retrieves user's orders
func (c *StockController) GetUserOrders(ctx context.Context, req *pb.GetUserOrdersRequest) (*pb.GetUserOrdersResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	resp, err := c.stockService.GetUserOrders(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get orders: %v", err)
	}
	return resp, nil
}

// CancelOrder cancels an order
func (c *StockController) CancelOrder(ctx context.Context, req *pb.CancelOrderRequest) (*pb.CancelOrderResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.OrderId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "order id is required")
	}

	resp, err := c.stockService.CancelOrder(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to cancel order: %v", err)
	}
	return resp, nil
}

// GetMarketIndices retrieves market indices
func (c *StockController) GetMarketIndices(ctx context.Context, req *pb.GetMarketIndicesRequest) (*pb.GetMarketIndicesResponse, error) {
	resp, err := c.stockService.GetMarketIndices(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get market indices: %v", err)
	}
	return resp, nil
}

// GetTrendingStocks retrieves trending stocks
func (c *StockController) GetTrendingStocks(ctx context.Context, req *pb.GetTrendingStocksRequest) (*pb.GetTrendingStocksResponse, error) {
	resp, err := c.stockService.GetTrendingStocks(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get trending stocks: %v", err)
	}
	return resp, nil
}

// GetTopGainers retrieves top gaining stocks
func (c *StockController) GetTopGainers(ctx context.Context, req *pb.GetTopGainersRequest) (*pb.GetTopGainersResponse, error) {
	resp, err := c.stockService.GetTopGainers(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get top gainers: %v", err)
	}
	return resp, nil
}

// GetTopLosers retrieves top losing stocks
func (c *StockController) GetTopLosers(ctx context.Context, req *pb.GetTopLosersRequest) (*pb.GetTopLosersResponse, error) {
	resp, err := c.stockService.GetTopLosers(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get top losers: %v", err)
	}
	return resp, nil
}

// CreateWatchlist creates a new watchlist
func (c *StockController) CreateWatchlist(ctx context.Context, req *pb.CreateWatchlistRequest) (*pb.CreateWatchlistResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "watchlist name is required")
	}

	resp, err := c.stockService.CreateWatchlist(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create watchlist: %v", err)
	}
	return resp, nil
}

// GetUserWatchlists retrieves user's watchlists
func (c *StockController) GetUserWatchlists(ctx context.Context, req *pb.GetUserWatchlistsRequest) (*pb.GetUserWatchlistsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	resp, err := c.stockService.GetUserWatchlists(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get watchlists: %v", err)
	}
	return resp, nil
}

// AddToWatchlist adds a symbol to watchlist
func (c *StockController) AddToWatchlist(ctx context.Context, req *pb.AddToWatchlistRequest) (*pb.AddToWatchlistResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.WatchlistId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "watchlist id is required")
	}
	if req.Symbol == "" {
		return nil, status.Errorf(codes.InvalidArgument, "symbol is required")
	}

	resp, err := c.stockService.AddToWatchlist(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to add to watchlist: %v", err)
	}
	return resp, nil
}

// RemoveFromWatchlist removes a symbol from watchlist
func (c *StockController) RemoveFromWatchlist(ctx context.Context, req *pb.RemoveFromWatchlistRequest) (*pb.RemoveFromWatchlistResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.WatchlistId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "watchlist id is required")
	}
	if req.Symbol == "" {
		return nil, status.Errorf(codes.InvalidArgument, "symbol is required")
	}

	resp, err := c.stockService.RemoveFromWatchlist(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove from watchlist: %v", err)
	}
	return resp, nil
}

package grpcApi

import (
	"context"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CryptoController struct {
	pb.UnimplementedCryptoServiceServer
	cryptoService services.ICryptoService
}

func NewCryptoController(cryptoService services.ICryptoService) *CryptoController {
	return &CryptoController{
		cryptoService: cryptoService,
	}
}

// GetCryptos retrieves a list of cryptocurrencies with market data
func (c *CryptoController) GetCryptos(ctx context.Context, req *pb.GetCryptosRequest) (*pb.GetCryptosResponse, error) {
	resp, err := c.cryptoService.GetCryptos(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get cryptocurrencies: %v", err)
	}
	return resp, nil
}

// GetCryptoById retrieves detailed information about a specific cryptocurrency
func (c *CryptoController) GetCryptoById(ctx context.Context, req *pb.GetCryptoByIdRequest) (*pb.GetCryptoByIdResponse, error) {
	if req.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "crypto id is required")
	}

	resp, err := c.cryptoService.GetCryptoById(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get cryptocurrency: %v", err)
	}
	return resp, nil
}

// SearchCryptos searches for cryptocurrencies by name or symbol
func (c *CryptoController) SearchCryptos(ctx context.Context, req *pb.SearchCryptosRequest) (*pb.SearchCryptosResponse, error) {
	if req.Query == "" {
		return nil, status.Errorf(codes.InvalidArgument, "search query is required")
	}

	resp, err := c.cryptoService.SearchCryptos(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search cryptocurrencies: %v", err)
	}
	return resp, nil
}

// GetCryptoPriceHistory retrieves historical price data for a cryptocurrency
func (c *CryptoController) GetCryptoPriceHistory(ctx context.Context, req *pb.GetCryptoPriceHistoryRequest) (*pb.GetCryptoPriceHistoryResponse, error) {
	if req.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "crypto id is required")
	}

	resp, err := c.cryptoService.GetCryptoPriceHistory(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get price history: %v", err)
	}
	return resp, nil
}

// GetTrendingCryptos retrieves currently trending cryptocurrencies
func (c *CryptoController) GetTrendingCryptos(ctx context.Context, req *pb.GetTrendingCryptosRequest) (*pb.GetTrendingCryptosResponse, error) {
	resp, err := c.cryptoService.GetTrendingCryptos(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get trending cryptocurrencies: %v", err)
	}
	return resp, nil
}

// GetTopCryptos retrieves top cryptocurrencies by market cap
func (c *CryptoController) GetTopCryptos(ctx context.Context, req *pb.GetTopCryptosRequest) (*pb.GetTopCryptosResponse, error) {
	resp, err := c.cryptoService.GetTopCryptos(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get top cryptocurrencies: %v", err)
	}
	return resp, nil
}

// GetMarketChart retrieves market chart data (prices, market caps, volumes)
func (c *CryptoController) GetMarketChart(ctx context.Context, req *pb.GetMarketChartRequest) (*pb.GetMarketChartResponse, error) {
	if req.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "crypto id is required")
	}

	resp, err := c.cryptoService.GetMarketChart(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get market chart: %v", err)
	}
	return resp, nil
}

// GetGlobalMarketData retrieves global cryptocurrency market data
func (c *CryptoController) GetGlobalMarketData(ctx context.Context, req *pb.GetGlobalMarketDataRequest) (*pb.GetGlobalMarketDataResponse, error) {
	resp, err := c.cryptoService.GetGlobalMarketData(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get global market data: %v", err)
	}
	return resp, nil
}

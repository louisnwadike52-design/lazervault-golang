package grpcApi

import (
	"context"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PortfolioController handles gRPC requests for portfolio operations
type PortfolioController struct {
	pb.UnimplementedPortfolioServiceServer
	portfolioService *services.PortfolioService
}

// NewPortfolioController creates a new portfolio controller
func NewPortfolioController(portfolioService *services.PortfolioService) *PortfolioController {
	return &PortfolioController{
		portfolioService: portfolioService,
	}
}

// getUserIDFromContext extracts user ID from JWT token
func (c *PortfolioController) getUserIDFromContext(ctx context.Context) (string, error) {
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return "", status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Return email as user identifier (service will look up UUID)
	return authPayload.Email, nil
}

// GetCompletePortfolio retrieves complete portfolio for authenticated user
func (c *PortfolioController) GetCompletePortfolio(ctx context.Context, req *pb.GetCompletePortfolioRequest) (*pb.GetCompletePortfolioResponse, error) {
	log.Println("[PortfolioController] GetCompletePortfolio called")

	// Get user ID from context
	email, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get portfolio data
	portfolio, err := c.portfolioService.GetUserPortfolio(ctx, email)
	if err != nil {
		log.Printf("[PortfolioController] Failed to get portfolio: %v", err)
		return &pb.GetCompletePortfolioResponse{
			Success: false,
			Message: "Failed to retrieve portfolio: " + err.Error(),
		}, nil
	}

	// Convert summary
	summary := &pb.PortfolioSummary{
		TotalValue:           portfolio.Summary.TotalValue,
		TotalGainLoss:        portfolio.Summary.TotalGainLoss,
		TotalGainLossPercent: portfolio.Summary.TotalGainLossPercent,
		TotalInvested:        portfolio.Summary.TotalInvested,
		Currency:             portfolio.Summary.Currency,
		AssetsByType:         portfolio.Summary.AssetsByType,
		AssetCount:           int32(portfolio.Summary.AssetCount),
		LastUpdated:          timestamppb.New(portfolio.Summary.LastUpdated),
	}

	// Convert assets
	assets := make([]*pb.PortfolioAsset, 0, len(portfolio.Assets))
	for _, asset := range portfolio.Assets {
		pbAsset := &pb.PortfolioAsset{
			Id:              asset.ID,
			AssetType:       asset.AssetType,
			Name:            asset.Name,
			Symbol:          asset.Symbol,
			CurrentValue:    asset.CurrentValue,
			Quantity:        asset.Quantity,
			CurrentPrice:    asset.CurrentPrice,
			InitialValue:    asset.InitialValue,
			GainLoss:        asset.GainLoss,
			GainLossPercent: asset.GainLossPercent,
			Currency:        asset.Currency,
			LastUpdated:     timestamppb.New(asset.LastUpdated),
			IconUrl:         asset.IconURL,
		}
		assets = append(assets, pbAsset)
	}

	log.Printf("[PortfolioController] Successfully retrieved portfolio with %d assets, total value: %.2f",
		len(assets), summary.TotalValue)

	return &pb.GetCompletePortfolioResponse{
		Success: true,
		Message: "Portfolio retrieved successfully",
		Summary: summary,
		Assets:  assets,
	}, nil
}

// GetPortfolioByAssetType retrieves portfolio filtered by asset type
func (c *PortfolioController) GetPortfolioByAssetType(ctx context.Context, req *pb.GetPortfolioByAssetTypeRequest) (*pb.GetPortfolioByAssetTypeResponse, error) {
	log.Printf("[PortfolioController] GetPortfolioByAssetType called for type: %s", req.AssetType)

	// Get user ID from context
	email, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Validate asset type
	validTypes := map[string]bool{
		"account":        true,
		"crypto":         true,
		"stock":          true,
		"investment":     true,
		"savings":        true,
		"financial_goal": true,
	}

	if !validTypes[req.AssetType] {
		return &pb.GetPortfolioByAssetTypeResponse{
			Success: false,
			Message: "Invalid asset type",
		}, nil
	}

	// Get filtered assets
	assets, err := c.portfolioService.GetPortfolioByAssetType(ctx, email, req.AssetType)
	if err != nil {
		log.Printf("[PortfolioController] Failed to get portfolio by type: %v", err)
		return &pb.GetPortfolioByAssetTypeResponse{
			Success: false,
			Message: "Failed to retrieve portfolio: " + err.Error(),
		}, nil
	}

	// Convert assets
	pbAssets := make([]*pb.PortfolioAsset, 0, len(assets))
	totalValue := 0.0

	for _, asset := range assets {
		pbAsset := &pb.PortfolioAsset{
			Id:              asset.ID,
			AssetType:       asset.AssetType,
			Name:            asset.Name,
			Symbol:          asset.Symbol,
			CurrentValue:    asset.CurrentValue,
			Quantity:        asset.Quantity,
			CurrentPrice:    asset.CurrentPrice,
			InitialValue:    asset.InitialValue,
			GainLoss:        asset.GainLoss,
			GainLossPercent: asset.GainLossPercent,
			Currency:        asset.Currency,
			LastUpdated:     timestamppb.New(asset.LastUpdated),
			IconUrl:         asset.IconURL,
		}
		pbAssets = append(pbAssets, pbAsset)
		totalValue += asset.CurrentValue
	}

	return &pb.GetPortfolioByAssetTypeResponse{
		Success:    true,
		Message:    "Portfolio retrieved successfully",
		Assets:     pbAssets,
		TotalValue: totalValue,
	}, nil
}

// GetPortfolioHistory retrieves historical portfolio values
func (c *PortfolioController) GetPortfolioHistory(ctx context.Context, req *pb.GetPortfolioHistoryRequest) (*pb.GetPortfolioHistoryResponse, error) {
	log.Printf("[PortfolioController] GetPortfolioHistory called for period: %s", req.Period)

	// Get user ID from context
	email, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get history data
	history, err := c.portfolioService.GetPortfolioHistory(ctx, email, req.Period)
	if err != nil {
		log.Printf("[PortfolioController] Failed to get portfolio history: %v", err)
		return &pb.GetPortfolioHistoryResponse{
			Success: false,
			Message: "Failed to retrieve portfolio history: " + err.Error(),
		}, nil
	}

	// Convert history to proto format
	// For now, we just return current value as placeholder
	historyPoints := []*pb.PortfolioHistoryPoint{}
	for dateStr, value := range history {
		// Parse date and add to history
		_ = dateStr // TODO: Parse date properly
		historyPoints = append(historyPoints, &pb.PortfolioHistoryPoint{
			Date:  timestamppb.Now(),
			Value: value,
		})
	}

	return &pb.GetPortfolioHistoryResponse{
		Success: true,
		Message: "Portfolio history retrieved successfully",
		History: historyPoints,
	}, nil
}

// GetPortfolioSummary retrieves just the portfolio summary
func (c *PortfolioController) GetPortfolioSummary(ctx context.Context, req *pb.GetPortfolioSummaryRequest) (*pb.GetPortfolioSummaryResponse, error) {
	log.Println("[PortfolioController] GetPortfolioSummary called")

	// Get user ID from context
	email, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get portfolio data
	portfolio, err := c.portfolioService.GetUserPortfolio(ctx, email)
	if err != nil {
		log.Printf("[PortfolioController] Failed to get portfolio summary: %v", err)
		return &pb.GetPortfolioSummaryResponse{
			Success: false,
			Message: "Failed to retrieve portfolio summary: " + err.Error(),
		}, nil
	}

	// Convert summary
	summary := &pb.PortfolioSummary{
		TotalValue:           portfolio.Summary.TotalValue,
		TotalGainLoss:        portfolio.Summary.TotalGainLoss,
		TotalGainLossPercent: portfolio.Summary.TotalGainLossPercent,
		TotalInvested:        portfolio.Summary.TotalInvested,
		Currency:             portfolio.Summary.Currency,
		AssetsByType:         portfolio.Summary.AssetsByType,
		AssetCount:           int32(portfolio.Summary.AssetCount),
		LastUpdated:          timestamppb.New(portfolio.Summary.LastUpdated),
	}

	return &pb.GetPortfolioSummaryResponse{
		Success: true,
		Message: "Portfolio summary retrieved successfully",
		Summary: summary,
	}, nil
}

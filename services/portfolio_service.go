package services

import (
	"context"
	"fmt"
	"lazervaultGo/models"
	"time"

	"gorm.io/gorm"
)

// PortfolioService handles portfolio aggregation and analytics
type PortfolioService struct {
	db *gorm.DB
}

// NewPortfolioService creates a new portfolio service
func NewPortfolioService(db *gorm.DB) *PortfolioService {
	return &PortfolioService{db: db}
}

// PortfolioAsset represents a single asset in the portfolio
type PortfolioAsset struct {
	ID              string  `json:"id"`
	AssetType       string  `json:"asset_type"`        // "crypto", "stock", "investment", "account", "savings"
	Name            string  `json:"name"`
	Symbol          string  `json:"symbol"`
	CurrentValue    float64 `json:"current_value"`
	Quantity        float64 `json:"quantity"`
	CurrentPrice    float64 `json:"current_price"`
	InitialValue    float64 `json:"initial_value"`
	GainLoss        float64 `json:"gain_loss"`
	GainLossPercent float64 `json:"gain_loss_percent"`
	Currency        string  `json:"currency"`
	LastUpdated     time.Time `json:"last_updated"`
	IconURL         string  `json:"icon_url,omitempty"`
}

// PortfolioSummary contains overall portfolio statistics
type PortfolioSummary struct {
	TotalValue           float64            `json:"total_value"`
	TotalGainLoss        float64            `json:"total_gain_loss"`
	TotalGainLossPercent float64            `json:"total_gain_loss_percent"`
	TotalInvested        float64            `json:"total_invested"`
	Currency             string             `json:"currency"`
	AssetsByType         map[string]float64 `json:"assets_by_type"`
	AssetCount           int                `json:"asset_count"`
	LastUpdated          time.Time          `json:"last_updated"`
}

// PortfolioResponse contains the complete portfolio data
type PortfolioResponse struct {
	Summary PortfolioSummary `json:"summary"`
	Assets  []PortfolioAsset `json:"assets"`
}

// GetUserPortfolio retrieves and aggregates all portfolio data for a user
func (s *PortfolioService) GetUserPortfolio(ctx context.Context, userIdentifier string) (*PortfolioResponse, error) {
	// Get user's numeric ID from email
	var user models.User
	fmt.Printf("[PortfolioService] Looking up user by email: %s\n", userIdentifier)
	if err := s.db.WithContext(ctx).Where("email = ?", userIdentifier).First(&user).Error; err != nil {
		fmt.Printf("[PortfolioService] Error fetching user: %v\n", err)
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}
	fmt.Printf("[PortfolioService] Found user ID: %d, UUID: %s\n", user.ID, user.UUID)

	assets := []PortfolioAsset{}
	totalValue := 0.0
	totalInvested := 0.0
	assetsByType := make(map[string]float64)

	// 1. Get Accounts
	accountAssets, accountValue, accountInitial, err := s.getAccountAssets(ctx, user.ID)
	if err == nil {
		assets = append(assets, accountAssets...)
		totalValue += accountValue
		totalInvested += accountInitial
		assetsByType["accounts"] = accountValue
	}

	// 2. Get Investments (Stocks, Crypto, etc.)
	investmentAssets, investmentValue, investmentInitial, err := s.getInvestmentAssets(ctx, user.ID)
	if err == nil {
		assets = append(assets, investmentAssets...)
		totalValue += investmentValue
		totalInvested += investmentInitial
		assetsByType["investments"] = investmentValue
	}

	// 3. Get Savings Goals
	savingsAssets, savingsValue, savingsInitial, err := s.getSavingsGoalAssets(ctx, user.ID)
	if err == nil {
		assets = append(assets, savingsAssets...)
		totalValue += savingsValue
		totalInvested += savingsInitial
		assetsByType["savings"] = savingsValue
	}

	// 4. Get Financial Goals
	goalAssets, goalValue, goalInitial, err := s.getFinancialGoalAssets(ctx, user.ID)
	if err == nil {
		assets = append(assets, goalAssets...)
		totalValue += goalValue
		totalInvested += goalInitial
		assetsByType["goals"] = goalValue
	}

	// Calculate overall gain/loss
	totalGainLoss := totalValue - totalInvested
	totalGainLossPercent := 0.0
	if totalInvested > 0 {
		totalGainLossPercent = (totalGainLoss / totalInvested) * 100
	}

	summary := PortfolioSummary{
		TotalValue:           totalValue,
		TotalGainLoss:        totalGainLoss,
		TotalGainLossPercent: totalGainLossPercent,
		TotalInvested:        totalInvested,
		Currency:             "USD", // Default currency
		AssetsByType:         assetsByType,
		AssetCount:           len(assets),
		LastUpdated:          time.Now(),
	}

	return &PortfolioResponse{
		Summary: summary,
		Assets:  assets,
	}, nil
}

// getAccountAssets retrieves account balances as portfolio assets
func (s *PortfolioService) getAccountAssets(ctx context.Context, userID uint) ([]PortfolioAsset, float64, float64, error) {
	var accounts []models.Account
	if err := s.db.WithContext(ctx).
		Where("owner_user_id = ? AND is_active = ?", userID, true).
		Find(&accounts).Error; err != nil {
		return nil, 0, 0, err
	}

	assets := make([]PortfolioAsset, 0, len(accounts))
	totalValue := 0.0

	for _, account := range accounts {
		value := float64(account.Balance) / 100.0 // Convert cents to dollars

		asset := PortfolioAsset{
			ID:           fmt.Sprintf("account_%d", account.ID),
			AssetType:    "account",
			Name:         fmt.Sprintf("%s Account", account.AccountType),
			Symbol:       account.Currency,
			CurrentValue: value,
			Quantity:     1,
			CurrentPrice: value,
			InitialValue: value, // Accounts don't have initial value, so current = initial
			GainLoss:     0,
			GainLossPercent: 0,
			Currency:     account.Currency,
			LastUpdated:  account.UpdatedAt,
		}

		assets = append(assets, asset)
		totalValue += value
	}

	return assets, totalValue, totalValue, nil
}

// getInvestmentAssets retrieves investments as portfolio assets
func (s *PortfolioService) getInvestmentAssets(ctx context.Context, userID uint) ([]PortfolioAsset, float64, float64, error) {
	var investments []models.Investment
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Find(&investments).Error; err != nil {
		return nil, 0, 0, err
	}

	assets := make([]PortfolioAsset, 0, len(investments))
	totalValue := 0.0
	totalInitial := 0.0

	for _, inv := range investments {
		currentValue := float64(inv.CurrentValue)
		initialValue := float64(inv.InitialInvestment)
		gainLoss := float64(inv.GainLoss)
		gainLossPercent := float64(inv.GainLossPercentage)

		asset := PortfolioAsset{
			ID:              inv.ID.String(),
			AssetType:       inv.InvestmentType,
			Name:            inv.Name,
			Symbol:          inv.TickerSymbol,
			CurrentValue:    currentValue,
			Quantity:        float64(inv.Quantity),
			CurrentPrice:    float64(inv.CurrentPrice),
			InitialValue:    initialValue,
			GainLoss:        gainLoss,
			GainLossPercent: gainLossPercent,
			Currency:        inv.Currency,
			LastUpdated:     inv.LastUpdated,
		}

		assets = append(assets, asset)
		totalValue += currentValue
		totalInitial += initialValue
	}

	return assets, totalValue, totalInitial, nil
}

// getSavingsGoalAssets retrieves savings goals as portfolio assets
func (s *PortfolioService) getSavingsGoalAssets(ctx context.Context, userID uint) ([]PortfolioAsset, float64, float64, error) {
	var savingsGoals []models.SavingsGoal
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Find(&savingsGoals).Error; err != nil {
		return nil, 0, 0, err
	}

	assets := make([]PortfolioAsset, 0, len(savingsGoals))
	totalValue := 0.0

	for _, goal := range savingsGoals {
		currentAmount := float64(goal.CurrentAmount)

		asset := PortfolioAsset{
			ID:           goal.ID.String(),
			AssetType:    "savings",
			Name:         goal.Name,
			Symbol:       "USD", // Default currency for savings
			CurrentValue: currentAmount,
			Quantity:     1,
			CurrentPrice: currentAmount,
			InitialValue: 0, // Savings start at 0
			GainLoss:     currentAmount,
			GainLossPercent: 0,
			Currency:     "USD",
			LastUpdated:  goal.UpdatedAt,
		}

		assets = append(assets, asset)
		totalValue += currentAmount
	}

	return assets, totalValue, 0, nil
}

// getFinancialGoalAssets retrieves financial goals as portfolio assets
func (s *PortfolioService) getFinancialGoalAssets(ctx context.Context, userID uint) ([]PortfolioAsset, float64, float64, error) {
	var financialGoals []models.FinancialGoal
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Find(&financialGoals).Error; err != nil {
		return nil, 0, 0, err
	}

	assets := make([]PortfolioAsset, 0, len(financialGoals))
	totalValue := 0.0

	for _, goal := range financialGoals {
		currentAmount := float64(goal.CurrentAmount)

		asset := PortfolioAsset{
			ID:           goal.ID.String(),
			AssetType:    "financial_goal",
			Name:         goal.Name,
			Symbol:       goal.Currency,
			CurrentValue: currentAmount,
			Quantity:     1,
			CurrentPrice: currentAmount,
			InitialValue: 0,
			GainLoss:     currentAmount,
			GainLossPercent: 0,
			Currency:     goal.Currency,
			LastUpdated:  goal.UpdatedAt,
		}

		assets = append(assets, asset)
		totalValue += currentAmount
	}

	return assets, totalValue, 0, nil
}

// GetPortfolioByAssetType gets portfolio data filtered by asset type
func (s *PortfolioService) GetPortfolioByAssetType(ctx context.Context, userID string, assetType string) ([]PortfolioAsset, error) {
	portfolio, err := s.GetUserPortfolio(ctx, userID)
	if err != nil {
		return nil, err
	}

	filtered := []PortfolioAsset{}
	for _, asset := range portfolio.Assets {
		if asset.AssetType == assetType {
			filtered = append(filtered, asset)
		}
	}

	return filtered, nil
}

// GetPortfolioHistory gets historical portfolio value (placeholder for future implementation)
func (s *PortfolioService) GetPortfolioHistory(ctx context.Context, userID string, period string) (map[string]float64, error) {
	// TODO: Implement historical portfolio tracking
	// For now, return current value
	portfolio, err := s.GetUserPortfolio(ctx, userID)
	if err != nil {
		return nil, err
	}

	history := map[string]float64{
		time.Now().Format("2006-01-02"): portfolio.Summary.TotalValue,
	}

	return history, nil
}

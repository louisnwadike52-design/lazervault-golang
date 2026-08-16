package proxy

import (
	"context"

	accountspb "accounts-service/proto"
)

// AccountsServiceProxy proxies AccountsService gRPC requests to the upstream accounts microservice
type AccountsServiceProxy struct {
	accountspb.UnimplementedAccountsServiceServer
	client accountspb.AccountsServiceClient
}

// NewAccountsServiceProxy creates a new AccountsServiceProxy
func NewAccountsServiceProxy(client accountspb.AccountsServiceClient) *AccountsServiceProxy {
	return &AccountsServiceProxy{
		client: client,
	}
}

// Implement critical AccountsService methods

func (p *AccountsServiceProxy) CreateAccount(ctx context.Context, req *accountspb.CreateAccountRequest) (*accountspb.CreateAccountResponse, error) {
	return p.client.CreateAccount(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetAccounts(ctx context.Context, req *accountspb.GetAccountsRequest) (*accountspb.GetAccountsResponse, error) {
	return p.client.GetAccounts(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetAccount(ctx context.Context, req *accountspb.GetAccountRequest) (*accountspb.GetAccountResponse, error) {
	return p.client.GetAccount(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) UpdateAccount(ctx context.Context, req *accountspb.UpdateAccountRequest) (*accountspb.UpdateAccountResponse, error) {
	return p.client.UpdateAccount(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) DeleteAccount(ctx context.Context, req *accountspb.DeleteAccountRequest) (*accountspb.DeleteAccountResponse, error) {
	return p.client.DeleteAccount(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetBalance(ctx context.Context, req *accountspb.GetBalanceRequest) (*accountspb.GetBalanceResponse, error) {
	return p.client.GetBalance(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetTransactions(ctx context.Context, req *accountspb.GetTransactionsRequest) (*accountspb.GetTransactionsResponse, error) {
	return p.client.GetTransactions(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetAccountByNumber(ctx context.Context, req *accountspb.GetAccountByNumberRequest) (*accountspb.GetAccountByNumberResponse, error) {
	return p.client.GetAccountByNumber(forwardContext(ctx), req)
}

// GetUserAccounts proxies the request for account summaries (dashboard carousel)
func (p *AccountsServiceProxy) GetUserAccounts(ctx context.Context, req *accountspb.GetUserAccountsRequest) (*accountspb.GetUserAccountsResponse, error) {
	return p.client.GetUserAccounts(forwardContext(ctx), req)
}

// === Transaction History Proxies ===

func (p *AccountsServiceProxy) CreateTransaction(ctx context.Context, req *accountspb.CreateTransactionRequest) (*accountspb.CreateTransactionResponse, error) {
	return p.client.CreateTransaction(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) UpdateTransactionStatus(ctx context.Context, req *accountspb.UpdateTransactionStatusRequest) (*accountspb.UpdateTransactionStatusResponse, error) {
	return p.client.UpdateTransactionStatus(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetTransactionHistory(ctx context.Context, req *accountspb.GetTransactionHistoryRequest) (*accountspb.GetTransactionHistoryResponse, error) {
	return p.client.GetTransactionHistory(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetTransactionStatistics(ctx context.Context, req *accountspb.GetTransactionStatisticsRequest) (*accountspb.GetTransactionStatisticsResponse, error) {
	return p.client.GetTransactionStatistics(forwardContext(ctx), req)
}

// === Financial Analytics Proxies ===

func (p *AccountsServiceProxy) GetFinancialAnalytics(ctx context.Context, req *accountspb.GetFinancialAnalyticsRequest) (*accountspb.GetFinancialAnalyticsResponse, error) {
	return p.client.GetFinancialAnalytics(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetCategoryAnalytics(ctx context.Context, req *accountspb.GetCategoryAnalyticsRequest) (*accountspb.GetCategoryAnalyticsResponse, error) {
	return p.client.GetCategoryAnalytics(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetMonthlyTrends(ctx context.Context, req *accountspb.GetMonthlyTrendsRequest) (*accountspb.GetMonthlyTrendsResponse, error) {
	return p.client.GetMonthlyTrends(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetExpenseTimeSeries(ctx context.Context, req *accountspb.GetExpenseTimeSeriesRequest) (*accountspb.GetExpenseTimeSeriesResponse, error) {
	return p.client.GetExpenseTimeSeries(forwardContext(ctx), req)
}

// === Account Freeze/Unfreeze Proxies ===

func (p *AccountsServiceProxy) FreezeAccount(ctx context.Context, req *accountspb.FreezeAccountRequest) (*accountspb.FreezeAccountResponse, error) {
	return p.client.FreezeAccount(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) UnfreezeAccount(ctx context.Context, req *accountspb.UnfreezeAccountRequest) (*accountspb.UnfreezeAccountResponse, error) {
	return p.client.UnfreezeAccount(forwardContext(ctx), req)
}

// === Card Management Proxies ===

func (p *AccountsServiceProxy) UpdateSpendingLimits(ctx context.Context, req *accountspb.UpdateSpendingLimitsRequest) (*accountspb.UpdateSpendingLimitsResponse, error) {
	return p.client.UpdateSpendingLimits(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GetSpendingUsage(ctx context.Context, req *accountspb.GetSpendingUsageRequest) (*accountspb.GetSpendingUsageResponse, error) {
	return p.client.GetSpendingUsage(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) RevealPIN(ctx context.Context, req *accountspb.RevealPINRequest) (*accountspb.RevealPINResponse, error) {
	return p.client.RevealPIN(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) RevealCardDetails(ctx context.Context, req *accountspb.RevealCardDetailsRequest) (*accountspb.RevealCardDetailsResponse, error) {
	return p.client.RevealCardDetails(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) UpdateSecuritySettings(ctx context.Context, req *accountspb.UpdateSecuritySettingsRequest) (*accountspb.UpdateSecuritySettingsResponse, error) {
	return p.client.UpdateSecuritySettings(forwardContext(ctx), req)
}

// === Category Management Proxies ===

func (p *AccountsServiceProxy) GetUserCategoryMappings(ctx context.Context, req *accountspb.GetUserCategoryMappingsRequest) (*accountspb.GetUserCategoryMappingsResponse, error) {
	return p.client.GetUserCategoryMappings(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) UpdateUserCategoryMapping(ctx context.Context, req *accountspb.UpdateUserCategoryMappingRequest) (*accountspb.UpdateUserCategoryMappingResponse, error) {
	return p.client.UpdateUserCategoryMapping(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) ReorderCategories(ctx context.Context, req *accountspb.ReorderCategoriesRequest) (*accountspb.ReorderCategoriesResponse, error) {
	return p.client.ReorderCategories(forwardContext(ctx), req)
}

// === Document Generation Proxies ===

func (p *AccountsServiceProxy) GenerateStatement(ctx context.Context, req *accountspb.GenerateStatementRequest) (*accountspb.GenerateStatementResponse, error) {
	return p.client.GenerateStatement(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GenerateAccountConfirmation(ctx context.Context, req *accountspb.GenerateAccountConfirmationRequest) (*accountspb.GenerateAccountConfirmationResponse, error) {
	return p.client.GenerateAccountConfirmation(forwardContext(ctx), req)
}

func (p *AccountsServiceProxy) GenerateProofOfFunds(ctx context.Context, req *accountspb.GenerateProofOfFundsRequest) (*accountspb.GenerateProofOfFundsResponse, error) {
	return p.client.GenerateProofOfFunds(forwardContext(ctx), req)
}

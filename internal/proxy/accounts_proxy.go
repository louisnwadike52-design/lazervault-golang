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

// Additional methods can be added as needed
// Any unimplemented methods will return "Unimplemented" error by default

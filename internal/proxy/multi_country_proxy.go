package proxy

import (
	"context"

	accountspb "accounts-service/proto"
)

// MultiCountryServiceProxy proxies MultiCountryAccountService gRPC requests to the upstream accounts microservice
type MultiCountryServiceProxy struct {
	accountspb.UnimplementedMultiCountryAccountServiceServer
	client accountspb.MultiCountryAccountServiceClient
}

// NewMultiCountryServiceProxy creates a new MultiCountryServiceProxy
func NewMultiCountryServiceProxy(client accountspb.MultiCountryAccountServiceClient) *MultiCountryServiceProxy {
	return &MultiCountryServiceProxy{
		client: client,
	}
}

// GetAccountsByLocale returns user's accounts grouped by locale
func (p *MultiCountryServiceProxy) GetAccountsByLocale(ctx context.Context, req *accountspb.GetAccountsByLocaleRequest) (*accountspb.GetAccountsByLocaleResponse, error) {
	return p.client.GetAccountsByLocale(forwardContext(ctx), req)
}

// CreateLocaleAccount creates an account for a specific locale
func (p *MultiCountryServiceProxy) CreateLocaleAccount(ctx context.Context, req *accountspb.CreateLocaleAccountRequest) (*accountspb.CreateLocaleAccountResponse, error) {
	return p.client.CreateLocaleAccount(forwardContext(ctx), req)
}

// GetSupportedLocales returns all supported locales
func (p *MultiCountryServiceProxy) GetSupportedLocales(ctx context.Context, req *accountspb.GetSupportedLocalesRequest) (*accountspb.GetSupportedLocalesResponse, error) {
	return p.client.GetSupportedLocales(forwardContext(ctx), req)
}

// GetUserLocale returns user's active locale preference
func (p *MultiCountryServiceProxy) GetUserLocale(ctx context.Context, req *accountspb.GetUserLocaleRequest) (*accountspb.GetUserLocaleResponse, error) {
	return p.client.GetUserLocale(forwardContext(ctx), req)
}

// SetUserLocale sets user's active locale preference
func (p *MultiCountryServiceProxy) SetUserLocale(ctx context.Context, req *accountspb.SetUserLocaleRequest) (*accountspb.SetUserLocaleResponse, error) {
	return p.client.SetUserLocale(forwardContext(ctx), req)
}

// TriggerMultiCountryCreation triggers background account creation for all locales
func (p *MultiCountryServiceProxy) TriggerMultiCountryCreation(ctx context.Context, req *accountspb.TriggerMultiCountryCreationRequest) (*accountspb.TriggerMultiCountryCreationResponse, error) {
	return p.client.TriggerMultiCountryCreation(forwardContext(ctx), req)
}

// GetAccountCreationStatus returns the status of multi-country account creation
func (p *MultiCountryServiceProxy) GetAccountCreationStatus(ctx context.Context, req *accountspb.GetAccountCreationStatusRequest) (*accountspb.GetAccountCreationStatusResponse, error) {
	return p.client.GetAccountCreationStatus(forwardContext(ctx), req)
}

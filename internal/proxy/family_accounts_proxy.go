package proxy

import (
	"context"

	accountspb "accounts-service/proto"
)

// FamilyAccountsServiceProxy proxies FamilyAccountsService gRPC requests to the upstream accounts microservice
type FamilyAccountsServiceProxy struct {
	accountspb.UnimplementedFamilyAccountsServiceServer
	client accountspb.FamilyAccountsServiceClient
}

// NewFamilyAccountsServiceProxy creates a new FamilyAccountsServiceProxy
func NewFamilyAccountsServiceProxy(client accountspb.FamilyAccountsServiceClient) *FamilyAccountsServiceProxy {
	return &FamilyAccountsServiceProxy{
		client: client,
	}
}

// Implement critical FamilyAccountsService methods

func (p *FamilyAccountsServiceProxy) CreateFamilyAccount(ctx context.Context, req *accountspb.CreateFamilyAccountRequest) (*accountspb.CreateFamilyAccountResponse, error) {
	return p.client.CreateFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) GetFamilyAccount(ctx context.Context, req *accountspb.GetFamilyAccountRequest) (*accountspb.GetFamilyAccountResponse, error) {
	return p.client.GetFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AddFamilyMember(ctx context.Context, req *accountspb.AddFamilyMemberRequest) (*accountspb.AddFamilyMemberResponse, error) {
	return p.client.AddFamilyMember(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) RemoveFamilyMember(ctx context.Context, req *accountspb.RemoveFamilyMemberRequest) (*accountspb.RemoveFamilyMemberResponse, error) {
	return p.client.RemoveFamilyMember(forwardContext(ctx), req)
}

// Additional methods can be added as needed
// Any unimplemented methods will return "Unimplemented" error by default

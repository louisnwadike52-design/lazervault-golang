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

func (p *FamilyAccountsServiceProxy) GetFamilyAccounts(ctx context.Context, req *accountspb.GetFamilyAccountsRequest) (*accountspb.GetFamilyAccountsResponse, error) {
	return p.client.GetFamilyAccounts(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) UpdateFamilyMember(ctx context.Context, req *accountspb.UpdateFamilyMemberRequest) (*accountspb.UpdateFamilyMemberResponse, error) {
	return p.client.UpdateFamilyMember(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AcceptFamilyInvitation(ctx context.Context, req *accountspb.AcceptFamilyInvitationRequest) (*accountspb.AcceptFamilyInvitationResponse, error) {
	return p.client.AcceptFamilyInvitation(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) DeclineFamilyInvitation(ctx context.Context, req *accountspb.DeclineFamilyInvitationRequest) (*accountspb.DeclineFamilyInvitationResponse, error) {
	return p.client.DeclineFamilyInvitation(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) GetPendingInvitations(ctx context.Context, req *accountspb.GetPendingInvitationsRequest) (*accountspb.GetPendingInvitationsResponse, error) {
	return p.client.GetPendingInvitations(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) GetFamilyTransactions(ctx context.Context, req *accountspb.GetFamilyTransactionsRequest) (*accountspb.GetFamilyTransactionsResponse, error) {
	return p.client.GetFamilyTransactions(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AllocateFunds(ctx context.Context, req *accountspb.AllocateFundsRequest) (*accountspb.AllocateFundsResponse, error) {
	return p.client.AllocateFunds(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) GenerateMemberCard(ctx context.Context, req *accountspb.GenerateMemberCardRequest) (*accountspb.GenerateMemberCardResponse, error) {
	return p.client.GenerateMemberCard(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) FreezeFamilyAccount(ctx context.Context, req *accountspb.FreezeFamilyAccountRequest) (*accountspb.FreezeFamilyAccountResponse, error) {
	return p.client.FreezeFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) UnfreezeFamilyAccount(ctx context.Context, req *accountspb.UnfreezeFamilyAccountRequest) (*accountspb.UnfreezeFamilyAccountResponse, error) {
	return p.client.UnfreezeFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) DeleteFamilyAccount(ctx context.Context, req *accountspb.DeleteFamilyAccountRequest) (*accountspb.DeleteFamilyAccountResponse, error) {
	return p.client.DeleteFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) ProcessMemberContribution(ctx context.Context, req *accountspb.ProcessMemberContributionRequest) (*accountspb.ProcessMemberContributionResponse, error) {
	return p.client.ProcessMemberContribution(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) SetupFamilyAccount(ctx context.Context, req *accountspb.SetupFamilyAccountRequest) (*accountspb.SetupFamilyAccountResponse, error) {
	return p.client.SetupFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) UpdateFundDistributionMode(ctx context.Context, req *accountspb.UpdateFundDistributionModeRequest) (*accountspb.UpdateFundDistributionModeResponse, error) {
	return p.client.UpdateFundDistributionMode(forwardContext(ctx), req)
}

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

func (p *FamilyAccountsServiceProxy) GetMyInvitationHistory(ctx context.Context, req *accountspb.GetMyInvitationHistoryRequest) (*accountspb.GetMyInvitationHistoryResponse, error) {
	return p.client.GetMyInvitationHistory(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) GetSentInvitations(ctx context.Context, req *accountspb.GetSentInvitationsRequest) (*accountspb.GetSentInvitationsResponse, error) {
	return p.client.GetSentInvitations(forwardContext(ctx), req)
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

// =============================================================================
// Admin proxy methods — forward admin-only RPCs to the upstream microservice.
// The admin role check is performed by the gateway's HTTP middleware before
// any of these are reached.
// =============================================================================

func (p *FamilyAccountsServiceProxy) AdminListFamilyAccounts(ctx context.Context, req *accountspb.AdminListFamilyAccountsRequest) (*accountspb.AdminListFamilyAccountsResponse, error) {
	return p.client.AdminListFamilyAccounts(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AdminGetFamilyAccount(ctx context.Context, req *accountspb.AdminGetFamilyAccountRequest) (*accountspb.AdminGetFamilyAccountResponse, error) {
	return p.client.AdminGetFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AdminFreezeFamilyAccount(ctx context.Context, req *accountspb.AdminFreezeFamilyAccountRequest) (*accountspb.AdminFreezeFamilyAccountResponse, error) {
	return p.client.AdminFreezeFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AdminUnfreezeFamilyAccount(ctx context.Context, req *accountspb.AdminUnfreezeFamilyAccountRequest) (*accountspb.AdminUnfreezeFamilyAccountResponse, error) {
	return p.client.AdminUnfreezeFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AdminDeleteFamilyAccount(ctx context.Context, req *accountspb.AdminDeleteFamilyAccountRequest) (*accountspb.AdminDeleteFamilyAccountResponse, error) {
	return p.client.AdminDeleteFamilyAccount(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AdminForceAllocateFunds(ctx context.Context, req *accountspb.AdminForceAllocateFundsRequest) (*accountspb.AdminForceAllocateFundsResponse, error) {
	return p.client.AdminForceAllocateFunds(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AdminRemoveFamilyMember(ctx context.Context, req *accountspb.AdminRemoveFamilyMemberRequest) (*accountspb.AdminRemoveFamilyMemberResponse, error) {
	return p.client.AdminRemoveFamilyMember(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AdminUpdateFamilyAccountNotes(ctx context.Context, req *accountspb.AdminUpdateFamilyAccountNotesRequest) (*accountspb.AdminUpdateFamilyAccountNotesResponse, error) {
	return p.client.AdminUpdateFamilyAccountNotes(forwardContext(ctx), req)
}

// LeaveFamilyAccount was missing here while the app called it over gRPC and
// accounts-service implemented it — so "Leave family account" returned
// Unimplemented to every user who tapped it.
func (p *FamilyAccountsServiceProxy) LeaveFamilyAccount(ctx context.Context, req *accountspb.LeaveFamilyAccountRequest) (*accountspb.LeaveFamilyAccountResponse, error) {
	return p.client.LeaveFamilyAccount(forwardContext(ctx), req)
}

// Paid family slots.
//
// This proxy embeds UnimplementedFamilyAccountsServiceServer, so an RPC missing
// from here does NOT fail the build — it returns Unimplemented to the app while
// the microservice behind it works perfectly. Every RPC added to the proto needs
// a line here.

func (p *FamilyAccountsServiceProxy) GetFamilyCapacity(ctx context.Context, req *accountspb.GetFamilyCapacityRequest) (*accountspb.GetFamilyCapacityResponse, error) {
	return p.client.GetFamilyCapacity(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) RequestExtraFamilySlot(ctx context.Context, req *accountspb.RequestExtraFamilySlotRequest) (*accountspb.RequestExtraFamilySlotResponse, error) {
	return p.client.RequestExtraFamilySlot(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) ListFamilySlots(ctx context.Context, req *accountspb.ListFamilySlotsRequest) (*accountspb.ListFamilySlotsResponse, error) {
	return p.client.ListFamilySlots(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) CancelFamilySlot(ctx context.Context, req *accountspb.CancelFamilySlotRequest) (*accountspb.CancelFamilySlotResponse, error) {
	return p.client.CancelFamilySlot(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) GetFamilySlotCharges(ctx context.Context, req *accountspb.GetFamilySlotChargesRequest) (*accountspb.GetFamilySlotChargesResponse, error) {
	return p.client.GetFamilySlotCharges(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AdminListFamilySlots(ctx context.Context, req *accountspb.AdminListFamilySlotsRequest) (*accountspb.AdminListFamilySlotsResponse, error) {
	return p.client.AdminListFamilySlots(forwardContext(ctx), req)
}

func (p *FamilyAccountsServiceProxy) AdminListFamilySlotCharges(ctx context.Context, req *accountspb.AdminListFamilySlotChargesRequest) (*accountspb.AdminListFamilySlotChargesResponse, error) {
	return p.client.AdminListFamilySlotCharges(forwardContext(ctx), req)
}

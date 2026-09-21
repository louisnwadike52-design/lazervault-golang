package proxy

import (
	"context"

	pb "lazervaultGo/pb"
)

// ReferralServiceProxy proxies ReferralService gRPC requests to referral-service
type ReferralServiceProxy struct {
	pb.UnimplementedReferralServiceServer
	client pb.ReferralServiceClient
}

// NewReferralServiceProxy creates a new ReferralServiceProxy
func NewReferralServiceProxy(client pb.ReferralServiceClient) *ReferralServiceProxy {
	return &ReferralServiceProxy{
		client: client,
	}
}

func (p *ReferralServiceProxy) ValidateReferralCode(ctx context.Context, req *pb.ValidateReferralCodeRequest) (*pb.ValidateReferralCodeResponse, error) {
	return p.client.ValidateReferralCode(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) GetMyReferralCode(ctx context.Context, req *pb.GetMyReferralCodeRequest) (*pb.GetMyReferralCodeResponse, error) {
	return p.client.GetMyReferralCode(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) GetMyReferralStats(ctx context.Context, req *pb.GetMyReferralStatsRequest) (*pb.GetMyReferralStatsResponse, error) {
	return p.client.GetMyReferralStats(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) GetMyReferrals(ctx context.Context, req *pb.GetMyReferralsRequest) (*pb.GetMyReferralsResponse, error) {
	return p.client.GetMyReferrals(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) GetReferralLeaderboard(ctx context.Context, req *pb.GetReferralLeaderboardRequest) (*pb.GetReferralLeaderboardResponse, error) {
	return p.client.GetReferralLeaderboard(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) GetCountryRewardConfig(ctx context.Context, req *pb.GetCountryRewardConfigRequest) (*pb.GetCountryRewardConfigResponse, error) {
	return p.client.GetCountryRewardConfig(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) RecordReferral(ctx context.Context, req *pb.RecordReferralRequest) (*pb.RecordReferralResponse, error) {
	return p.client.RecordReferral(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) CreditReferralRewards(ctx context.Context, req *pb.CreditReferralRewardsRequest) (*pb.CreditReferralRewardsResponse, error) {
	return p.client.CreditReferralRewards(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) GetMyPointsBalance(ctx context.Context, req *pb.GetMyPointsBalanceRequest) (*pb.GetMyPointsBalanceResponse, error) {
	return p.client.GetMyPointsBalance(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) GetMyPointsHistory(ctx context.Context, req *pb.GetMyPointsHistoryRequest) (*pb.GetMyPointsHistoryResponse, error) {
	return p.client.GetMyPointsHistory(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) GetPointsConfig(ctx context.Context, req *pb.GetPointsConfigRequest) (*pb.GetPointsConfigResponse, error) {
	return p.client.GetPointsConfig(forwardContext(ctx), req)
}

// GetRedemptionQuote and RedeemPoints move MONEY (points -> wallet cash), so
// both forward the caller's context rather than dialling with the gateway's own
// identity — referral-service resolves the user from the forwarded token and
// must never take the redeeming user from the request body.
//
// Note what is NOT here: AwardTransactionPoints / ReverseTransactionPoints.
// Those mint and claw back points and are called service-to-service on the
// internal port. They stay off the public gateway.
func (p *ReferralServiceProxy) GetRedemptionQuote(ctx context.Context, req *pb.GetRedemptionQuoteRequest) (*pb.GetRedemptionQuoteResponse, error) {
	return p.client.GetRedemptionQuote(forwardContext(ctx), req)
}

func (p *ReferralServiceProxy) RedeemPoints(ctx context.Context, req *pb.RedeemPointsRequest) (*pb.RedeemPointsResponse, error) {
	return p.client.RedeemPoints(forwardContext(ctx), req)
}

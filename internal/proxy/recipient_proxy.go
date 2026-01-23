package proxy

import (
	"context"

	accountspb "accounts-service/proto"
)

// RecipientServiceProxy proxies RecipientService gRPC requests to the upstream accounts microservice
type RecipientServiceProxy struct {
	accountspb.UnimplementedRecipientServiceServer
	client accountspb.RecipientServiceClient
}

// NewRecipientServiceProxy creates a new RecipientServiceProxy
func NewRecipientServiceProxy(client accountspb.RecipientServiceClient) *RecipientServiceProxy {
	return &RecipientServiceProxy{
		client: client,
	}
}

// Implement RecipientService methods

func (p *RecipientServiceProxy) CreateRecipient(ctx context.Context, req *accountspb.CreateRecipientRequest) (*accountspb.CreateRecipientResponse, error) {
	return p.client.CreateRecipient(forwardContext(ctx), req)
}

func (p *RecipientServiceProxy) ListRecipients(ctx context.Context, req *accountspb.ListRecipientsRequest) (*accountspb.ListRecipientsResponse, error) {
	return p.client.ListRecipients(forwardContext(ctx), req)
}

func (p *RecipientServiceProxy) UpdateRecipient(ctx context.Context, req *accountspb.UpdateRecipientRequest) (*accountspb.UpdateRecipientResponse, error) {
	return p.client.UpdateRecipient(forwardContext(ctx), req)
}

func (p *RecipientServiceProxy) DeleteRecipient(ctx context.Context, req *accountspb.DeleteRecipientRequest) (*accountspb.DeleteRecipientResponse, error) {
	return p.client.DeleteRecipient(forwardContext(ctx), req)
}

func (p *RecipientServiceProxy) GetRecipient(ctx context.Context, req *accountspb.GetRecipientRequest) (*accountspb.GetRecipientResponse, error) {
	return p.client.GetRecipient(forwardContext(ctx), req)
}

func (p *RecipientServiceProxy) GetSimilarRecipientsByName(ctx context.Context, req *accountspb.GetSimilarRecipientsByNameRequest) (*accountspb.GetSimilarRecipientsByNameResponse, error) {
	return p.client.GetSimilarRecipientsByName(forwardContext(ctx), req)
}

func (p *RecipientServiceProxy) SearchRecipientsByAccount(ctx context.Context, req *accountspb.SearchRecipientsByAccountRequest) (*accountspb.SearchRecipientsByAccountResponse, error) {
	return p.client.SearchRecipientsByAccount(forwardContext(ctx), req)
}

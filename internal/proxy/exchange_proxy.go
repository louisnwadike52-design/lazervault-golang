package proxy

import (
	"context"

	pb "lazervaultGo/pb"
)

// ExchangeServiceProxy proxies ExchangeService gRPC requests to exchange-service
type ExchangeServiceProxy struct {
	pb.UnimplementedExchangeServiceServer
	client pb.ExchangeServiceClient
}

// NewExchangeServiceProxy creates a new ExchangeServiceProxy
func NewExchangeServiceProxy(client pb.ExchangeServiceClient) *ExchangeServiceProxy {
	return &ExchangeServiceProxy{
		client: client,
	}
}

func (p *ExchangeServiceProxy) GetExchangeRate(ctx context.Context, req *pb.GetExchangeRateRequest) (*pb.GetExchangeRateResponse, error) {
	return p.client.GetExchangeRate(forwardContext(ctx), req)
}

func (p *ExchangeServiceProxy) InitiateInternationalTransfer(ctx context.Context, req *pb.InitiateInternationalTransferRequest) (*pb.InitiateInternationalTransferResponse, error) {
	return p.client.InitiateInternationalTransfer(forwardContext(ctx), req)
}

func (p *ExchangeServiceProxy) GetRecentExchanges(ctx context.Context, req *pb.GetRecentExchangesRequest) (*pb.GetRecentExchangesResponse, error) {
	return p.client.GetRecentExchanges(forwardContext(ctx), req)
}

func (p *ExchangeServiceProxy) ConvertCurrency(ctx context.Context, req *pb.ConvertCurrencyRequest) (*pb.ConvertCurrencyResponse, error) {
	return p.client.ConvertCurrency(forwardContext(ctx), req)
}

func (p *ExchangeServiceProxy) GetTransactionStatus(ctx context.Context, req *pb.GetTransactionStatusRequest) (*pb.GetTransactionStatusResponse, error) {
	return p.client.GetTransactionStatus(forwardContext(ctx), req)
}

func (p *ExchangeServiceProxy) GetSupportedCurrencies(ctx context.Context, req *pb.GetSupportedCurrenciesRequest) (*pb.GetSupportedCurrenciesResponse, error) {
	return p.client.GetSupportedCurrencies(forwardContext(ctx), req)
}

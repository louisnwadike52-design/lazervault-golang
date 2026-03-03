package proxy

import (
	"context"

	pb "lazervaultGo/pb"
)

// TransactionPinServiceProxy proxies TransactionPinService gRPC requests to the upstream auth microservice
type TransactionPinServiceProxy struct {
	pb.UnimplementedTransactionPinServiceServer
	client pb.TransactionPinServiceClient
}

// NewTransactionPinServiceProxy creates a new TransactionPinServiceProxy
func NewTransactionPinServiceProxy(client pb.TransactionPinServiceClient) *TransactionPinServiceProxy {
	return &TransactionPinServiceProxy{
		client: client,
	}
}

// CreateTransactionPin creates a new transaction PIN for a user
func (p *TransactionPinServiceProxy) CreateTransactionPin(ctx context.Context, req *pb.CreateTransactionPinRequest) (*pb.CreateTransactionPinResponse, error) {
	return p.client.CreateTransactionPin(forwardContext(ctx), req)
}

// VerifyTransactionPin verifies a PIN before processing a payment
func (p *TransactionPinServiceProxy) VerifyTransactionPin(ctx context.Context, req *pb.VerifyTransactionPinRequest) (*pb.VerifyTransactionPinResponse, error) {
	return p.client.VerifyTransactionPin(forwardContext(ctx), req)
}

// ChangeTransactionPin changes an existing PIN
func (p *TransactionPinServiceProxy) ChangeTransactionPin(ctx context.Context, req *pb.ChangeTransactionPinRequest) (*pb.ChangeTransactionPinResponse, error) {
	return p.client.ChangeTransactionPin(forwardContext(ctx), req)
}

// ResetTransactionPin resets a PIN with proper verification
func (p *TransactionPinServiceProxy) ResetTransactionPin(ctx context.Context, req *pb.ResetTransactionPinRequest) (*pb.ResetTransactionPinResponse, error) {
	return p.client.ResetTransactionPin(forwardContext(ctx), req)
}

// CheckUserHasPin checks if a user has a transaction PIN set up
func (p *TransactionPinServiceProxy) CheckUserHasPin(ctx context.Context, req *pb.CheckUserHasPinRequest) (*pb.CheckUserHasPinResponse, error) {
	return p.client.CheckUserHasPin(forwardContext(ctx), req)
}

// ValidateTransactionPinToken validates a token issued after PIN verification
func (p *TransactionPinServiceProxy) ValidateTransactionPinToken(ctx context.Context, req *pb.ValidateTransactionPinTokenRequest) (*pb.ValidateTransactionPinTokenResponse, error) {
	return p.client.ValidateTransactionPinToken(forwardContext(ctx), req)
}

// InitiatePinOTP sends a 6-digit OTP code via email or SMS for PIN operations
func (p *TransactionPinServiceProxy) InitiatePinOTP(ctx context.Context, req *pb.InitiatePinOTPRequest) (*pb.InitiatePinOTPResponse, error) {
	return p.client.InitiatePinOTP(forwardContext(ctx), req)
}

// VerifyPinOTP verifies the OTP and executes the PIN operation
func (p *TransactionPinServiceProxy) VerifyPinOTP(ctx context.Context, req *pb.VerifyPinOTPRequest) (*pb.VerifyPinOTPResponse, error) {
	return p.client.VerifyPinOTP(forwardContext(ctx), req)
}

// GetPinOTPChannels returns available OTP delivery channels for the user
func (p *TransactionPinServiceProxy) GetPinOTPChannels(ctx context.Context, req *pb.GetPinOTPChannelsRequest) (*pb.GetPinOTPChannelsResponse, error) {
	return p.client.GetPinOTPChannels(forwardContext(ctx), req)
}

// CompleteForgotPin verifies OTP and resets the PIN
func (p *TransactionPinServiceProxy) CompleteForgotPin(ctx context.Context, req *pb.CompleteForgotPinRequest) (*pb.CompleteForgotPinResponse, error) {
	return p.client.CompleteForgotPin(forwardContext(ctx), req)
}

// GetUserChannelPins returns PIN setup status for all channels
func (p *TransactionPinServiceProxy) GetUserChannelPins(ctx context.Context, req *pb.GetUserChannelPinsRequest) (*pb.GetUserChannelPinsResponse, error) {
	return p.client.GetUserChannelPins(forwardContext(ctx), req)
}

// CreateChannelRegistration registers a user for a banking channel
func (p *TransactionPinServiceProxy) CreateChannelRegistration(ctx context.Context, req *pb.CreateChannelRegistrationRequest) (*pb.CreateChannelRegistrationResponse, error) {
	return p.client.CreateChannelRegistration(forwardContext(ctx), req)
}

// VerifyChannelOTP verifies OTP for channel registration
func (p *TransactionPinServiceProxy) VerifyChannelOTP(ctx context.Context, req *pb.VerifyChannelOTPRequest) (*pb.VerifyChannelOTPResponse, error) {
	return p.client.VerifyChannelOTP(forwardContext(ctx), req)
}

// GetChannelRegistrations gets all channel registrations for a user
func (p *TransactionPinServiceProxy) GetChannelRegistrations(ctx context.Context, req *pb.GetChannelRegistrationsRequest) (*pb.GetChannelRegistrationsResponse, error) {
	return p.client.GetChannelRegistrations(forwardContext(ctx), req)
}

// DeactivateChannel deactivates a banking channel
func (p *TransactionPinServiceProxy) DeactivateChannel(ctx context.Context, req *pb.DeactivateChannelRequest) (*pb.DeactivateChannelResponse, error) {
	return p.client.DeactivateChannel(forwardContext(ctx), req)
}

// ResolvePhoneToUser resolves a phone number to a user ID (service-to-service)
func (p *TransactionPinServiceProxy) ResolvePhoneToUser(ctx context.Context, req *pb.ResolvePhoneToUserRequest) (*pb.ResolvePhoneToUserResponse, error) {
	return p.client.ResolvePhoneToUser(forwardContext(ctx), req)
}

package proxy

import (
	"context"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	pb "lazervaultGo/pb"
	"google.golang.org/grpc/metadata"
)

// AuthServiceProxy proxies AuthService gRPC requests to the upstream auth microservice
type AuthServiceProxy struct {
	pb.UnimplementedAuthServiceServer
	client pb.AuthServiceClient
}

// NewAuthServiceProxy creates a new AuthServiceProxy
func NewAuthServiceProxy(client pb.AuthServiceClient) *AuthServiceProxy {
	return &AuthServiceProxy{
		client: client,
	}
}

// forwardContext propagates metadata from incoming context to outgoing context
func forwardContext(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(map[string]string{})
	}

	// Forward important headers
	outgoingMD := metadata.New(map[string]string{})

	if auth := md.Get("authorization"); len(auth) > 0 {
		outgoingMD.Set("authorization", auth[0])
	}
	if reqID := md.Get("x-request-id"); len(reqID) > 0 {
		outgoingMD.Set("x-request-id", reqID[0])
	}
	if locale := md.Get("x-locale"); len(locale) > 0 {
		outgoingMD.Set("x-locale", locale[0])
	}
	if accountID := md.Get("x-account-id"); len(accountID) > 0 {
		outgoingMD.Set("x-account-id", accountID[0])
	}
	if country := md.Get("x-user-country"); len(country) > 0 {
		outgoingMD.Set("x-user-country", country[0])
	}
	if currency := md.Get("x-currency"); len(currency) > 0 {
		outgoingMD.Set("x-currency", currency[0])
	}

	// Extract user_id from auth context and add to metadata
	userID, err := authinterceptor.GetUserID(ctx)
	if err == nil && userID != "" {
		outgoingMD.Set("x-user-id", userID)
	}

	// Add service name for tracing
	outgoingMD.Set("x-service-name", "core-gateway")

	return metadata.NewOutgoingContext(ctx, outgoingMD)
}

// Implement critical AuthService methods for signup flow

func (p *AuthServiceProxy) Signup(ctx context.Context, req *pb.SignupRequest) (*pb.SignupResponse, error) {
	return p.client.Signup(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	return p.client.Login(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	return p.client.RefreshToken(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	return p.client.Logout(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) VerifyEmail(ctx context.Context, req *pb.VerifyEmailRequest) (*pb.VerifyEmailResponse, error) {
	return p.client.VerifyEmail(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ForgotPassword(ctx context.Context, req *pb.ForgotPasswordRequest) (*pb.ForgotPasswordResponse, error) {
	return p.client.ForgotPassword(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) (*pb.ResetPasswordResponse, error) {
	return p.client.ResetPassword(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) GetMe(ctx context.Context, req *pb.GetMeRequest) (*pb.GetMeResponse, error) {
	return p.client.GetMe(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {
	return p.client.ChangePassword(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	return p.client.ValidateToken(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ResendVerificationEmail(ctx context.Context, req *pb.ResendVerificationEmailRequest) (*pb.ResendVerificationEmailResponse, error) {
	return p.client.ResendVerificationEmail(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) CheckEmailAvailability(ctx context.Context, req *pb.CheckEmailAvailabilityRequest) (*pb.CheckEmailAvailabilityResponse, error) {
	return p.client.CheckEmailAvailability(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) RegisterPasscode(ctx context.Context, req *pb.RegisterPasscodeRequest) (*pb.RegisterPasscodeResponse, error) {
	return p.client.RegisterPasscode(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) LoginWithPasscode(ctx context.Context, req *pb.LoginWithPasscodeRequest) (*pb.LoginResponse, error) {
	return p.client.LoginWithPasscode(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ChangePasscode(ctx context.Context, req *pb.ChangePasscodeRequest) (*pb.ChangePasscodeResponse, error) {
	return p.client.ChangePasscode(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) RequestEmailVerification(ctx context.Context, req *pb.RequestEmailVerificationRequest) (*pb.RequestEmailVerificationResponse, error) {
	return p.client.RequestEmailVerification(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) RequestPasswordReset(ctx context.Context, req *pb.RequestPasswordResetRequest) (*pb.RequestPasswordResetResponse, error) {
	return p.client.RequestPasswordReset(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) RequestPhoneVerification(ctx context.Context, req *pb.RequestPhoneVerificationRequest) (*pb.RequestPhoneVerificationResponse, error) {
	return p.client.RequestPhoneVerification(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) VerifyPhoneNumber(ctx context.Context, req *pb.VerifyPhoneNumberRequest) (*pb.VerifyPhoneNumberResponse, error) {
	return p.client.VerifyPhoneNumber(forwardContext(ctx), req)
}

// VerifyIdentity verifies user identity using BVN or NIN
func (p *AuthServiceProxy) VerifyIdentity(ctx context.Context, req *pb.VerifyIdentityRequest) (*pb.VerifyIdentityResponse, error) {
	return p.client.VerifyIdentity(forwardContext(ctx), req)
}

// GetIdentityVerificationStatus returns the identity verification status
func (p *AuthServiceProxy) GetIdentityVerificationStatus(ctx context.Context, req *pb.GetIdentityVerificationStatusRequest) (*pb.GetIdentityVerificationStatusResponse, error) {
	return p.client.GetIdentityVerificationStatus(forwardContext(ctx), req)
}

// SearchUsers searches for users by name, username, email, or phone
func (p *AuthServiceProxy) SearchUsers(ctx context.Context, req *pb.UserSearchRequest) (*pb.UserSearchResponse, error) {
	return p.client.SearchUsers(forwardContext(ctx), req)
}

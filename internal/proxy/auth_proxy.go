package proxy

import (
	"context"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	"lazervaultGo/internal/interceptors"
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

	// Forward the App Check (device attestation) verdict so auth-service risk
	// scoring can use it. Only set when a token was actually present; absence is
	// left unset so report-only App Check doesn't penalise every login.
	if verdict, ok := interceptors.AppCheckVerdictFromContext(ctx); ok && verdict.Present {
		if verdict.Valid {
			outgoingMD.Set("x-appcheck-verdict", "valid")
		} else {
			outgoingMD.Set("x-appcheck-verdict", "invalid")
		}
	}

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

// VerifyPasswordResetCode must be forwarded (and is public in the interceptor):
// the user is logged out during password reset, so the native-gRPC client path
// (app → core-gateway gRPC → auth-service) hits it without a JWT. Without this
// forwarder the embedded UnimplementedAuthServiceServer answers Unimplemented
// and the app surfaced a misleading "Session expired".
func (p *AuthServiceProxy) VerifyPasswordResetCode(ctx context.Context, req *pb.VerifyPasswordResetCodeRequest) (*pb.VerifyPasswordResetCodeResponse, error) {
	return p.client.VerifyPasswordResetCode(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) (*pb.ResetPasswordResponse, error) {
	return p.client.ResetPassword(forwardContext(ctx), req)
}

// Configurable auth mode: phone + passcode flow. Forwarded to auth-service so
// the native-gRPC client path (app → core-gateway gRPC → auth-service) reaches
// these new methods just like the HTTP grpc-gateway path does. Without these,
// the embedded UnimplementedAuthServiceServer answers Unimplemented and the app
// sees a generic "network error".
func (p *AuthServiceProxy) GetAuthenticationConfig(ctx context.Context, req *pb.GetAuthenticationConfigRequest) (*pb.GetAuthenticationConfigResponse, error) {
	return p.client.GetAuthenticationConfig(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) RequestSignupPhoneOTP(ctx context.Context, req *pb.RequestSignupPhoneOTPRequest) (*pb.RequestSignupPhoneOTPResponse, error) {
	return p.client.RequestSignupPhoneOTP(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) VerifySignupPhoneOTP(ctx context.Context, req *pb.VerifySignupPhoneOTPRequest) (*pb.VerifySignupPhoneOTPResponse, error) {
	return p.client.VerifySignupPhoneOTP(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) SignupWithPhone(ctx context.Context, req *pb.SignupWithPhoneRequest) (*pb.LoginResponse, error) {
	return p.client.SignupWithPhone(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) LoginWithPhonePasscode(ctx context.Context, req *pb.LoginWithPhonePasscodeRequest) (*pb.LoginResponse, error) {
	return p.client.LoginWithPhonePasscode(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) RequestPasscodeReset(ctx context.Context, req *pb.RequestPasscodeResetRequest) (*pb.RequestPasscodeResetResponse, error) {
	return p.client.RequestPasscodeReset(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ResetPasscodeWithOTP(ctx context.Context, req *pb.ResetPasscodeWithOTPRequest) (*pb.ResetPasscodeWithOTPResponse, error) {
	return p.client.ResetPasscodeWithOTP(forwardContext(ctx), req)
}

// VerifyPasscodeResetOTP validates the reset code without consuming it (public;
// user is logged out). Forwarded for the native-gRPC client path.
func (p *AuthServiceProxy) VerifyPasscodeResetOTP(ctx context.Context, req *pb.VerifyPasscodeResetOTPRequest) (*pb.VerifyPasscodeResetOTPResponse, error) {
	return p.client.VerifyPasscodeResetOTP(forwardContext(ctx), req)
}

// SetPreferredLoginMethod / SetPassword are authenticated; forwardContext carries
// the caller's JWT metadata to auth-service, which derives identity from it.
func (p *AuthServiceProxy) SetPreferredLoginMethod(ctx context.Context, req *pb.SetPreferredLoginMethodRequest) (*pb.SetPreferredLoginMethodResponse, error) {
	return p.client.SetPreferredLoginMethod(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) SetPassword(ctx context.Context, req *pb.SetPasswordRequest) (*pb.SetPasswordResponse, error) {
	return p.client.SetPassword(forwardContext(ctx), req)
}

// RequestPhoneChange / VerifyPhoneChange — authenticated new-phone flow. Identity
// is forwarded as x-user-id by forwardContext (JWT-derived).
func (p *AuthServiceProxy) RequestPhoneChange(ctx context.Context, req *pb.RequestPhoneChangeRequest) (*pb.RequestPhoneChangeResponse, error) {
	return p.client.RequestPhoneChange(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) VerifyPhoneChange(ctx context.Context, req *pb.VerifyPhoneChangeRequest) (*pb.VerifyPhoneChangeResponse, error) {
	return p.client.VerifyPhoneChange(forwardContext(ctx), req)
}

// VerifyLoginOtp completes an adaptive step-up login (public passthrough).
func (p *AuthServiceProxy) VerifyLoginOtp(ctx context.Context, req *pb.VerifyLoginOtpRequest) (*pb.LoginResponse, error) {
	return p.client.VerifyLoginOtp(forwardContext(ctx), req)
}

// ===== Device registry (trusted devices / security center) — authed passthrough =====
func (p *AuthServiceProxy) RegisterDevice(ctx context.Context, req *pb.RegisterDeviceRequest) (*pb.RegisterDeviceResponse, error) {
	return p.client.RegisterDevice(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ListDevices(ctx context.Context, req *pb.ListDevicesRequest) (*pb.ListDevicesResponse, error) {
	return p.client.ListDevices(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) RevokeDevice(ctx context.Context, req *pb.RevokeDeviceRequest) (*pb.RevokeDeviceResponse, error) {
	return p.client.RevokeDevice(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) GetLoginHistory(ctx context.Context, req *pb.GetLoginHistoryRequest) (*pb.GetLoginHistoryResponse, error) {
	return p.client.GetLoginHistory(forwardContext(ctx), req)
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

// Legacy phone-verification RPCs (kept proxied for older onboarding callers).
func (p *AuthServiceProxy) VerifyPhone(ctx context.Context, req *pb.VerifyPhoneRequest) (*pb.VerifyPhoneResponse, error) {
	return p.client.VerifyPhone(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) ResendPhoneVerification(ctx context.Context, req *pb.ResendPhoneVerificationRequest) (*pb.ResendPhoneVerificationResponse, error) {
	return p.client.ResendPhoneVerification(forwardContext(ctx), req)
}

// ── Two-Factor Authentication (TOTP / SMS / email) ──
// These were previously unimplemented on the gateway, so Flutter's 2FA calls
// fell through to UNIMPLEMENTED. Forward them to auth-service like every other
// AuthService RPC (user identity travels in the forwarded JWT metadata).
func (p *AuthServiceProxy) EnableTwoFactor(ctx context.Context, req *pb.EnableTwoFactorRequest) (*pb.EnableTwoFactorResponse, error) {
	return p.client.EnableTwoFactor(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) VerifyTwoFactor(ctx context.Context, req *pb.VerifyTwoFactorRequest) (*pb.VerifyTwoFactorResponse, error) {
	return p.client.VerifyTwoFactor(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) CompleteTwoFactorSetup(ctx context.Context, req *pb.CompleteTwoFactorSetupRequest) (*pb.CompleteTwoFactorSetupResponse, error) {
	return p.client.CompleteTwoFactorSetup(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) DisableTwoFactor(ctx context.Context, req *pb.DisableTwoFactorRequest) (*pb.DisableTwoFactorResponse, error) {
	return p.client.DisableTwoFactor(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) GetTwoFactorStatus(ctx context.Context, req *pb.GetTwoFactorStatusRequest) (*pb.GetTwoFactorStatusResponse, error) {
	return p.client.GetTwoFactorStatus(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) RegenerateBackupCodes(ctx context.Context, req *pb.RegenerateBackupCodesRequest) (*pb.RegenerateBackupCodesResponse, error) {
	return p.client.RegenerateBackupCodes(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) SendTwoFactorCode(ctx context.Context, req *pb.SendTwoFactorCodeRequest) (*pb.SendTwoFactorCodeResponse, error) {
	return p.client.SendTwoFactorCode(forwardContext(ctx), req)
}

func (p *AuthServiceProxy) GetAvailable2FAMethods(ctx context.Context, req *pb.GetAvailable2FAMethodsRequest) (*pb.GetAvailable2FAMethodsResponse, error) {
	return p.client.GetAvailable2FAMethods(forwardContext(ctx), req)
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

// RequestAccountDeletion schedules the caller's account for deletion (30-day
// cancellable grace). Without this forwarder the embedded UnimplementedAuthServiceServer
// answered UNIMPLEMENTED, so the app's "Delete my account" always failed.
func (p *AuthServiceProxy) RequestAccountDeletion(ctx context.Context, req *pb.RequestAccountDeletionRequest) (*pb.RequestAccountDeletionResponse, error) {
	return p.client.RequestAccountDeletion(forwardContext(ctx), req)
}

// CancelAccountDeletion cancels a pending deletion within the grace window
// (also invoked implicitly when a user signs back in).
func (p *AuthServiceProxy) CancelAccountDeletion(ctx context.Context, req *pb.CancelAccountDeletionRequest) (*pb.CancelAccountDeletionResponse, error) {
	return p.client.CancelAccountDeletion(forwardContext(ctx), req)
}

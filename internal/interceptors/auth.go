package interceptors

import (
	"context"
	"strings"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// JWTAuthInterceptor creates a gRPC interceptor for JWT authentication
func JWTAuthInterceptor(verifier *authinterceptor.JWTVerifier) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Check if method requires authentication
		if isPublicEndpoint(info.FullMethod) {
			return handler(ctx, req)
		}

		// Extract authorization metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "metadata not provided")
		}

		authHeader := md.Get("authorization")
		if len(authHeader) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization token not provided")
		}

		// Parse "Bearer <token>"
		fields := strings.Fields(authHeader[0])
		if len(fields) < 2 || strings.ToLower(fields[0]) != "bearer" {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization format")
		}

		token := fields[1]

		// Verify JWT using JWKS
		payload, err := verifier.VerifyToken(ctx, token)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		// Add user info to context
		ctx = authinterceptor.SetAuthPayload(ctx, payload)
		ctx = authinterceptor.SetUserID(ctx, payload.UserID)

		return handler(ctx, req)
	}
}

// isPublicEndpoint determines if a gRPC method is public (doesn't require authentication)
func isPublicEndpoint(method string) bool {
	publicEndpoints := map[string]bool{
		// Auth Service public endpoints
		"/pb.AuthService/Login":                   true,
		"/pb.AuthService/Signup":                  true,
		"/pb.AuthService/LoginWithPasscode":       true, // Public - login with passcode
		"/pb.AuthService/VerifyLoginOtp":          true, // Public - completes an adaptive step-up login
		"/pb.AuthService/RefreshToken":            true,
		"/pb.AuthService/VerifyEmail":             true,
		"/pb.AuthService/ForgotPassword":          true,
		"/pb.AuthService/VerifyPasswordResetCode": true, // Public - verify reset code/OTP (user is logged out)
		"/pb.AuthService/ResetPassword":           true,
		"/pb.AuthService/ResendVerificationEmail": true,
		"/pb.AuthService/CheckEmailAvailability":  true,
		// Configurable auth mode: phone + passcode flow (public — registration/login)
		"/pb.AuthService/GetAuthenticationConfig": true, // Public - read active auth mode
		"/pb.AuthService/RequestSignupPhoneOTP":   true, // Public - send signup phone OTP
		"/pb.AuthService/VerifySignupPhoneOTP":    true, // Public - verify signup phone OTP
		"/pb.AuthService/SignupWithPhone":         true, // Public - phone+passcode signup
		"/pb.AuthService/LoginWithPhonePasscode":  true, // Public - phone+passcode login
		"/pb.AuthService/RequestPasscodeReset":    true, // Public - forgot passcode (phone OTP)
		"/pb.AuthService/VerifyPasscodeResetOTP":  true, // Public - validate reset OTP (logged out)
		"/pb.AuthService/ResetPasscodeWithOTP":    true, // Public - set new passcode via OTP

		// Crypto Service public endpoints (if needed)
		"/crypto.CryptoService/GetCryptos":     true,
		"/crypto.CryptoService/GetCryptoPrice": true,

		// Gift Card Service public endpoints
		"/giftcard.GiftCardService/GetBrands":     true,
		"/giftcard.GiftCardService/SearchBrands":  true,
		"/giftcard.GiftCardService/GetCategories": true,

		// Stock Service public endpoints
		"/stock.StockService/GetStockData":  true,
		"/stock.StockService/SearchStocks":  true,
		"/stock.StockService/GetMarketData": true,
	}

	return publicEndpoints[method]
}

package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	authpb "auth-service/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Define a custom type for the context key to avoid collisions
type contextKey string

// Exported context key for authorization payload
const AuthorizationPayloadKey contextKey = "authorization_payload"
const AccessTokenKey contextKey = "accessToken" // New key for the raw access token string

const (
	authorizationHeader = "authorization"
	authorizationBearer = "bearer"
)

// AuthPayload represents the validated token payload
type AuthPayload struct {
	UserID    string
	Email     string
	ExpiresAt int64
}

var (
	authServiceClient authpb.AuthServiceClient
	authServiceAddr   string
)

// InitAuthServiceClient initializes the auth service gRPC client
func InitAuthServiceClient(addr string) error {
	authServiceAddr = addr
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to auth service at %s: %w", addr, err)
	}
	authServiceClient = authpb.NewAuthServiceClient(conn)
	return nil
}

// GetAuthServiceClient returns the initialized auth service client
func GetAuthServiceClient() authpb.AuthServiceClient {
	return authServiceClient
}

// AuthInterceptor creates a new unary interceptor for authentication
// This version calls auth-service's ValidateToken RPC instead of validating locally
func AuthInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if !requiresAuth(info.FullMethod) {
			return handler(ctx, req)
		}

		// authorize now calls auth-service's ValidateToken RPC
		payload, rawToken, err := authorize(ctx)
		if err != nil {
			return nil, err
		}

		// Add user info to context using the exported key
		ctx = context.WithValue(ctx, AuthorizationPayloadKey, payload)
		// Store the raw access token string in the context
		if rawToken != "" {
			ctx = context.WithValue(ctx, AccessTokenKey, rawToken)
		}
		return handler(ctx, req)
	}
}

// requiresAuth determines if the given method requires authentication
func requiresAuth(method string) bool {
	// List of public endpoints that don't require authentication
	publicEndpoints := map[string]bool{
		// Auth Service public endpoints
		"/pb.AuthService/Login":                  false,
		"/pb.AuthService/LoginWithPasscode":      false, // Public - login with passcode
		"/pb.AuthService/Logout":                 true,  // Should be true, logout needs auth
		"/pb.AuthService/RefreshToken":           false,
		"/pb.AuthService/Register":               false,
		"/pb.AuthService/VerifyEmail":            false,
		"/pb.AuthService/ResendEmail":            false,
		"/pb.AuthService/CheckEmailAvailability": false, // Public - check before signup
		"/pb.AuthService/RequestPasswordReset":   false, // Public - initiate password reset
		"/pb.AuthService/ResetPassword":          false, // Public - reset with token

		// User Service public endpoints
		"/pb.UserService/CreateUser": false,

		// Crypto Service public endpoints (all crypto data is public)
		"/pb.CryptoService/GetCryptos":            false,
		"/pb.CryptoService/GetCryptoById":         false,
		"/pb.CryptoService/SearchCryptos":         false,
		"/pb.CryptoService/GetCryptoPriceHistory": false,
		"/pb.CryptoService/GetTrendingCryptos":    false,
		"/pb.CryptoService/GetTopCryptos":         false,
		"/pb.CryptoService/GetMarketChart":        false,
		"/pb.CryptoService/GetGlobalMarketData":   false,

		// Gift Card Service public endpoints (browsing/searching)
		"/pb.GiftCardService/GetGiftCardBrands":           false,
		"/pb.GiftCardService/GetGiftCardBrandsByCategory": false,
		"/pb.GiftCardService/SearchGiftCardBrands":        false,
		"/pb.GiftCardService/GetGiftCardBrandById":        false,
		"/pb.GiftCardService/GetPopularBrands":            false,

		// Stock Service public endpoints (market data)
		"/pb.StockService/GetStocks":            false,
		"/pb.StockService/GetStockBySymbol":     false,
		"/pb.StockService/SearchStocks":         false,
		"/pb.StockService/GetStockPriceHistory": false,
		"/pb.StockService/GetMarketIndices":     false,
		"/pb.StockService/GetTrendingStocks":    false,
		"/pb.StockService/GetTopGainers":        false,
		"/pb.StockService/GetTopLosers":         false,

		// Electricity Bill Service public endpoints (for testing)
		"/pb.ElectricityBillService/GetProviders":        false,
		"/pb.ElectricityBillService/SyncProviders":       false,
		"/pb.ElectricityBillService/ValidateMeterNumber": false,

		// Add more public endpoints as needed
	}

	// Check if the method is in our public endpoints map
	if requireAuth, exists := publicEndpoints[method]; exists {
		return requireAuth
	}

	// By default, require authentication for all other endpoints
	return true
}

// authorize verifies the authentication token by calling auth-service's ValidateToken RPC
func authorize(ctx context.Context) (*AuthPayload, string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, "", status.Error(codes.Unauthenticated, "metadata is not provided")
	}

	values := md.Get(authorizationHeader)
	if len(values) == 0 {
		return nil, "", status.Error(codes.Unauthenticated, "authorization token is not provided")
	}

	authHeader := values[0]
	fields := strings.Fields(authHeader)
	if len(fields) < 2 {
		return nil, "", status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	authType := strings.ToLower(fields[0])
	if authType != authorizationBearer {
		return nil, "", status.Error(codes.Unauthenticated, "unsupported authorization type")
	}

	accessTokenString := fields[1] // This is the raw token string

	// Call auth-service's ValidateToken RPC
	validateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := authServiceClient.ValidateToken(validateCtx, &authpb.ValidateTokenRequest{
		Token: accessTokenString,
	})
	if err != nil {
		return nil, "", status.Errorf(codes.Unauthenticated, "failed to validate token: %v", err)
	}

	if !resp.Valid {
		return nil, "", status.Error(codes.Unauthenticated, "invalid token")
	}

	// Create payload from auth-service response
	payload := &AuthPayload{
		UserID:    resp.UserId,
		Email:     resp.Email,
		ExpiresAt: resp.ExpiresAt,
	}

	return payload, accessTokenString, nil
}

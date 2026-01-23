package middleware

import (
	"context"
	"strings"
	"time"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	"github.com/gin-gonic/gin"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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

var jwtVerifier *authinterceptor.JWTVerifier

// InitJWTVerifier initializes the JWT verifier with JWKS
func InitJWTVerifier(jwksURL, issuer, audience string, logger *zap.Logger) error {
	var err error
	jwtVerifier, err = authinterceptor.NewJWTVerifier(authinterceptor.JWTVerifierConfig{
		JWKSURL:       jwksURL,
		Issuer:        issuer,
		Audience:      audience,
		CacheDuration: 5 * time.Minute,
		Logger:        logger,
	})
	return err
}

// GetJWTVerifier returns the initialized JWT verifier
func GetJWTVerifier() *authinterceptor.JWTVerifier {
	return jwtVerifier
}

// AuthInterceptor creates a new unary interceptor for authentication
// This version uses banking-grade JWT verification with local cryptographic validation
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

// authorize verifies JWT token using local cryptographic verification
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

	accessToken := fields[1]

	// Verify JWT using banking-grade cryptographic verification (NO RPC call)
	payload, err := jwtVerifier.VerifyToken(ctx, accessToken)
	if err != nil {
		return nil, "", status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}

	// Convert to gateway's AuthPayload format
	authPayload := &AuthPayload{
		UserID:    payload.UserID,
		Email:     payload.Email,
		ExpiresAt: payload.ExpiresAt,
	}

	return authPayload, accessToken, nil
}

// JWTAuthMiddleware validates JWT tokens for protected endpoints
// This middleware is used for HTTP API routes (grpc-gateway path)
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip auth for public endpoints
		if isPublicHTTPPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Get Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(401, gin.H{"error": "authorization token not provided"})
			c.Abort()
			return
		}

		// Parse "Bearer <token>"
		fields := strings.Fields(authHeader)
		if len(fields) < 2 || strings.ToLower(fields[0]) != "bearer" {
			c.JSON(401, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		token := fields[1]

		// Verify JWT using JWKS
		payload, err := jwtVerifier.VerifyToken(c.Request.Context(), token)
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Set user ID in context for downstream use
		c.Set("user_id", payload.UserID)
		c.Set("auth_payload", payload)

		// Also set x-user-id header for grpc-gateway to pass to microservices
		c.Request.Header.Set("x-user-id", payload.UserID)

		c.Next()
	}
}

// isPublicHTTPPath determines if an HTTP path is public (doesn't require authentication)
func isPublicHTTPPath(path string) bool {
	publicPaths := []string{
		"/api/v1/auth/login",
		"/api/v1/auth/signup",
		"/api/v1/auth/login-passcode",
		"/api/v1/auth/refresh",
		"/api/v1/auth/verify-email",
		"/api/v1/auth/forgot-password",
		"/api/v1/auth/reset-password",
		"/api/v1/auth/resend-verification",
		"/api/v1/auth/check-email-availability",
		"/api/v1/auth/request-email-verification",
		"/api/v1/auth/request-phone-verification",
		"/.well-known/jwks.json",
		"/health",
		"/ready",
		"/metrics",
	}

	for _, public := range publicPaths {
		if strings.HasPrefix(path, public) {
			return true
		}
	}
	return false
}

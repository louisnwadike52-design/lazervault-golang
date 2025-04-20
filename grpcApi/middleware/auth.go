package middleware

import (
	"context"
	"lazervaultGo/token"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Define a custom type for the context key to avoid collisions
type contextKey string

// Exported context key for authorization payload
const AuthorizationPayloadKey contextKey = "authorization_payload"

const (
	authorizationHeader = "authorization"
	authorizationBearer = "bearer"
)

// AuthInterceptor creates a new unary interceptor for authentication
func AuthInterceptor(tokenMaker token.Maker) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if !requiresAuth(info.FullMethod) {
			return handler(ctx, req)
		}

		payload, err := authorize(ctx, tokenMaker)
		if err != nil {
			return nil, err
		}

		// Add user info to context using the exported key
		ctx = context.WithValue(ctx, AuthorizationPayloadKey, payload)
		return handler(ctx, req)
	}
}

// requiresAuth determines if the given method requires authentication
func requiresAuth(method string) bool {
	// List of public endpoints that don't require authentication
	publicEndpoints := map[string]bool{
		// Auth Service public endpoints
		"/pb.AuthService/Login":        false,
		"/pb.AuthService/Logout":       true,
		"/pb.AuthService/RefreshToken": false,
		"/pb.AuthService/Register":     false,
		"/pb.AuthService/VerifyEmail":  false,
		"/pb.AuthService/ResendEmail":  false,

		// User Service public endpoints
		"/pb.UserService/CreateUser": false,

		// Add more public endpoints as needed
	}

	// Check if the method is in our public endpoints map
	if requireAuth, exists := publicEndpoints[method]; exists {
		return requireAuth
	}

	// By default, require authentication for all other endpoints
	return true
}

// authorize verifies the authentication token from the context
func authorize(ctx context.Context, tokenMaker token.Maker) (*token.Payload, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
	}

	values := md.Get(authorizationHeader)
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
	}

	authHeader := values[0]
	fields := strings.Fields(authHeader)
	if len(fields) < 2 {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	authType := strings.ToLower(fields[0])
	if authType != authorizationBearer {
		return nil, status.Error(codes.Unauthenticated, "unsupported authorization type")
	}

	accessToken := fields[1]
	payload, err := tokenMaker.VerifyToken(accessToken)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}

	return payload, nil
}

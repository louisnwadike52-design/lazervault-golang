package interceptors

import (
	"context"
	"fmt"
	"time"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RateLimitInterceptor creates a gRPC interceptor for rate limiting
func RateLimitInterceptor(redisClient *redis.Client) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if redisClient == nil {
			return handler(ctx, req)
		}

		// Get user ID from context (set by auth interceptor)
		userID := getUserIDFromContext(ctx)
		if userID == "" {
			// No auth, use IP-based rate limiting
			md, _ := metadata.FromIncomingContext(ctx)
			peer := md.Get("x-forwarded-for")
			if len(peer) > 0 {
				userID = peer[0]
			} else {
				userID = "anonymous"
			}
		}

		// Rate limit: 100 requests per minute per user
		key := fmt.Sprintf("ratelimit:grpc:%s", userID)
		count, err := redisClient.Incr(ctx, key).Result()
		if err == nil && count == 1 {
			redisClient.Expire(ctx, key, time.Minute)
		}

		if count > 100 {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}

		return handler(ctx, req)
	}
}

func getUserIDFromContext(ctx context.Context) string {
	// Extract from auth payload set by JWT interceptor
	payload, err := authinterceptor.GetAuthPayload(ctx)
	if err == nil && payload != nil {
		return payload.UserID
	}
	return ""
}

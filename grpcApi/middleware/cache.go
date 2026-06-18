package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// CacheMiddleware creates a Redis-based caching middleware for GET requests
func CacheMiddleware(redisClient *redis.Client, defaultTTL time.Duration) gin.HandlerFunc {
	if redisClient == nil {
		// If Redis is not available, return a no-op middleware
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		// Only cache GET requests
		if c.Request.Method != "GET" {
			c.Next()
			return
		}

		// Skip caching for authenticated endpoints or specific routes
		if shouldSkipCache(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Generate cache key
		cacheKey := generateCacheKey(c)

		// Try to get from cache
		ctx := context.Background()
		cachedData, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil {
			// Cache hit - return cached response
			var responseData interface{}
			if err := json.Unmarshal([]byte(cachedData), &responseData); err == nil {
				c.Header("X-Cache", "HIT")
				c.JSON(http.StatusOK, responseData)
				c.Abort()
				return
			}
		}

		// Cache miss - proceed with request
		c.Header("X-Cache", "MISS")

		// Use a custom writer to capture response
		w := &responseWriter{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = w

		c.Next()

		// Cache the response if successful
		if w.status == http.StatusOK && len(c.Errors) == 0 {
			// Store response in cache
			responseData := gin.H{}
			if w.body != "" {
				json.Unmarshal([]byte(w.body), &responseData)
			}

			if data, err := json.Marshal(gin.H{
				"data": responseData,
			}); err == nil {
				redisClient.Set(ctx, cacheKey, data, defaultTTL)
			}
		}
	}
}

// InvalidateCacheMiddleware invalidates cache for specific patterns after mutations
func InvalidateCacheMiddleware(redisClient *redis.Client) gin.HandlerFunc {
	if redisClient == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		c.Next()

		// Only invalidate cache for successful mutations
		if c.Request.Method != "GET" && c.Writer.Status() < 300 {
			ctx := context.Background()
			patterns := getInvalidationPatterns(c.Request.URL.Path, c.Request.Method)

			for _, pattern := range patterns {
				iter := redisClient.Scan(ctx, 0, pattern, 0).Iterator()
				for iter.Next(ctx) {
					redisClient.Del(ctx, iter.Val())
				}
			}
		}
	}
}

// shouldSkipCache determines if a path should skip caching
func shouldSkipCache(path string) bool {
	skipPatterns := []string{
		"/api/v1/auth/",
		"/api/v1/accounts/verify",
		"/api/v1/accounts/balance",
		"/health",
		"/metrics",
	}

	for _, pattern := range skipPatterns {
		if strings.Contains(path, pattern) {
			return true
		}
	}

	return false
}

// generateCacheKey creates a unique cache key for the request
func generateCacheKey(c *gin.Context) string {
	// Format: cache:gateway:method:path:query
	path := strings.ReplaceAll(c.Request.URL.Path, "/", ":")
	query := c.Request.URL.Query().Encode()

	if query != "" {
		return fmt.Sprintf("cache:gateway:%s:%s:%s", c.Request.Method, path, query)
	}

	return fmt.Sprintf("cache:gateway:%s:%s", c.Request.Method, path)
}

// getInvalidationPatterns returns cache key patterns to invalidate
func getInvalidationPatterns(path, method string) []string {
	patterns := []string{}

	// Invalidate user-specific caches
	if strings.Contains(path, "/accounts/") || strings.Contains(path, "/users/") {
		patterns = append(patterns, "cache:gateway:GET:*accounts*")
		patterns = append(patterns, "cache:gateway:GET:*users*")
	}

	// Invalidate transaction caches
	if strings.Contains(path, "/transfers/") || strings.Contains(path, "/payments/") {
		patterns = append(patterns, "cache:gateway:GET:*transfers*")
		patterns = append(patterns, "cache:gateway:GET:*payments*")
		patterns = append(patterns, "cache:gateway:GET:*balance*")
	}

	return patterns
}

// responseWriter captures response status and body
type responseWriter struct {
	gin.ResponseWriter
	status int
	body   string
}

func (w *responseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(data []byte) (int, error) {
	w.body += string(data)
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) WriteString(s string) (int, error) {
	w.body += s
	return w.ResponseWriter.WriteString(s)
}

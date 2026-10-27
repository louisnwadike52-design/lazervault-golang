package middleware

// Per-principal HTTP response cache.
//
// This file is the SAME in every gateway (copied from business-gateway, which
// got it right first). The version it replaces keyed entries by method+path+
// query only — no user — so one person's authenticated GET could be served to
// the next person who asked for the same path, and because the middleware
// runs before JWT auth, a HIT answered without authenticating at all. It also
// stored the body wrapped in {"data": …} while a MISS returned the flat body,
// so clients broke on the first HIT. None of that could happen through gRPC
// (the app's main transport), but every HTTP route was exposed.
//
// Now: the key is scoped to a hash of the Authorization header (+ the active
// account), unauthenticated responses are never cached or served, the body
// is stored verbatim, any mutation by a principal drops that principal's
// cached reads, and a request carrying Cache-Control: no-cache bypasses the
// HIT path so a deliberate refresh always reaches the origin.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Cache design (business-gateway)
// --------------------------------
// GET responses for the business APIs are cached in Redis, but the cache is
// scoped PER PRINCIPAL — the key embeds a hash of the Authorization bearer token
// (+ the active account id). This is critical:
//   * Business A's cached list can never be served to Business B (no cross-tenant
//     leak) — the previous key was method:path:query only, shared across users.
//   * A forged/expired token produces a different key → cache MISS → the request
//     falls through to the JWT auth middleware and is rejected, so a cache HIT can
//     never bypass auth.
// On any successful mutation by a principal, ALL of that principal's cached GETs
// are invalidated, so a create/edit reflects immediately (no ≤5-min lag) without
// per-entity bookkeeping.

const cacheKeyPrefix = "cache:core-gateway:GET"

// principalScope derives a stable, per-caller cache scope from the request.
// Returns "" when the request is unauthenticated (→ do not cache).
func principalScope(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(auth))
	scope := hex.EncodeToString(sum[:])[:16]
	// Active business account also scopes the data (a user can switch accounts).
	if acct := c.GetHeader("X-Account-Id"); acct != "" {
		scope = scope + "-" + acct
	}
	return scope
}

// CacheMiddleware caches successful GET JSON responses per principal.
func CacheMiddleware(redisClient *redis.Client, defaultTTL time.Duration) gin.HandlerFunc {
	if redisClient == nil {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet || shouldSkipCache(c.Request.URL.Path) {
			c.Next()
			return
		}

		scope := principalScope(c)
		if scope == "" {
			// Never cache unauthenticated responses (and never serve cache to
			// an unauthenticated caller).
			c.Next()
			return
		}

		cacheKey := generateCacheKey(scope, c)
		ctx := context.Background()

		// A client that is deliberately refreshing (the app's background
		// revalidation sends Cache-Control: no-cache) must reach the origin;
		// the fresh response still refills the entry for everyone else.
		wantsFresh := strings.Contains(strings.ToLower(c.GetHeader("Cache-Control")), "no-cache")
		if cached, err := redisClient.Get(ctx, cacheKey).Result(); !wantsFresh && err == nil && cached != "" {
			// Return the cached body VERBATIM (same shape as a live response).
			c.Header("X-Cache", "HIT")
			c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(cached))
			c.Abort()
			return
		}

		c.Header("X-Cache", "MISS")
		w := &responseWriter{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = w
		c.Next()

		// Cache only clean 200 JSON responses.
		if w.status == http.StatusOK && len(c.Errors) == 0 && w.body != "" {
			redisClient.Set(ctx, cacheKey, w.body, defaultTTL)
		}
	}
}

// InvalidateCacheMiddleware clears the caller's cached GETs after any successful
// mutation, so their next read reflects the change immediately.
func InvalidateCacheMiddleware(redisClient *redis.Client) gin.HandlerFunc {
	if redisClient == nil {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		c.Next()

		if c.Request.Method == http.MethodGet || c.Writer.Status() >= 300 {
			return
		}
		scope := principalScope(c)
		if scope == "" {
			return
		}
		ctx := context.Background()
		// Coarse but correct: a mutation typically affects the entity list AND
		// the cross-derived overview/tax dashboard, so drop all of this
		// principal's cached reads.
		pattern := fmt.Sprintf("%s:%s:*", cacheKeyPrefix, scope)
		iter := redisClient.Scan(ctx, 0, pattern, 200).Iterator()
		for iter.Next(ctx) {
			redisClient.Del(ctx, iter.Val())
		}
	}
}

// shouldSkipCache determines if a path should never be cached.
func shouldSkipCache(path string) bool {
	skipPatterns := []string{
		"/api/v1/auth/",
		"/api/v1/accounts/verify",
		"/api/v1/accounts/balance",
		"/api/v1/payroll/process",
		"/api/v1/payroll/approve",
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

// generateCacheKey builds a per-principal cache key.
func generateCacheKey(scope string, c *gin.Context) string {
	path := strings.ReplaceAll(c.Request.URL.Path, "/", ":")
	query := c.Request.URL.Query().Encode()
	if query != "" {
		return fmt.Sprintf("%s:%s:%s:%s", cacheKeyPrefix, scope, path, query)
	}
	return fmt.Sprintf("%s:%s:%s", cacheKeyPrefix, scope, path)
}

// responseWriter captures response status and body.
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

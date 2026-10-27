package middleware

// Per-principal HTTP response cache.
//
// This file is the SAME in every gateway (copied from business-gateway, which
// got it right first) except for CacheKeyPrefix. The version it replaces keyed
// entries by method+path+query only — no user — so one person's authenticated
// GET could be served to the next person who asked for the same path, and
// because the middleware runs before JWT auth, a HIT answered without
// authenticating at all. It also stored the body wrapped in {"data": …} while
// a MISS returned the flat body, so clients broke on the first HIT.
//
// Now: the key is scoped to a hash of the Authorization header plus every
// request header these gateways FORWARD to the services behind them,
// unauthenticated responses are never cached or served, the body is stored
// verbatim, any mutation by a principal drops that principal's cached reads,
// and a request carrying Cache-Control: no-cache bypasses the HIT path so a
// deliberate refresh always reaches the origin.
//
// WHAT THE 2026-10-27 AUDIT CHANGED, AND WHY
//
//  1. THE KEY WAS MISSING THREE DIMENSIONS. customHeaderMatcher in every
//     gateway's main.go promotes X-Currency, X-User-Country and X-Locale into
//     gRPC metadata, and services behind these gateways read them to decide
//     WHICH ROWS AND WHICH PRICES to answer with — gift-card retail pricing by
//     X-Currency, the stock broker (NGX vs US) by X-User-Country, bill/tag-pay/
//     statistics row scope by X-Locale. Keyed on Authorization + X-Account-Id
//     only, one locale's or currency's answer was served for another. Every
//     forwarded, response-affecting header is now part of the scope.
//
//  2. A HIT COULD OUTLIVE THE CREDENTIAL. The design note claimed an expired
//     token "produces a different key → MISS". It does not: a token's STRING
//     does not change when it expires, so the same bearer kept being served
//     from cache after expiry — past the auth middleware, which a HIT never
//     reaches. An entry's TTL is now clamped to the token's own remaining
//     lifetime, and an already-expired token is never served or stored.
//
//  3. THE SKIP LIST WAS DEAD. It was copied from business-gateway and listed
//     payroll routes none of these gateways serve, plus
//     /api/v1/accounts/balance and /api/v1/accounts/verify, which are not
//     routes anywhere in the platform (the real one is
//     /api/v1/accounts/{account_id}/balance). Applied to the real route set it
//     matched almost nothing — so balances, KYC status, transfer status, fraud
//     freeze status, fee quotes, FX rates, and GET /v1/cards/{uuid}/reveal
//     (full PAN + CVV + card PIN) and GET /v1/cards/{uuid}/pin were all being
//     written to Redis. The list is now written against the routes these
//     gateways actually serve.
//
//  4. A MUTATION COULD MISS ITS OWN ENTRIES. Invalidation scanned
//     prefix:<full scope>:* while the scope carries the active account and
//     locale, so a mutation sent without X-Account-Id did not clear the reads
//     made with it. Invalidation is now by the token hash alone, which is the
//     prefix of every one of that principal's keys whatever the other
//     dimensions were.
//
// WHAT IT DOES NOT FIX (bounded, not closed — see the report)
//
//   - A mutation through a DIFFERENT gateway, over direct gRPC, from a
//     background worker, or from a provider webhook clears nothing here. The
//     money-visible reads are now skipped outright rather than invalidated,
//     and the default TTL is a minute, which is what bounds the rest.
//   - A mutation by a COUNTERPARTY (a group contribution, a split bill, a
//     donation) clears only the mutator's entries.
//   - A logout or a revoked impersonation session does not change the token
//     string, so a HIT remains possible until the token expires or the entry
//     ages out. Closing that properly means registering this middleware AFTER
//     the JWT and impersonation middlewares, which is a routing change per
//     gateway rather than a change to this file.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// CacheKeyPrefix is the namespace every entry of this gateway's response cache
// lives under. Exported because anything else that invalidates these entries
// has to build its patterns from it — the Kafka balance invalidator was still
// scanning the pre-2026-10-27 "cache:gateway:GET" namespace and silently
// matching nothing.
const CacheKeyPrefix = "cache:core-gateway:GET"

// maxCachedBodyBytes caps what one response may buffer. The old writer
// accumulated the whole body into a string with += on every Write, so a large
// (or streaming) response cost quadratic copying and unbounded memory with no
// upper bound at all. Past the cap the response still streams to the client
// untouched; it simply is not cached.
const maxCachedBodyBytes = 1 << 20 // 1 MiB

// scopingHeaders are the request headers this gateway FORWARDS to the services
// behind it (see customHeaderMatcher in main.go), so any of them can change the
// response body for the same principal on the same path+query. Each must be
// part of the cache key.
//
// X-User-Id and X-Username are forwarded too but are derived from the verified
// token by the auth middleware, so the Authorization hash already covers them.
var scopingHeaders = []struct{ tag, header string }{
	{"a", "X-Account-Id"},
	{"c", "X-Currency"},
	{"l", "X-Locale"},
	{"k", "X-User-Country"},
}

// nonceHeaders mark a request as something other than a plain read: a
// single-use transaction-PIN verification token authorises one action, and a
// response produced under one must never be stored or replayed.
var nonceHeaders = []string{"X-Transaction-Pin"}

// principalScope derives a stable per-caller cache scope from the request.
//
// Returns ("", "") when the request must not use the cache at all:
// unauthenticated, carrying a single-use nonce, or presenting a token that has
// already expired.
//
// tokenHash is the first segment of the scope and therefore the prefix of every
// key belonging to this principal, whatever their active account, currency or
// locale — which is what invalidation scans.
func principalScope(c *gin.Context) (tokenHash, scope string) {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		return "", ""
	}
	for _, h := range nonceHeaders {
		if c.GetHeader(h) != "" {
			return "", ""
		}
	}
	if exp, ok := bearerExpiry(auth); ok && !time.Now().Before(exp) {
		return "", "" // already expired: let it reach the auth middleware
	}
	sum := sha256.Sum256([]byte(auth))
	tokenHash = hex.EncodeToString(sum[:])[:16]

	var b strings.Builder
	b.WriteString(tokenHash)
	for _, s := range scopingHeaders {
		if v := sanitizeScopeValue(c.GetHeader(s.header)); v != "" {
			b.WriteString("-")
			b.WriteString(s.tag)
			b.WriteString(":")
			b.WriteString(v)
		}
	}
	return tokenHash, b.String()
}

// sanitizeScopeValue bounds and normalises a header value before it becomes
// part of a Redis key. Header values are caller-controlled, so without this a
// long header is an unbounded key and a value containing ':' could be made to
// read as another dimension's tag.
func sanitizeScopeValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

// bearerExpiry reads the `exp` claim out of a bearer JWT.
//
// The signature is deliberately NOT verified: this value is only ever used to
// SHORTEN an entry's life, never to authorise anything. A forged token with a
// far-future exp still hashes to its own scope, and the origin answers it with
// a 401 that is never cached, so it can only ever read entries it populated
// itself — which it cannot.
func bearerExpiry(auth string) (time.Time, bool) {
	tok := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(auth, "Bearer"), "bearer"))
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp <= 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

// cacheTTLFor clamps an entry's lifetime to whichever ends first: the
// configured TTL, or the credential that may read it.
func cacheTTLFor(auth string, defaultTTL time.Duration) time.Duration {
	exp, ok := bearerExpiry(auth)
	if !ok {
		return defaultTTL
	}
	if remaining := time.Until(exp); remaining < defaultTTL {
		return remaining
	}
	return defaultTTL
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

		_, scope := principalScope(c)
		if scope == "" {
			// Never cache unauthenticated responses (and never serve cache to
			// an unauthenticated caller, to a request carrying a single-use
			// nonce, or to an already-expired token).
			c.Next()
			return
		}

		// Tell any cache between here and the client that this body belongs to
		// one caller and one scope. Without it a shared proxy keyed on URL
		// alone is free to do exactly what this middleware was fixed for.
		c.Header("Vary", varyHeader)

		cacheKey := generateCacheKey(scope, c)
		ctx := context.Background()

		// A client that is deliberately refreshing (the app's background
		// revalidation sends Cache-Control: no-cache) must reach the origin;
		// the fresh response still refills the entry for everyone else.
		wantsFresh := strings.Contains(strings.ToLower(c.GetHeader("Cache-Control")), "no-cache")
		if cached, err := redisClient.Get(ctx, cacheKey).Result(); !wantsFresh && err == nil && cached != "" {
			// Return the cached body VERBATIM (same shape as a live response).
			c.Header("X-Cache", "HIT")
			c.Header("Cache-Control", "private, no-store")
			c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(cached))
			c.Abort()
			return
		}

		c.Header("X-Cache", "MISS")
		w := &responseWriter{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = w
		c.Next()

		if ttl := cacheTTLFor(c.GetHeader("Authorization"), defaultTTL); ttl > 0 && w.cacheable(c) {
			redisClient.Set(ctx, cacheKey, w.body.String(), ttl)
		}
	}
}

// varyHeader lists every request header this cache keys on.
var varyHeader = func() string {
	names := []string{"Authorization"}
	for _, s := range scopingHeaders {
		names = append(names, s.header)
	}
	return strings.Join(names, ", ")
}()

// cacheable says whether a captured response may be stored.
//
// Beyond "200 and non-empty", which is all the first version checked: the body
// must be JSON (this cache replays everything as application/json, so a PDF,
// an image or a WebSocket upgrade would be served back as a broken JSON
// document), the origin must not have asked for it not to be stored, and it
// must not carry a Set-Cookie — only the body is cached, so replaying it would
// drop the cookie the response existed to set.
func (w *responseWriter) cacheable(c *gin.Context) bool {
	if w.status != http.StatusOK || len(c.Errors) > 0 || w.tooLarge || w.body.Len() == 0 {
		return false
	}
	ct := strings.ToLower(w.Header().Get("Content-Type"))
	if !strings.Contains(ct, "application/json") && !strings.Contains(ct, "+json") {
		return false
	}
	if cc := strings.ToLower(w.Header().Get("Cache-Control")); strings.Contains(cc, "no-store") {
		return false
	}
	return w.Header().Get("Set-Cookie") == ""
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
		tokenHash, _ := principalScope(c)
		if tokenHash == "" {
			return
		}
		ctx := context.Background()
		// Coarse but correct: a mutation typically affects the entity list AND
		// the cross-derived overview, so drop all of this principal's cached
		// reads.
		//
		// Scanned by TOKEN HASH, not by the full scope. The scope also carries
		// the active account, currency and locale, so a mutation sent without
		// X-Account-Id built a different scope from the reads made with it and
		// cleared none of them. The hash is the prefix of every key this
		// principal owns, and it is exactly 16 hex characters, so it cannot
		// reach another principal's.
		pattern := fmt.Sprintf("%s:%s*", CacheKeyPrefix, tokenHash)
		iter := redisClient.Scan(ctx, 0, pattern, 200).Iterator()
		for iter.Next(ctx) {
			redisClient.Del(ctx, iter.Val())
		}
	}
}

// skipPatterns are the path fragments that must never be served from, or
// written to, this cache. Matched as substrings against the request path.
//
// Written against the routes these eight gateways actually serve. The list it
// replaces was copied from business-gateway and, applied to the real route
// set, matched 14 of 655 routes — none of them a balance, a status, a quote or
// a secret.
//
// Deliberately generous: the only cost of skipping a route that would have
// been safe is a cache miss, while the cost of caching one of these is a wrong
// balance, a stale fraud freeze, a priced-wrong trade, or a card PAN sitting
// in Redis.
var skipPatterns = []string{
	// ---- Bearer instruments and secret material. A cached copy is a copy of
	// a credential, replayable for the life of the entry.
	// GET /v1/cards/{uuid}/reveal returns card_number + cvv + card_pin;
	// GET /v1/cards/{uuid}/pin returns the PIN.
	"/cards", "reveal", "card-details", "/pin",
	"redeemcode", "/epin", "/qrpay/details", "/offers/shared", "group-join-links",

	// ---- Money state. Must be the truth at the instant it is asked for, and
	// the writes that change it mostly arrive over gRPC, from a worker, from a
	// provider webhook or through another gateway — none of which reach the
	// invalidator here.
	"balance", "/holds", "/accounts", "/transaction", "/ledger",
	"usage", "/statement", "/portfolio", "/holdings", "/limit",
	// Single-resource money reads: GET /api/v1/transfers/{reference},
	// /deposits/{id}, /withdrawals/{id}, /v1/cryptos/swap/{id} are what the
	// app polls while the money is still moving, and the id is in the path, so
	// only the collection name can match them.
	"transfers", "/deposits", "withdraw", "/mandates", "/charges", "/swap",
	// Payment lists and single payments, including the contactless merchant
	// view — live money state whose writes come from the counterparty.
	"/payments",

	// ---- Anything the app polls while money is in flight, and anything that
	// gates money. A five-minute-old "pending", "not frozen" or "tier 1" is a
	// money-safety defect, not a stale screen.
	"status", "verif", "/freeze", "fraud", "/kyc", "/tier", "credit-score",

	// ---- Prices. Showing a rate or a fee that is no longer the one that will
	// be charged is a priced-wrong risk.
	"rate", "quote", "fee",

	// ---- Credentials, sessions and one-time values.
	"/auth/", "/otp", "passcode", "/2fa", "/token", "/session",
	"challenge", "nonce",

	// ---- Receipts are generated while a transaction can still change.
	"/receipt",

	// ---- Not JSON reads at all: a WebSocket upgrade must never be buffered
	// or replayed, and the probes are pointless to cache.
	"/ws/", "/health", "/metrics", "/ready",
}

// shouldSkipCache determines if a path should never be cached.
func shouldSkipCache(path string) bool {
	lower := strings.ToLower(path)
	for _, pattern := range skipPatterns {
		if strings.Contains(lower, pattern) {
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
		return fmt.Sprintf("%s:%s:%s:%s", CacheKeyPrefix, scope, path, query)
	}
	return fmt.Sprintf("%s:%s:%s", CacheKeyPrefix, scope, path)
}

// responseWriter captures response status and body.
type responseWriter struct {
	gin.ResponseWriter
	status   int
	body     bytes.Buffer
	tooLarge bool
}

func (w *responseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

// capture buffers the body for storage, and stops — marking the response
// uncacheable — rather than growing without limit.
func (w *responseWriter) capture(data []byte) {
	if w.tooLarge {
		return
	}
	if w.body.Len()+len(data) > maxCachedBodyBytes {
		w.tooLarge = true
		w.body.Reset()
		return
	}
	w.body.Write(data)
}

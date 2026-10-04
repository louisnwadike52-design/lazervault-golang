package middleware

// READ-ONLY ENFORCEMENT FOR IMPERSONATED SESSIONS.
//
// GENERATED-IDENTICAL FILE. One copy lives in every gateway's
// grpcApi/middleware/ directory and they must stay byte-identical — there is a
// checksum test that fails if one drifts. Edit the copy in core-gateway and
// re-run scripts/sync_impersonation_guard.sh.
//
// WHAT THIS CLOSES
// ----------------
// auth-service already refuses to mint a transaction-PIN verification token for
// an impersonated session, which closes every MONEY path at once (all fourteen
// money services require that token). What it did not close was everything that
// needs no PIN: changing a display name, marking a notification read, deleting a
// saved recipient. This is that gap.
//
// The rule is deliberately crude in the safe direction:
//
//   HTTP  — only GET / HEAD / OPTIONS are allowed. There is NO allowlist of
//           "read-only POSTs" (fee quotes, name inquiry). An impersonated admin
//           is looking, not transacting, so it does not need to quote a fee —
//           and a missing allowlist entry shows up as a visibly refused read,
//           which someone fixes, rather than as a mutation that quietly went
//           through, which nobody sees.
//
//   gRPC  — an ALLOWLIST of read verb prefixes. An unrecognised method name is
//           DENIED. New RPCs appear constantly and a deny-list would silently
//           permit every one of them; an allowlist fails closed, so the cost of
//           forgetting is a refused read rather than an open write.
//
// Note `Verify` is NOT a read prefix. VerifyTransactionPin mutates attempt
// counters and is the money gate itself.
//
// REVOCATION (so "exit" means something server-side)
// --------------------------------------------------
// Each impersonation token carries a jti. Ending a session records that jti as
// revoked in auth-service, and this guard refuses any request whose jti is on
// that list. The list is fetched from auth-service and cached briefly, so the
// window between an admin pressing Exit and the token becoming inert is the
// cache TTL (10s) rather than the token's full 15-minute lifetime.
//
// Fetch failures FAIL OPEN on revocation only — a token that is still within
// its TTL and not known-revoked continues to work for READS. That is deliberate:
// failing closed would mean a hiccup talking to auth-service logs every
// impersonated admin out mid-investigation, and the thing being protected here
// is read access to data the admin is already authorised to see, with all
// writes already denied above.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// impersonationReadOnlyMessage is what the client shows the admin.
const impersonationReadOnlyMessage = "You're viewing this account in read-only mode. " +
	"Exit view-as-user to make changes."

const impersonationRevokedMessage = "This view-as-user session has ended. " +
	"Start a new one to continue."

// grpcReadVerbPrefixes is the ALLOWLIST. Anything not starting with one of
// these is treated as a mutation and denied.
var grpcReadVerbPrefixes = []string{
	"Get", "List", "Search", "Check", "Fetch", "Resolve", "Calculate",
	"Quote", "Download", "Export", "Preview", "Lookup", "Describe",
	"Count", "Has", "Is", "Read", "Query", "Find", "Load", "Validate",
	"Health", "Watch", "Stream", "Subscribe",
}

// ImpersonationClaims is the minimal shape the guard reads from a token.
type ImpersonationClaims struct {
	Impersonation  bool   `json:"imp"`
	ImpersonatedBy string `json:"imp_by"`
	jwt.RegisteredClaims
}

// impersonationFromToken reads the impersonation claims out of a raw bearer
// token WITHOUT verifying the signature.
//
// Safe here for the same reason as auth-service's copy: this guard only ever
// DENIES, so a forged token can at worst have its own request refused. And by
// the time a request reaches this point the gateway has ALREADY verified the
// signature in authorize()/JWTAuthMiddleware — this runs after that, on a token
// known to be genuine. Claims validation is skipped so an EXPIRED impersonation
// token is still recognised as impersonated rather than silently treated as an
// ordinary session.
func impersonationFromToken(raw string) (actorID, sessionID string, impersonated bool) {
	tokenStr := strings.TrimSpace(raw)
	if tokenStr == "" {
		return "", "", false
	}
	if len(tokenStr) > 7 && strings.EqualFold(tokenStr[:7], "bearer ") {
		tokenStr = strings.TrimSpace(tokenStr[7:])
	}
	if tokenStr == "" {
		return "", "", false
	}
	claims := &ImpersonationClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(tokenStr, claims); err != nil {
		return "", "", false
	}
	if !claims.Impersonation && strings.TrimSpace(claims.ImpersonatedBy) == "" {
		return "", "", false
	}
	return strings.TrimSpace(claims.ImpersonatedBy), strings.TrimSpace(claims.ID), true
}

func bearerFromIncomingContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, key := range []string{"authorization", "grpcgateway-authorization"} {
		if vals := md.Get(key); len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
			return vals[0]
		}
	}
	return ""
}

// grpcMethodIsRead reports whether a fully-qualified gRPC method looks like a
// read. Unknown shapes are NOT reads.
func grpcMethodIsRead(fullMethod string) bool {
	idx := strings.LastIndex(fullMethod, "/")
	if idx < 0 || idx == len(fullMethod)-1 {
		return false
	}
	name := fullMethod[idx+1:]
	for _, p := range grpcReadVerbPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// httpMethodIsSafe reports whether an HTTP method cannot mutate.
func httpMethodIsSafe(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// ── revocation list ───────────────────────────────────────────────────────

var (
	revokedMu      sync.RWMutex
	revokedSet     map[string]struct{}
	revokedFetched time.Time
	revokedTTL     = 10 * time.Second
	revokedClient  = &http.Client{Timeout: 3 * time.Second}
)

// ImpersonationRevocationURL is where the revoked-jti list is fetched from.
//
// Resolved from the environment on first use rather than set by each gateway's
// main: there are eleven mains with eleven different startup shapes, and a
// setter that one of them forgets to call is a guard that silently stops
// honouring Exit in exactly one service. Reading the env means every gateway
// gets it for free. Settable for tests and for an explicit override.
var ImpersonationRevocationURL string

var revocationURLOnce sync.Once

func revocationBaseURL() string {
	if u := strings.TrimSpace(ImpersonationRevocationURL); u != "" {
		return u
	}
	revocationURLOnce.Do(func() {
		for _, key := range []string{
			"IMPERSONATION_REVOCATION_URL",
			"AUTH_SERVICE_HTTP_URL",
			"AUTH_SERVICE_URL",
		} {
			if v := strings.TrimSpace(os.Getenv(key)); v != "" {
				ImpersonationRevocationURL = v
				return
			}
		}
		// Every gateway runs alongside auth-service on this host. A default
		// beats "revocation silently off because an env var was missing".
		ImpersonationRevocationURL = "http://127.0.0.1:18081"
	})
	return strings.TrimSpace(ImpersonationRevocationURL)
}

func impersonationSessionRevoked(sessionID string) bool {
	if sessionID == "" || revocationBaseURL() == "" {
		return false
	}
	revokedMu.RLock()
	fresh := time.Since(revokedFetched) < revokedTTL
	set := revokedSet
	revokedMu.RUnlock()

	if !fresh {
		set = refreshRevokedSet()
	}
	if set == nil {
		// Fetch failed and nothing cached. Fails OPEN on revocation only, by
		// design — see the note at the top of this file.
		return false
	}
	_, found := set[sessionID]
	return found
}

func refreshRevokedSet() map[string]struct{} {
	revokedMu.Lock()
	defer revokedMu.Unlock()
	// Another goroutine may have refreshed while we waited for the lock.
	if time.Since(revokedFetched) < revokedTTL {
		return revokedSet
	}

	resp, err := revokedClient.Get(
		strings.TrimRight(revocationBaseURL(), "/") +
			"/api/v1/internal/impersonation/revoked")
	if err != nil {
		// Keep whatever we had. A stale list is better than none: it still
		// blocks everything it already knew about.
		return revokedSet
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return revokedSet
	}
	var body struct {
		Revoked []string `json:"revoked"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return revokedSet
	}
	next := make(map[string]struct{}, len(body.Revoked))
	for _, id := range body.Revoked {
		if id = strings.TrimSpace(id); id != "" {
			next[id] = struct{}{}
		}
	}
	revokedSet = next
	revokedFetched = time.Now()
	return revokedSet
}

// ── the two enforcement points ────────────────────────────────────────────

// CheckImpersonationGRPC returns a non-nil error when an impersonated session
// must not be allowed to call fullMethod.
func CheckImpersonationGRPC(ctx context.Context, fullMethod string) error {
	_, sessionID, impersonated := impersonationFromToken(bearerFromIncomingContext(ctx))
	if !impersonated {
		return nil
	}
	if impersonationSessionRevoked(sessionID) {
		return status.Error(codes.PermissionDenied, impersonationRevokedMessage)
	}
	if grpcMethodIsRead(fullMethod) {
		return nil
	}
	return status.Error(codes.PermissionDenied, impersonationReadOnlyMessage)
}

// ImpersonationHTTPGuard aborts a mutating HTTP request made by an
// impersonated session. Mount AFTER the JWT middleware.
func ImpersonationHTTPGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, sessionID, impersonated := impersonationFromToken(
			c.GetHeader("Authorization"))
		if !impersonated {
			c.Next()
			return
		}
		if impersonationSessionRevoked(sessionID) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": impersonationRevokedMessage,
				"code":  "IMPERSONATION_REVOKED",
			})
			return
		}
		if httpMethodIsSafe(c.Request.Method) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": impersonationReadOnlyMessage,
			"code":  "IMPERSONATION_READ_ONLY",
		})
	}
}

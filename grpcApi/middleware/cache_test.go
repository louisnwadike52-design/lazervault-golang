package middleware

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// req builds a GET context with the given headers.
func req(path string, headers map[string]string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", path, nil)
	for k, v := range headers {
		c.Request.Header.Set(k, v)
	}
	return c
}

// jwtWithExp mints an UNSIGNED token whose payload carries exp. The middleware
// never verifies the signature — it only reads exp to shorten an entry's life.
func jwtWithExp(exp time.Time) string {
	payload, _ := json.Marshal(map[string]int64{"exp": exp.Unix()})
	return "Bearer x." + base64.RawURLEncoding.EncodeToString(payload) + ".y"
}

// ---- the key's scoping dimensions ----

// A rotated or refreshed JWT is a different string, so it hashes to a different
// scope: a MISS, never another principal's entry. This is the property the
// whole design rests on.
func TestRotatedTokenIsAMissNotALeak(t *testing.T) {
	_, a := principalScope(req("/x", map[string]string{"Authorization": "Bearer old"}))
	_, b := principalScope(req("/x", map[string]string{"Authorization": "Bearer new"}))
	if a == "" || b == "" {
		t.Fatal("both tokens must produce a scope")
	}
	if a == b {
		t.Fatal("two different tokens must not share a scope")
	}
	ka := generateCacheKey(a, req("/x", nil))
	kb := generateCacheKey(b, req("/x", nil))
	if ka == kb {
		t.Fatal("different scopes must produce different keys")
	}
}

// X-Currency, X-Locale and X-User-Country are forwarded to the services behind
// this gateway (customHeaderMatcher) and they change which rows and which
// prices come back. Each must split the key.
func TestEveryForwardedScopingHeaderSplitsTheKey(t *testing.T) {
	base := map[string]string{"Authorization": "Bearer t"}
	_, want := principalScope(req("/x", base))

	for _, h := range []string{"X-Account-Id", "X-Currency", "X-Locale", "X-User-Country"} {
		with := map[string]string{"Authorization": "Bearer t", h: "VALUE"}
		_, got := principalScope(req("/x", with))
		if got == want {
			t.Fatalf("%s does not change the cache scope — a response built for one "+
				"%s can be served for another", h, h)
		}
	}

	// And two dimensions together differ from either alone.
	_, cur := principalScope(req("/x", map[string]string{"Authorization": "Bearer t", "X-Currency": "USD"}))
	_, loc := principalScope(req("/x", map[string]string{"Authorization": "Bearer t", "X-Locale": "USD"}))
	if cur == loc {
		t.Fatal("X-Currency: USD and X-Locale: USD must not collapse to one scope")
	}
}

// A value crafted to look like another dimension's tag must not be able to
// forge a scope (…-a:A-c:B could otherwise be produced two ways).
func TestScopeValuesCannotForgeAnotherDimension(t *testing.T) {
	_, forged := principalScope(req("/x", map[string]string{
		"Authorization": "Bearer t", "X-Account-Id": "A-c:USD",
	}))
	_, real := principalScope(req("/x", map[string]string{
		"Authorization": "Bearer t", "X-Account-Id": "A", "X-Currency": "USD",
	}))
	if forged == real {
		t.Fatal("a header value must not be able to impersonate another dimension")
	}
}

func TestScopeValuesAreBounded(t *testing.T) {
	_, scope := principalScope(req("/x", map[string]string{
		"Authorization": "Bearer t", "X-Locale": strings.Repeat("z", 5000),
	}))
	if len(scope) > 200 {
		t.Fatalf("a caller-controlled header must not make an unbounded Redis key: %d chars", len(scope))
	}
}

func TestUnauthenticatedIsNeverCached(t *testing.T) {
	if _, scope := principalScope(req("/x", nil)); scope != "" {
		t.Fatal("no Authorization header must mean no cache")
	}
}

// ---- the credential's lifetime bounds the entry's ----

func TestExpiredTokenIsNeverServedFromCache(t *testing.T) {
	auth := jwtWithExp(time.Now().Add(-time.Minute))
	if _, scope := principalScope(req("/x", map[string]string{"Authorization": auth})); scope != "" {
		t.Fatal("an already-expired token must fall through to the auth middleware, not to a HIT")
	}
}

func TestLiveTokenStillCaches(t *testing.T) {
	auth := jwtWithExp(time.Now().Add(time.Hour))
	if _, scope := principalScope(req("/x", map[string]string{"Authorization": auth})); scope == "" {
		t.Fatal("a valid token must still be cacheable")
	}
}

func TestTTLIsClampedToTheTokensRemainingLife(t *testing.T) {
	short := jwtWithExp(time.Now().Add(10 * time.Second))
	if got := cacheTTLFor(short, 5*time.Minute); got > 11*time.Second {
		t.Fatalf("an entry must not outlive the token that may read it: got %v", got)
	}
	long := jwtWithExp(time.Now().Add(time.Hour))
	if got := cacheTTLFor(long, 60*time.Second); got != 60*time.Second {
		t.Fatalf("a long-lived token keeps the configured TTL: got %v", got)
	}
	// An opaque (non-JWT) credential has no claim to honour.
	if got := cacheTTLFor("Bearer opaque-api-key", 60*time.Second); got != 60*time.Second {
		t.Fatalf("a non-JWT bearer keeps the configured TTL: got %v", got)
	}
}

// ---- nonce-bearing requests ----

func TestSingleUsePinTokenIsNeverCached(t *testing.T) {
	_, scope := principalScope(req("/x", map[string]string{
		"Authorization": "Bearer t", "X-Transaction-Pin": "one-shot",
	}))
	if scope != "" {
		t.Fatal("a request carrying a single-use PIN token must not be cached or served from cache")
	}
}

// ---- invalidation reaches every one of the principal's keys ----

// The mutation and the read need not carry the same account / currency /
// locale. Invalidation scans by token hash, so it must be a prefix of every key
// the principal owns.
func TestInvalidationPrefixCoversEveryScopeOfThePrincipal(t *testing.T) {
	hash, bare := principalScope(req("/x", map[string]string{"Authorization": "Bearer t"}))
	pattern := fmt.Sprintf("%s:%s", CacheKeyPrefix, hash)

	for _, headers := range []map[string]string{
		{"Authorization": "Bearer t"},
		{"Authorization": "Bearer t", "X-Account-Id": "ACC1"},
		{"Authorization": "Bearer t", "X-Account-Id": "ACC1", "X-Currency": "USD"},
		{"Authorization": "Bearer t", "X-Locale": "en-NG", "X-User-Country": "NG"},
	} {
		_, scope := principalScope(req("/x", headers))
		key := generateCacheKey(scope, req("/api/v1/thing", nil))
		if !strings.HasPrefix(key, pattern) {
			t.Fatalf("key %q is not reachable from invalidation pattern %q*", key, pattern)
		}
	}

	// And it must NOT reach another principal: every hash is exactly 16 chars.
	otherHash, _ := principalScope(req("/x", map[string]string{"Authorization": "Bearer other"}))
	if len(hash) != 16 || len(otherHash) != 16 {
		t.Fatalf("hash length must be fixed at 16 or the prefix could straddle principals: %d / %d",
			len(hash), len(otherHash))
	}
	if strings.HasPrefix(otherHash, hash) {
		t.Fatal("one principal's pattern must not match another's keys")
	}
	_ = bare
}

// ---- the skip list is about the real routes ----

func TestSkipsEveryRouteThatMustNeverBeCached(t *testing.T) {
	// Each of these is a live GET on one of the eight gateways.
	mustSkip := []string{
		// secret material
		"/v1/cards/9d1/reveal",            // card_number + cvv + card_pin
		"/v1/cards/9d1/pin",               // the card PIN
		"/v1/cards",                       // limits + masked card state
		"/api/v1/giftcards/redeemcode",    // redemption code + pin
		"/api/v1/bills/epin/orders/1",     // e-PIN voucher codes
		"/api/v1/qrpay/details/QR123",     // QR payload resolution
		"/api/v1/escrow/offers/shared/tk", // share-token resource
		"/v1/group-join-links/tk",         // join-link token
		// money state
		"/api/v1/accounts/abc/balance",
		"/api/v1/accounts/abc/holds",
		"/api/v1/accounts/abc/transactions",
		"/api/v1/accounts/summary",
		"/api/v1/transactions/history",
		"/api/v1/transactions/analytics/financial",
		"/api/v1/accounts/abc/spending-usage",
		"/api/v1/transfers/daily-usage",
		"/api/v1/accounts/abc/statement",
		"/v1/cryptos/wallets/w1/balance",
		"/v1/cryptos/holdings",
		"/api/v1/investments/portfolio",
		"/v1/insurance/wallet/balance",
		"/v1/crowdfunds/c1/wallet-balance",
		"/v1/referral/points/balance",
		"/api/v1/sms/balance",
		// gates and polled status
		"/api/v1/kyc/status",
		"/api/v1/kyc/documents",
		"/api/v1/kyc/prove/status",
		"/api/v1/fraud-detection/freeze-status",
		"/api/v1/fraud-detection/setting",
		"/api/v1/transfers/TRF-1",
		"/api/v1/deposits/d1",
		"/api/v1/withdrawals/w1",
		"/api/v1/contactless/sessions/s1/status",
		"/v1/cryptos/swap/t1",
		"/v1/insurance/purchase/REF/status",
		"/api/v1/users/u1/verification",
		"/api/v1/users/u1/credit-score",
		"/v1/exchange/transfers/t1/status",
		// prices
		"/v1/exchange/rates",
		"/v1/exchange/limits",
		"/api/v1/transfers/exchange-rate",
		"/api/v1/giftcards/sellrate",
		"/api/v1/investments/quote/AAPL",
		"/api/v1/transfers/fee",
		"/api/v1/deposits/calculate-fee",
		"/v1/uplifts/fee-quote",
		"/api/v1/invoices/servicefee",
		// credentials and sessions
		"/api/v1/auth/me",
		"/v1/users/passcode/exists",
		"/api/v1/auth/2fa/status",
		"/api/v1/whatsapp/sessions/s1",
		// receipts, upgrades, probes
		"/api/v1/qrpay/receipt/t1",
		// payment lists — live money state written by the counterparty
		"/api/v1/contactless/payments",
		"/v1/insurance/payments",
		"/v1/electricity/payments",
		"/ws/transfer",
		"/health",
		"/metrics",
		"/ready",
	}
	for _, p := range mustSkip {
		if !shouldSkipCache(p) {
			t.Errorf("MUST NOT be cached but is: %s", p)
		}
	}
}

// The four entries the old list carried matched no route in the platform. They
// are gone; this records what they were meant to protect, through patterns that
// actually match.
func TestTheDeadSkipEntriesAreNowCoveredByRealOnes(t *testing.T) {
	// "/api/v1/accounts/balance" was not a route. The real one is:
	if !shouldSkipCache("/api/v1/accounts/0f3e/balance") {
		t.Error("the real balance route must be skipped")
	}
	// "/api/v1/accounts/verify" was not a route either.
	if !shouldSkipCache("/api/v1/users/u1/verification") {
		t.Error("the real verification route must be skipped")
	}
}

// Catalogue reads are the whole point of having a cache here, so they must
// survive the list.
func TestCataloguesStayCacheable(t *testing.T) {
	cacheable := []string{
		"/api/v1/bills/billproviders",
		"/api/v1/bills/dataplans",
		"/api/v1/bills/tvpackages",
		"/api/v1/bills/internetpackages",
		"/api/v1/bills/educationproviders",
		"/api/v1/bills/intl-airtime/countries",
		"/api/v1/sms/providers",
		"/api/v1/sms/packages",
		"/v1/insurance/products",
		"/v1/insurance/auxiliary/states",
		"/v1/insurance/auxiliary/vehicle-makes",
		"/v1/crowdfunds/categories/custom",
		"/v1/cryptos/learn/lessons",
		"/api/v1/banks",
		"/api/v1/investments/assets/categories",
	}
	for _, p := range cacheable {
		if shouldSkipCache(p) {
			t.Errorf("a catalogue read should still be cacheable: %s", p)
		}
	}
}

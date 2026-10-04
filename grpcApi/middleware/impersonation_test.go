package middleware

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func mintImpToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte("irrelevant-the-gateway-already-verified"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestImpersonationClaimsAreRead(t *testing.T) {
	token := mintImpToken(t, jwt.MapClaims{
		"sub": "target", "imp": true, "imp_by": "actor-9", "jti": "sess-1",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	})
	actor, session, impersonated := impersonationFromToken("Bearer " + token)
	if !impersonated || actor != "actor-9" || session != "sess-1" {
		t.Fatalf("got (%q,%q,%v)", actor, session, impersonated)
	}
}

func TestOrdinaryTokenIsNotImpersonated(t *testing.T) {
	token := mintImpToken(t, jwt.MapClaims{
		"sub": "u", "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, _, imp := impersonationFromToken("Bearer " + token); imp {
		t.Fatal("an ordinary session must pass through untouched — otherwise " +
			"this guard would block every real user's writes")
	}
}

func TestExpiredImpersonationTokenIsStillRecognised(t *testing.T) {
	// Claims validation is off on purpose. If an expired impersonation token
	// read as "not impersonated", waiting out the 15-minute TTL would buy full
	// write access across every gateway.
	token := mintImpToken(t, jwt.MapClaims{
		"sub": "u", "imp": true, "imp_by": "a", "jti": "s",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	if _, _, imp := impersonationFromToken("Bearer " + token); !imp {
		t.Fatal("an expired impersonation token must still be recognised")
	}
}

func TestHTTPOnlySafeMethodsAreAllowed(t *testing.T) {
	for _, m := range []string{"GET", "HEAD", "OPTIONS", "get", "head"} {
		if !httpMethodIsSafe(m) {
			t.Errorf("%s must be allowed", m)
		}
	}
	// No read-only-POST allowlist, deliberately: a missed entry shows up as a
	// visibly refused read rather than a mutation that quietly succeeded.
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE", "post", ""} {
		if httpMethodIsSafe(m) {
			t.Errorf("%s must be denied for an impersonated session", m)
		}
	}
}

func TestGRPCReadAllowlistFailsClosed(t *testing.T) {
	reads := []string{
		"/pb.AccountsService/GetUserAccounts",
		"/pb.BankingService/ListDeposits",
		"/pb.AuthService/SearchUsers",
		"/pb.TransactionPinService/CheckUserHasPin",
		"/pb.TransactionPinService/ValidateTransactionPinToken",
		"/pb.BankingService/CalculateDepositFee",
		"/grpc.health.v1.Health/Check",
	}
	for _, m := range reads {
		if !grpcMethodIsRead(m) {
			t.Errorf("%s should be readable in a read-only session", m)
		}
	}

	writes := []string{
		// The money gate itself. `Verify` is deliberately NOT a read prefix:
		// VerifyTransactionPin mutates attempt counters and mints the token all
		// fourteen money services accept.
		"/pb.TransactionPinService/VerifyTransactionPin",
		"/pb.PaymentService/ExecuteTransfer",
		"/pb.AccountsService/UpdateProfile",
		"/pb.AccountsService/DeleteRecipient",
		"/pb.BankingService/InitiateDeposit",
		"/pb.BankingService/CreateMandate",
		"/pb.NotificationService/MarkAllRead",
		"/pb.AuthService/ChangePassword",
		"/pb.AuthService/SetDefaultAccount",
		// Unknown verbs must be denied, which is the whole point of an
		// allowlist: a new RPC nobody added here is refused, not permitted.
		"/pb.SomeService/FrobnicateWidget",
		// Malformed shapes are not reads.
		"NoSlashAtAll",
		"/pb.Service/",
		"",
	}
	for _, m := range writes {
		if grpcMethodIsRead(m) {
			t.Errorf("%s must be DENIED for an impersonated session", m)
		}
	}
}

func TestRevocationIsOffWhenNoURLResolves(t *testing.T) {
	// An empty session id can never be revoked — there is nothing to match.
	if impersonationSessionRevoked("") {
		t.Fatal("an empty session id must not be treated as revoked")
	}
}

// TestGuardCopiesAreIdentical is the drift check the file header promises.
//
// The guard is duplicated into eleven gateways because they are separate Go
// modules that cannot import a shared package. Duplication is only safe if it
// is verified duplication: one gateway quietly falling behind would be a
// gateway where impersonated writes still work, and nothing else would notice.
func TestGuardCopiesAreIdentical(t *testing.T) {
	servicesDir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve services dir: %v", err)
	}
	gateways := []string{
		"core-gateway", "banking-gateway", "business-gateway", "chat-proxy-gateway",
		"commerce-gateway", "financial-gateway", "investment-gateway",
		"lifestyle-gateway", "products-gateway", "statistics-gateway",
		"transfer-gateway",
	}

	sums := map[string][]string{}
	for _, g := range gateways {
		p := filepath.Join(servicesDir, g, "grpcApi", "middleware", "impersonation.go")
		b, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: guard copy missing (%v) — that gateway has NO "+
				"read-only enforcement", g, err)
			continue
		}
		h := md5.Sum(b)
		sum := hex.EncodeToString(h[:])
		sums[sum] = append(sums[sum], g)
	}
	if len(sums) > 1 {
		for sum, gs := range sums {
			t.Errorf("checksum %s: %v", sum[:8], gs)
		}
		t.Fatal("the guard copies have DIVERGED — edit core-gateway's copy and " +
			"run scripts/sync_impersonation_guard.sh")
	}
}

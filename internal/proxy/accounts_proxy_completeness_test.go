package proxy

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	accountspb "accounts-service/proto"
)

// Every CLIENT-FACING AccountsService RPC must be forwarded by the proxy.
//
// THE BUG CLASS THIS CATCHES
// --------------------------
// AccountsServiceProxy embeds UnimplementedAccountsServiceServer so it
// satisfies the server interface without listing every RPC. That convenience
// is a runtime-only trap:
//
//	add an RPC → implement it in accounts-service → regenerate →
//	forget the one-line forwarder here → everything builds, everything
//	deploys, and the app gets Unimplemented.
//
// Confirmed instances: LeaveFamilyAccount (2026-09-17), and both
// foreign-virtual-account RPCs (2026-09-22) — implemented, deployed, and
// unreachable from the app for their entire life.
//
// WHY THERE IS AN EXCLUSION LIST, AND WHY IT IS NOT LAZINESS
// ----------------------------------------------------------
// Most AccountsService RPCs must NOT be reachable from core-gateway. The first
// version of this test asserted "forward everything" and reported 68 missing —
// a list led by CreditBalance, DebitBalance, HoldFunds, CaptureHold and
// TransferFromPlatformWallet. Forwarding those would put the platform's money
// primitives on the public edge, where a client could credit its own balance.
// "Make the test pass" would have been a severe security regression.
//
// So exclusions are explicit and categorised, and anything NOT excluded must be
// forwarded. A new client-facing RPC still fails this test; a new money
// primitive has to be named here, which is a decision someone makes on purpose.
//
// REFLECTION DOES NOT WORK HERE. Promoted (embedded) methods are
// indistinguishable from declared ones at runtime — the proxy "has" every
// method either way. The source is the only place the distinction survives.
func TestAccountsProxyForwardsEveryClientRPC(t *testing.T) {
	const proxyFile = "accounts_proxy.go"

	src, err := os.ReadFile(proxyFile)
	if err != nil {
		t.Fatalf("cannot read %s: %v", proxyFile, err)
	}

	declared := map[string]bool{}
	re := regexp.MustCompile(`func \(\w+ \*AccountsServiceProxy\) (\w+)\(`)
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		declared[m[1]] = true
	}
	if len(declared) == 0 {
		t.Fatal("no proxy methods found — the regex or the file layout changed")
	}

	iface := reflect.TypeOf((*accountspb.AccountsServiceServer)(nil)).Elem()

	var missing []string
	for i := 0; i < iface.NumMethod(); i++ {
		name := iface.Method(i).Name
		if name == "mustEmbedUnimplementedAccountsServiceServer" {
			continue
		}
		if notClientFacing(name) {
			continue
		}
		if !declared[name] {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Errorf("%d client-facing AccountsService RPC(s) are NOT forwarded — "+
			"callers get Unimplemented at runtime:\n  %v\n\n"+
			"Either add a forwarder to %s, or — if it is genuinely not for "+
			"clients — name it in notClientFacing() with the reason.",
			len(missing), missing, proxyFile)
	}
}

// notClientFacing reports whether an RPC is deliberately unreachable here.
func notClientFacing(name string) bool {
	// Operator console. Served by admin-gateway, behind admin auth and role
	// checks. Exposing these on the customer edge would hand any client
	// account freezes, platform-wallet reads and ops-alert control.
	if strings.HasPrefix(name, "Admin") {
		return true
	}
	return internalMoneyRPCs[name] || otherGatewayRPCs[name]
}

// internalMoneyRPCs are called SERVICE-TO-SERVICE and must never be public.
//
// These are the primitives every other service uses to move value. A client
// able to call CreditBalance directly could mint its own balance; one able to
// call ReleaseHold could free funds it had not paid for. They are reachable
// only from inside the mesh, and that is the whole control.
var internalMoneyRPCs = map[string]bool{
	"CreditBalance":             true,
	"DebitBalance":              true,
	"UpdateBalance":             true,
	"TransferBalance":           true,
	"HoldFunds":                 true,
	"CaptureHold":               true,
	"ReleaseHold":               true,
	"UnlockFunds":               true,
	"CreditPlatformWallet":      true,
	"DebitPlatformWallet":       true,
	"TransferToPlatformWallet":  true,
	"TransferFromPlatformWallet": true,
	"GetPlatformWallet":         true,
	"GetPlatformWalletTransactions": true,
	"ClearDeposit":              true,
	"CreditToClearing":          true,
	"ReverseClearingDeposit":    true,
	"CreateTransfer":            true,
	"GetTransfer":               true,
	"CreateVirtualAccount":      true,
	"GetAccountByProviderRef":   true,
	"GetLedgerEntriesByReference":  true,
	"GetTotalLedgerBalance":     true,
	"LookupTransactionByReference": true,
}

// otherGatewayRPCs are client-facing but served by a DIFFERENT gateway.
//
// PiggyVault / lock-funds and AutoSave reach the app through
// financial-gateway, which owns those product surfaces. Forwarding them here
// too would give one feature two public entry points with separate middleware
// — and the one nobody remembers is the one that drifts.
var otherGatewayRPCs = map[string]bool{
	"CreateLockFunds":          true,
	"CancelLockFunds":          true,
	"RenewLockFunds":           true,
	"TopUpLockFunds":           true,
	"GetLockFunds":             true,
	"GetLockFundsSettlements":  true,
	"CreateLockFundAutoSave":   true,
	"DeleteLockFundAutoSave":   true,
	"GetLockFundAutoSave":      true,
	"UpdateLockFundAutoSave":   true,
	"CreateAutoSave":           true,
	"UpdateAutoSave":           true,
	"DeleteAutoSave":           true,
	"GetAutoSaves":             true,
	"GetPiggyVaultConfig":      true,
	"GetAllPiggyVaultConfigs":  true,
	"UpdatePiggyVaultConfig":   true,
}

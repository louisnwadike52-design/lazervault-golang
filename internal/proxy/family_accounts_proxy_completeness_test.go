package proxy

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	accountspb "accounts-service/proto"
)

// The proxy embeds UnimplementedFamilyAccountsServiceServer so it satisfies the
// server interface without listing every RPC. That convenience is also a trap:
// an RPC added to the proto and implemented in the microservice still returns
// Unimplemented to the app, because the gateway in between silently answers with
// the embedded stub. It builds, it deploys, and it fails only in production —
// which is exactly how the paid family-slot RPCs were dark.
//
// Reflection cannot tell a promoted method from a declared one (method values
// are wrappers, so every Pointer() compares equal), so this reads the source and
// checks which methods are actually declared on the proxy.
func TestFamilyAccountsProxyImplementsEveryRPC(t *testing.T) {
	const src = "family_accounts_proxy.go"
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("cannot read %s: %v", src, err)
	}

	declared := map[string]bool{}
	re := regexp.MustCompile(`func \(\w+ \*FamilyAccountsServiceProxy\) (\w+)\(`)
	for _, m := range re.FindAllStringSubmatch(string(body), -1) {
		declared[m[1]] = true
	}
	if len(declared) == 0 {
		t.Fatalf("parsed no methods from %s — the regex no longer matches the file", src)
	}

	// RPCs that deliberately do NOT belong on this gateway. Listed explicitly
	// so the test keeps its teeth: the alternative to naming them is deleting
	// the test the first time it goes red for a good reason.
	notOnThisGateway := map[string]string{
		// Service-internal spend RPCs. core-payments calls accounts-service
		// directly; exposing them through the PUBLIC gateway would let anyone
		// who can reach it authorize and record spend against a family pool.
		"AuthorizeFamilySpend":      "service-internal, called by core-payments",
		"RecordFamilySpend":         "service-internal, called by core-payments",
		"ReleaseFamilySpend":        "service-internal, called by core-payments",
		"RefundFamilySpend":         "service-internal, called by core-payments",
		"GetFamilySpendByReference": "service-internal, called by core-payments",

		// Admin surface lives behind admin-gateway, which does its own role
		// checks. Proxying them here would put ops tooling on the customer path.
		"AdminListFamilyAccounts":       "admin-gateway",
		"AdminGetFamilyAccount":         "admin-gateway",
		"AdminFreezeFamilyAccount":      "admin-gateway",
		"AdminUnfreezeFamilyAccount":    "admin-gateway",
		"AdminDeleteFamilyAccount":      "admin-gateway",
		"AdminForceAllocateFunds":       "admin-gateway",
		"AdminRemoveFamilyMember":       "admin-gateway",
		"AdminUpdateFamilyAccountNotes": "admin-gateway",
		"AdminGetFamilyTransactions":    "admin-gateway",
		"AdminGetFamilyReconciliation":  "admin-gateway",
		"AdminReconcileFamilyAccount":   "admin-gateway",
		"AdminGetFamilyAuditLog":        "admin-gateway",
		"AdminListFamilySlots":          "admin-gateway",
		"AdminListFamilySlotCharges":    "admin-gateway",
	}

	serverIface := reflect.TypeOf((*accountspb.FamilyAccountsServiceServer)(nil)).Elem()

	var missing []string
	for i := 0; i < serverIface.NumMethod(); i++ {
		name := serverIface.Method(i).Name
		if strings.HasPrefix(name, "mustEmbed") {
			continue // generated marker, not an RPC
		}
		if _, skip := notOnThisGateway[name]; skip {
			continue
		}
		if !declared[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Fatalf("%d RPC(s) fall through to the Unimplemented stub and will 501 in "+
			"production even though accounts-service implements them. Add a "+
			"forwarding method to %s:\n  %s",
			len(missing), src, strings.Join(missing, "\n  "))
	}
}

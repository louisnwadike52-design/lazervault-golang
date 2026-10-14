package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A rule naming a service this gateway does not serve matches no method and
// enforces NOTHING, while reading exactly like enforcement. That failure is
// completely silent: no error, no log, no refusal — just a switch that does
// nothing while the dashboard shows it set.
//
// It has already happened once here. On financial-gateway the first rule set
// keyed on ServiceName constants found in the pb package; those types are
// compiled in but not all are registered, so seven of eight rules were dead.
// The guard written alongside them checked only that the prefix LOOKED like
// "/pkg.Service/", which it did.
func TestRulesTargetServedServices(t *testing.T) {
	for _, r := range featureGateRules() {
		if !strings.HasPrefix(r.MethodPrefix, "/") || !strings.HasSuffix(r.MethodPrefix, "/") {
			t.Errorf("rule gate %q has MethodPrefix %q — it must be /pkg.Service/ "+
				"or prefix matching silently never fires", r.Gate, r.MethodPrefix)
			continue
		}
		svc := strings.Trim(r.MethodPrefix, "/")
		if !servedServices[svc] {
			t.Errorf("rule gates %q but this gateway does not serve %s. A rule for "+
				"an unserved service enforces nothing while looking like enforcement.",
				r.Gate, svc)
		}
	}
}

// Guard the guard: a stale servedServices list could quietly bless a rule for a
// service that has since been removed.
func TestServedServicesAreActuallyRegistered(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	var code strings.Builder
	for _, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		code.WriteString(line)
		code.WriteString("\n")
	}
	ms := regexp.MustCompile(`Register(\w+?)Server\(`).FindAllStringSubmatch(code.String(), -1)
	if len(ms) == 0 {
		t.Fatal("no Register*Server calls found — scan broken, assertions vacuous")
	}
	reg := map[string]bool{}
	for _, m := range ms {
		reg[strings.ToLower(m[1])] = true
	}
	for svc := range servedServices {
		short := svc
		if i := strings.LastIndex(svc, "."); i >= 0 {
			short = svc[i+1:]
		}
		if !reg[strings.ToLower(short)] {
			t.Errorf("servedServices lists %s but main.go registers no Register%sServer "+
				"— the list is stale", svc, short)
		}
	}
}

// The gate id is a string shared with the Dart enum and the admin dashboard,
// and nothing makes them agree at compile time. A drifted id gates on a key the
// dashboard never writes, so the switch appears broken rather than misnamed.
func TestQuickServiceGateIdsExistInTheAppEnum(t *testing.T) {
	const enumPath = "../../lazervaultapp/lib/core/types/services.dart"
	raw, err := os.ReadFile(enumPath)
	if err != nil {
		t.Skipf("app enum not available (%v)", err)
	}
	body := string(raw)
	start := strings.Index(body, "enum AppServiceName {")
	if start < 0 {
		t.Fatal("AppServiceName enum not found")
	}
	end := strings.Index(body[start:], "}")
	known := map[string]bool{}
	ident := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*$`)
	for _, line := range strings.Split(body[start:start+end], "\n") {
		v := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
		if v == "" || strings.HasPrefix(v, "//") || strings.HasPrefix(v, "enum ") {
			continue
		}
		if ident.MatchString(v) {
			known[v] = true
		}
	}
	if len(known) == 0 {
		t.Fatal("parsed no enum values — scan broken, assertions vacuous")
	}
	for _, r := range featureGateRules() {
		if !strings.HasPrefix(r.Gate, "quick_service_") {
			continue
		}
		svc := strings.TrimPrefix(r.Gate, "quick_service_")
		if !known[svc] {
			t.Errorf("rule %q gates quick_service_%s, which is NOT an AppServiceName — "+
				"the dashboard builds its keys from that enum, so this gate would never "+
				"be written and the switch would appear to do nothing",
				r.MethodPrefix, svc)
		}
	}
}

// Reads must stay open unless a rule deliberately opts in: withdrawing a
// product must not hide what a user already holds.
func TestNoRuleGatesReadsWithoutIntent(t *testing.T) {
	for _, r := range featureGateRules() {
		if r.GateReads {
			t.Errorf("rule %q sets GateReads — that hides balances and history from "+
				"their owner. Remove it unless the read IS the product.", r.MethodPrefix)
		}
	}
}

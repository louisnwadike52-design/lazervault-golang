package main

import (
	"context"
	"os"

	authinterceptor "github.com/lazervault/shared/auth-interceptor"
	"lazervault-app-platform/microservices/shared/featuregate"
)

// Server-side enforcement of the admin feature gates for the services this
// gateway fronts.
//
// The Flutter gates close the app's doors and nothing else: the chat agents,
// the voice agents, an already-installed older build and anything holding a
// token reach these RPCs directly. A switch that only hides a tile is one an
// operator will believe in and that does not hold.
//
// LazerPoints is served by referral-service, and the Family & Friends pot by the
// family accounts service — both front gates the app already renders.
//
// MAPPED BY SERVICE so a new RPC inherits its service's switch rather than
// escaping it until somebody lists it. READS ARE EXEMPT by default
// (featuregate.Rule.GateReads) — withdrawing a product must stop new activity,
// never hide what a user already holds.
//
// The prefixes below are the reflection list from the RUNNING server, not names
// read out of the proto files. Compiled-in and registered are different sets:
// keying on the former once left seven of eight rules matching no method at
// all, enforcing nothing while reading exactly like enforcement.
//
//	grpcurl -plaintext 127.0.0.1:60070 list
func featureGateRules() []featuregate.Rule {
	return []featuregate.Rule{
		{MethodPrefix: "/pb.ReferralService/", Gate: "feature_lifestyle_lazerpoints"},
		{MethodPrefix: "/accounts.v1.FamilyAccountsService/", Gate: "feature_account_family"},
	}
}

// servedServices is that same reflection list, pinned so a rule naming a
// service this gateway does not serve fails the build instead of going quiet.
var servedServices = map[string]bool{
	"accounts.AccountsService":            true,
	"accounts.MultiCountryAccountService": true,
	"accounts.v1.FamilyAccountsService":   true,
	"notifications.NotificationsService":  true,
	"pb.AIChatService":                    true,
	"pb.AuthService":                      true,
	"pb.ContactSyncService":               true,
	"pb.ReferralService":                  true,
	"pb.TransactionPinService":            true,
	"pb.UserService":                      true,
	"pb.VoiceSessionService":              true,
	"recipient.RecipientService":          true,
}

// emailFromContext reads the caller's address for the whitelist override.
//
// "" on any failure is NOT a refusal — it means no whitelist can match and the
// gate falls back to its switch. Service-to-service calls legitimately carry no
// user, and denying those would break every internal caller the moment a gate
// was switched off.
func emailFromContext(ctx context.Context) string {
	email, err := authinterceptor.GetUserEmail(ctx)
	if err != nil {
		return ""
	}
	return email
}

// newFeatureGateCache polls admin-gateway's internal settings endpoint — the
// SAME snapshot the Flutter app reads, so the server and the UI cannot disagree
// about whether something is switched off. Nothing blocks on it: until the
// first poll lands every gate resolves to ALLOWED.
func newFeatureGateCache(ctx context.Context) *featuregate.Cache {
	url := os.Getenv("FEATURE_GATE_SETTINGS_URL")
	if url == "" {
		url = "http://127.0.0.1:18096/api/v1/internal/voice-agents/settings"
	}
	c := featuregate.NewCache(url)
	c.Start(ctx, 0)
	return c
}

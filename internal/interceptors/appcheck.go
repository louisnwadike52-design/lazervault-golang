package interceptors

import (
	"context"

	"github.com/lazervault/shared/auth-interceptor/appcheck"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AppCheckMode controls how the App Check interceptor behaves.
type AppCheckMode string

const (
	// AppCheckOff disables App Check entirely (pass-through).
	AppCheckOff AppCheckMode = "off"
	// AppCheckReport verifies the token and records a verdict but NEVER blocks —
	// the safe default during client rollout. Invalid/missing tokens are logged
	// + metered so we can watch adoption before enforcing.
	AppCheckReport AppCheckMode = "report"
	// AppCheckEnforce rejects EVERY non-public call that lacks a valid App Check
	// token (strictest — requires attestation on every request).
	AppCheckEnforce AppCheckMode = "enforce"
	// AppCheckEnforceAuth rejects only the token-ISSUING calls (login/signup)
	// that lack a valid token, while merely reporting on everything else. This is
	// the recommended enforcement level: a JWT is only ever minted for an
	// attested device, so every downstream JWT-authed call is transitively from
	// an attested device — without having to attest each individual request.
	AppCheckEnforceAuth AppCheckMode = "enforce_auth"
)

// authTokenIssuingMethods are the gRPC methods that MINT a NEW session — they
// accept credentials or create an account and return a token. Gating App Check
// HERE means the JWT chain is rooted in an attested device. Step-up finishers
// (VerifyLoginOtp / VerifyTwoFactor) are intentionally EXCLUDED: they require
// the temp token from an already-attested Login, so they're transitively gated
// and don't carry the App Check header themselves. The Flutter client attaches
// x-firebase-appcheck on exactly these methods (GrpcCallOptionsHelper.withAppCheck).
var authTokenIssuingMethods = map[string]bool{
	"/pb.AuthService/Login":                  true,
	"/pb.AuthService/LoginWithPasscode":      true,
	"/pb.AuthService/LoginWithPhonePasscode": true,
	"/pb.AuthService/Signup":                 true,
	"/pb.AuthService/SignupWithPhone":        true,
	"/pb.AuthService/RequestSignupPhoneOTP":  true,
	"/pb.AuthService/VerifySignupPhoneOTP":   true,
}

// ParseAppCheckMode normalises a config string into an AppCheckMode (defaults to
// report, the backwards-compatible setting).
func ParseAppCheckMode(s string) AppCheckMode {
	switch AppCheckMode(s) {
	case AppCheckOff:
		return AppCheckOff
	case AppCheckEnforce:
		return AppCheckEnforce
	case AppCheckEnforceAuth:
		return AppCheckEnforceAuth
	default:
		return AppCheckReport
	}
}

// appCheckMetadataKey is the lowercase gRPC metadata key the Flutter client
// sends (see GrpcCallOptionsHelper.withAuth). HTTP requests send the
// `X-Firebase-AppCheck` header which grpc-gateway maps to this key.
const appCheckMetadataKey = "x-firebase-appcheck"

type appCheckVerdictKey struct{}

// AppCheckVerdict is stashed in the request context so downstream handlers /
// the auth-service can incorporate the attestation result into risk scoring.
type AppCheckVerdict struct {
	Present bool
	Valid   bool
	AppID   string
	Err     string
}

// AppCheckVerdictFromContext returns the recorded verdict, if any.
func AppCheckVerdictFromContext(ctx context.Context) (AppCheckVerdict, bool) {
	v, ok := ctx.Value(appCheckVerdictKey{}).(AppCheckVerdict)
	return v, ok
}

// AppCheckInterceptor verifies the Firebase App Check token attached to a call.
//
// Behaviour by mode:
//   - off:     pass-through, no verdict recorded.
//   - report:  verify, record verdict, log on missing/invalid, NEVER block.
//   - enforce: on non-public methods, reject calls without a valid token.
//
// The verdict is always stashed in context so the risk engine can use it even
// in report mode. A nil verifier degrades to pass-through (fail-open) so a
// misconfiguration can never lock users out.
// modeFn resolves the active mode per call (admin-tunable, cached). A nil modeFn
// is treated as AppCheckReport.
func AppCheckInterceptor(verifier *appcheck.Verifier, modeFn func() AppCheckMode, logger *zap.Logger) grpc.UnaryServerInterceptor {
	if logger == nil {
		logger = zap.NewNop()
	}
	if modeFn == nil {
		modeFn = func() AppCheckMode { return AppCheckReport }
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		mode := modeFn()
		if mode == AppCheckOff || verifier == nil {
			return handler(ctx, req)
		}

		verdict := AppCheckVerdict{}
		var token string
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(appCheckMetadataKey); len(vals) > 0 {
				token = vals[0]
			}
		}
		verdict.Present = token != ""

		if verdict.Present {
			claims, err := verifier.Verify(ctx, token)
			if err != nil {
				verdict.Err = err.Error()
			} else {
				verdict.Valid = true
				verdict.AppID = claims.AppID
			}
		}

		ctx = context.WithValue(ctx, appCheckVerdictKey{}, verdict)

		// Decide whether THIS call must be blocked when unattested:
		//   - enforce:      every non-public call.
		//   - enforce_auth: only the token-issuing (login/signup) calls — the
		//                   JWT they mint then transitively proves attestation
		//                   for all downstream calls.
		//   - report/off:   never block.
		// We do NOT skip public auth endpoints — login/signup are exactly the
		// highest-value targets we want attested.
		enforceThis := mode == AppCheckEnforce ||
			(mode == AppCheckEnforceAuth && authTokenIssuingMethods[info.FullMethod])

		if enforceThis && !verdict.Valid {
			logger.Warn("app check: rejecting unattested call",
				zap.String("mode", string(mode)),
				zap.String("method", info.FullMethod),
				zap.Bool("present", verdict.Present),
				zap.String("err", verdict.Err))
			return nil, status.Error(codes.Unauthenticated, "device attestation required")
		}

		if !verdict.Valid {
			logger.Info("app check: unattested call (not enforced)",
				zap.String("mode", string(mode)),
				zap.String("method", info.FullMethod),
				zap.Bool("present", verdict.Present),
				zap.String("err", verdict.Err))
		}

		return handler(ctx, req)
	}
}

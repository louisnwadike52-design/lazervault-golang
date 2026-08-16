package interceptors

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const appCheckModeTTL = 60 * time.Second

// AppCheckModeProvider resolves the App Check enforcement mode (off/report/
// enforce) from admin-gateway's unauthenticated internal settings endpoint
// (system_settings key auth_appcheck_mode), with a 60s cache and an env-derived
// fallback (AUTH_APPCHECK_MODE). Fail-safe: on any error it serves the last good
// value, or the fallback — so the gateway never hard-depends on admin-gateway.
type AppCheckModeProvider struct {
	baseURL  string
	client   *http.Client
	fallback AppCheckMode
	logger   *zap.Logger

	mu       sync.RWMutex
	cached   AppCheckMode
	cachedAt time.Time
	hasValue bool
}

// NewAppCheckModeProvider builds the provider. fallback is the env-derived
// default used until a value is read. A blank ADMIN_GATEWAY_URL disables remote
// reads (the provider always returns the fallback).
func NewAppCheckModeProvider(fallback AppCheckMode, logger *zap.Logger) *AppCheckModeProvider {
	base := strings.TrimRight(os.Getenv("ADMIN_GATEWAY_URL"), "/")
	if base == "" {
		base = "http://localhost:8096"
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AppCheckModeProvider{
		baseURL:  base,
		client:   &http.Client{Timeout: 4 * time.Second},
		fallback: fallback,
		logger:   logger,
	}
}

// Mode returns the active App Check mode, refreshing from admin-gateway when the
// cache has expired. Cheap on the hot path (cached); safe to call per request.
func (p *AppCheckModeProvider) Mode() AppCheckMode {
	p.mu.RLock()
	if p.hasValue && time.Since(p.cachedAt) < appCheckModeTTL {
		v := p.cached
		p.mu.RUnlock()
		return v
	}
	p.mu.RUnlock()

	mode, err := p.fetch(context.Background())
	if err != nil {
		p.mu.RLock()
		defer p.mu.RUnlock()
		if p.hasValue {
			return p.cached
		}
		return p.fallback
	}

	p.mu.Lock()
	p.cached = mode
	p.cachedAt = time.Now()
	p.hasValue = true
	p.mu.Unlock()
	return mode
}

func (p *AppCheckModeProvider) fetch(ctx context.Context) (AppCheckMode, error) {
	url := p.baseURL + "/api/v1/internal/voice-agents/settings/auth_appcheck_mode"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errAppCheckBadStatus
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if strings.TrimSpace(body.Value) == "" {
		return p.fallback, nil // key unset → keep the env fallback
	}
	return ParseAppCheckMode(body.Value), nil
}

type appCheckErr struct{ msg string }

func (e *appCheckErr) Error() string { return e.msg }

var errAppCheckBadStatus = &appCheckErr{"non-200 from admin gateway"}

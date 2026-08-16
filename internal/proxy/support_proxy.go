package proxy

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// SupportProxy reverse-proxies the Flutter app's user-facing support requests
// (/api/v1/support/...) to support-service. support-service validates the JWT
// itself (JWKS/ES256) and owns all ticket/chat state, so we forward the
// caller's Authorization header verbatim — the same thin-proxy shape
// admin-gateway uses for the admin support surface (see
// services/admin-gateway/internal/handlers/support_admin.go).
//
// This keeps the app on the platform rule that the frontend NEVER calls a
// microservice directly: Contact support now reaches support-service through
// core-gateway on 443 instead of a raw :8030 port that isn't publicly exposed.
type SupportProxy struct {
	baseURL string // e.g. http://127.0.0.1:8030
	client  *http.Client
}

// NewSupportProxy builds a proxy targeting support-service's HTTP base URL.
func NewSupportProxy(baseURL string) *SupportProxy {
	return &SupportProxy{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

// Handle forwards the incoming request (method, full path, query, body, and the
// Authorization header) to support-service and streams the response back. The
// gateway's JWTAuthMiddleware has already validated the token before this runs;
// support-service re-validates and derives the user from it.
func (p *SupportProxy) Handle(c *gin.Context) {
	target := fmt.Sprintf("%s%s", p.baseURL, c.Request.URL.Path)
	if raw := c.Request.URL.RawQuery; raw != "" {
		target += "?" + raw
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, target, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "build request failed"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if auth := c.GetHeader("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		log.Error().Err(err).Str("path", c.Request.URL.Path).Msg("support proxy: upstream unreachable")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "support service unavailable", "code": "SUPPORT_UNAVAILABLE"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", body)
}

package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HTTPProxy handles proxying HTTP requests to HTTP microservices
type HTTPProxy struct {
	client *http.Client
}

// NewHTTPProxy creates a new HTTP proxy
func NewHTTPProxy() *HTTPProxy {
	return &HTTPProxy{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ProxyHTTPRequest forwards an HTTP request to a microservice
func (p *HTTPProxy) ProxyHTTPRequest(c *gin.Context, targetURL string) {
	// Read request body
	var bodyBytes []byte
	if c.Request.Body != nil {
		bodyBytes, _ = io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Create new request to microservice
	req, err := http.NewRequestWithContext(
		c.Request.Context(),
		c.Request.Method,
		targetURL,
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request"})
		return
	}

	// Copy headers (especially Authorization)
	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Make request to microservice
	resp, err := p.client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Microservice unavailable: %v", err)})
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	// Copy response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read microservice response"})
		return
	}

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), responseBody)
}

// CreateProxyHandler creates a Gin handler that proxies to a microservice
func (p *HTTPProxy) CreateProxyHandler(serviceBaseURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Build target URL: serviceBaseURL + request path
		targetURL := serviceBaseURL + c.Request.URL.Path
		if c.Request.URL.RawQuery != "" {
			targetURL += "?" + c.Request.URL.RawQuery
		}

		p.ProxyHTTPRequest(c, targetURL)
	}
}

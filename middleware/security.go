package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// SecurityHeaders adds security headers to responses
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// XSS Protection
		c.Header("X-XSS-Protection", "1; mode=block")

		// Referrer Policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy - strict policy for production
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data: https:; " +
			"font-src 'self' data:; " +
			"connect-src 'self'; " +
			"frame-ancestors 'none';"
		c.Header("Content-Security-Policy", csp)

		// HSTS - Force HTTPS (only in production with HTTPS)
		// c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")

		// Permissions Policy - restrict browser features
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		c.Next()
	}
}

// CSRF middleware provides CSRF protection
type CSRFConfig struct {
	TokenLength int
	CookieName  string
	HeaderName  string
	Secret      string
}

// DefaultCSRFConfig returns default CSRF configuration
func DefaultCSRFConfig() *CSRFConfig {
	return &CSRFConfig{
		TokenLength: 32,
		CookieName:  "csrf_token",
		HeaderName:  "X-CSRF-Token",
		Secret:      uuid.New().String(), // In production, load from secure config
	}
}

// CSRF creates a CSRF protection middleware
func CSRF(config *CSRFConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultCSRFConfig()
	}

	return func(c *gin.Context) {
		// Skip CSRF for safe methods
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}

		// Get token from header
		token := c.GetHeader(config.HeaderName)

		// Get token from cookie
		cookieToken, err := c.Cookie(config.CookieName)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "CSRF token missing"})
			c.Abort()
			return
		}

		// Compare tokens using constant-time comparison
		if !secureCompare(token, cookieToken) {
			c.JSON(http.StatusForbidden, gin.H{"error": "CSRF token mismatch"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// secureCompare performs constant-time string comparison
func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// RateLimiter stores rate limiters per IP
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
	}
}

// GetLimiter returns a rate limiter for an IP address
func (rl *RateLimiter) GetLimiter(ip string) *rate.Limiter {
	if limiter, exists := rl.limiters[ip]; exists {
		return limiter
	}

	limiter := rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[ip] = limiter
	return limiter
}

// RateLimitMiddleware creates a rate limiting middleware
// Example: RateLimitMiddleware(100, 200) = 100 requests per second with burst of 200
func RateLimitMiddleware(requestsPerSecond int, burst int) gin.HandlerFunc {
	limiter := NewRateLimiter(rate.Limit(requestsPerSecond), burst)

	return func(c *gin.Context) {
		// Get client IP
		ip := c.ClientIP()

		// Get or create limiter for this IP
		l := limiter.GetLimiter(ip)

		if !l.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"retry_after": time.Second.Seconds(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// SQLInjectionProtection checks for common SQL injection patterns
func SQLInjectionProtection() gin.HandlerFunc {
	// Common SQL injection patterns
	sqlPatterns := []string{
		"'", "\"", "--", "/*", "*/", "xp_", "sp_",
		"OR 1=1", "OR '1'='1'", "UNION SELECT", "DROP TABLE",
		"INSERT INTO", "DELETE FROM", "UPDATE ", "EXEC(",
		"EXECUTE(", ";--", "';", "\";",
	}

	return func(c *gin.Context) {
		// Check query parameters
		for _, param := range c.Request.URL.Query() {
			for _, value := range param {
				if containsSQLInjection(value, sqlPatterns) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input detected"})
					c.Abort()
					return
				}
			}
		}

		// Check form data
		if c.Request.Method == "POST" || c.Request.Method == "PUT" {
			c.Request.ParseForm()
			for _, values := range c.Request.PostForm {
				for _, value := range values {
					if containsSQLInjection(value, sqlPatterns) {
						c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input detected"})
						c.Abort()
						return
					}
				}
			}
		}

		c.Next()
	}
}

// containsSQLInjection checks if a string contains SQL injection patterns
func containsSQLInjection(input string, patterns []string) bool {
	upperInput := strings.ToUpper(input)
	for _, pattern := range patterns {
		if strings.Contains(upperInput, strings.ToUpper(pattern)) {
			return true
		}
	}
	return false
}

// XSSProtection sanitizes input to prevent XSS attacks
func XSSProtection() gin.HandlerFunc {
	xssPatterns := []string{
		"<script", "</script>", "javascript:", "onerror=",
		"onload=", "onclick=", "<iframe", "eval(",
		"alert(", "document.cookie", "document.write",
	}

	return func(c *gin.Context) {
		// Check query parameters
		for _, param := range c.Request.URL.Query() {
			for _, value := range param {
				if containsXSS(value, xssPatterns) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input detected"})
					c.Abort()
					return
				}
			}
		}

		c.Next()
	}
}

// containsXSS checks if input contains XSS patterns
func containsXSS(input string, patterns []string) bool {
	lowerInput := strings.ToLower(input)
	for _, pattern := range patterns {
		if strings.Contains(lowerInput, pattern) {
			return true
		}
	}
	return false
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           time.Duration
}

// DefaultCORSConfig returns secure default CORS configuration
func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowOrigins:     []string{}, // Must be explicitly set for production
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-CSRF-Token"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
}

// CORS creates a CORS middleware
func CORS(config *CORSConfig) gin.HandlerFunc {
	if config == nil {
		config = DefaultCORSConfig()
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowedOrigin := ""
		for _, allowed := range config.AllowOrigins {
			if allowed == "*" || allowed == origin {
				allowedOrigin = allowed
				break
			}
		}

		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
			c.Header("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ", "))
			c.Header("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ", "))
			c.Header("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ", "))
			c.Header("Access-Control-Max-Age", config.MaxAge.String())

			if config.AllowCredentials {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// RequestID adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		c.Set("RequestID", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// ============================================================================
// HTTP HANDLER VERSIONS FOR GRPC GATEWAY
// ============================================================================

// ChainMiddleware chains multiple HTTP middleware functions
func ChainMiddleware(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	// Apply middleware in reverse order so they execute in the order provided
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// RequestIDHTTP adds a unique request ID to each HTTP request
func RequestIDHTTP() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			w.Header().Set("X-Request-ID", requestID)
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersHTTP adds security headers to HTTP responses
func SecurityHeadersHTTP() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prevent clickjacking
			w.Header().Set("X-Frame-Options", "DENY")

			// Prevent MIME type sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// XSS Protection
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Referrer Policy
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content Security Policy
			csp := "default-src 'self'; " +
				"script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
				"style-src 'self' 'unsafe-inline'; " +
				"img-src 'self' data: https:; " +
				"font-src 'self' data:; " +
				"connect-src 'self'; " +
				"frame-ancestors 'none';"
			w.Header().Set("Content-Security-Policy", csp)

			// Permissions Policy
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMiddlewareHTTP provides rate limiting for HTTP requests
func RateLimitMiddlewareHTTP(requestsPerSecond int, burst int) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(rate.Limit(requestsPerSecond), burst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			// Extract IP without port
			if strings.Contains(ip, ":") {
				ip = strings.Split(ip, ":")[0]
			}

			l := limiter.GetLimiter(ip)

			if !l.Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"Rate limit exceeded","retry_after":1}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// XSSProtectionHTTP prevents XSS attacks in HTTP requests
func XSSProtectionHTTP() func(http.Handler) http.Handler {
	xssPatterns := []string{
		"<script", "</script>", "javascript:", "onerror=",
		"onload=", "onclick=", "<iframe", "eval(",
		"alert(", "document.cookie", "document.write",
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for XSS patterns in query parameters
			for _, param := range r.URL.Query() {
				for _, value := range param {
					if containsXSS(value, xssPatterns) {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"error":"Potential XSS attack detected"}`))
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SQLInjectionProtectionHTTP prevents SQL injection attacks in HTTP requests
func SQLInjectionProtectionHTTP() func(http.Handler) http.Handler {
	sqlPatterns := []string{
		"'", "\"", "--", "/*", "*/", "xp_", "sp_",
		"OR 1=1", "OR '1'='1'", "UNION SELECT", "DROP TABLE",
		"INSERT INTO", "DELETE FROM", "UPDATE ", "EXEC(",
		"EXECUTE(", ";--", "';", "\";",
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for SQL injection patterns in query parameters
			for _, param := range r.URL.Query() {
				for _, value := range param {
					if containsSQLInjection(value, sqlPatterns) {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						w.Write([]byte(`{"error":"Potential SQL injection detected"}`))
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

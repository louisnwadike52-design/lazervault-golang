package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PanicRecoveryMiddleware recovers from panics and returns proper error responses
func PanicRecoveryMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
				)
				c.JSON(500, gin.H{
					"error": "Internal server error",
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}

// RequestLoggingMiddleware logs all incoming requests
func RequestLoggingMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		logger.Info("HTTP request",
			zap.String("method", method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", statusCode),
			zap.String("ip", clientIP),
			zap.Duration("latency", latency),
		)
	}
}

// RequestID adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		c.Set("RequestID", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// generateRequestID generates a simple request ID
func generateRequestID() string {
	return uuid.New().String()
}

// TimeoutMiddleware handles request timeouts
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Set timeout on context
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		// Check if context was canceled due to timeout
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				c.JSON(408, gin.H{
					"error": "Request timeout",
				})
				c.Abort()
			}
		default:
		}
	}
}

// ErrorHandlerMiddleware handles errors and returns proper error responses
func ErrorHandlerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check if there were any errors
		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			logger.Error("Request error",
				zap.String("path", c.Request.URL.Path),
				zap.Error(err.Err),
			)

			c.JSON(500, gin.H{
				"error": err.Error(),
			})
		}
	}
}

// RateLimitMiddleware implements rate limiting (simple in-memory version)
func RateLimitMiddleware(requestsPerMinute int, burst int) gin.HandlerFunc {
	// Simple rate limiter - in production, use Redis or a dedicated rate limiting library
	return func(c *gin.Context) {
		// TODO: Implement proper rate limiting with Redis
		c.Next()
	}
}

// HealthCheckMiddleware provides health check endpoint
func HealthCheckMiddleware(checks map[string]func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		statusCode := 200
		status := "ok"
		details := make(map[string]string)

		for name, check := range checks {
			if err := check(); err != nil {
				statusCode = 503
				status = "unhealthy"
				details[name] = err.Error()
			} else {
				details[name] = "ok"
			}
		}

		c.JSON(statusCode, gin.H{
			"status":  status,
			"service": "core-gateway",
			"checks":  details,
		})
	}
}

// ReadinessCheckMiddleware provides readiness check endpoint
func ReadinessCheckMiddleware(isReady func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isReady() {
			c.JSON(200, gin.H{
				"status": "ready",
			})
		} else {
			c.JSON(503, gin.H{
				"status": "not_ready",
			})
		}
	}
}

// SecurityHeaders adds security headers to responses
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Next()
	}
}

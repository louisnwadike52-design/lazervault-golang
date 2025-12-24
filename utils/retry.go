package utils

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/sirupsen/logrus"
)

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Logger         *logrus.Logger
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		Multiplier:     2.0,
		Logger:         logrus.New(),
	}
}

// RetryableFunc is a function that can be retried
type RetryableFunc func() error

// RetryableFunc With Context
type RetryableFuncWithContext func(ctx context.Context) error

// RetryWithExponentialBackoff retries a function with exponential backoff
func RetryWithExponentialBackoff(fn RetryableFunc, config *RetryConfig) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	backoff := config.InitialBackoff

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff
			backoff = time.Duration(float64(config.InitialBackoff) * math.Pow(config.Multiplier, float64(attempt-1)))
			if backoff > config.MaxBackoff {
				backoff = config.MaxBackoff
			}

			if config.Logger != nil {
				config.Logger.WithFields(logrus.Fields{
					"attempt": attempt,
					"backoff": backoff.String(),
				}).Info("retrying operation")
			}

			time.Sleep(backoff)
		}

		err := fn()
		if err == nil {
			if attempt > 0 && config.Logger != nil {
				config.Logger.WithField("attempts", attempt+1).Info("operation succeeded after retry")
			}
			return nil
		}

		lastErr = err
		if config.Logger != nil {
			config.Logger.WithFields(logrus.Fields{
				"attempt": attempt + 1,
				"error":   err.Error(),
			}).Warn("operation failed")
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// RetryWithContext retries a function with context and exponential backoff
func RetryWithContext(ctx context.Context, fn RetryableFuncWithContext, config *RetryConfig) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	backoff := config.InitialBackoff

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation cancelled: %w", ctx.Err())
		default:
		}

		if attempt > 0 {
			// Calculate exponential backoff
			backoff = time.Duration(float64(config.InitialBackoff) * math.Pow(config.Multiplier, float64(attempt-1)))
			if backoff > config.MaxBackoff {
				backoff = config.MaxBackoff
			}

			if config.Logger != nil {
				config.Logger.WithFields(logrus.Fields{
					"attempt": attempt,
					"backoff": backoff.String(),
				}).Info("retrying operation with context")
			}

			// Sleep with context cancellation support
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return fmt.Errorf("operation cancelled during backoff: %w", ctx.Err())
			}
		}

		err := fn(ctx)
		if err == nil {
			if attempt > 0 && config.Logger != nil {
				config.Logger.WithField("attempts", attempt+1).Info("operation succeeded after retry")
			}
			return nil
		}

		lastErr = err
		if config.Logger != nil {
			config.Logger.WithFields(logrus.Fields{
				"attempt": attempt + 1,
				"error":   err.Error(),
			}).Warn("operation with context failed")
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// IsRetryableError checks if an error should trigger a retry
// Add custom logic here for specific error types
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Add specific error type checks here
	// For example: network errors, timeout errors, 5xx HTTP errors, etc.

	// For now, retry on any error (can be customized)
	return true
}

// RetryableHTTPFunc is a function that returns an HTTP status code and error
type RetryableHTTPFunc func() (int, error)

// RetryHTTP retries HTTP operations, only retrying on 5xx errors and network issues
func RetryHTTP(fn RetryableHTTPFunc, config *RetryConfig) (int, error) {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	var statusCode int
	backoff := config.InitialBackoff

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff = time.Duration(float64(config.InitialBackoff) * math.Pow(config.Multiplier, float64(attempt-1)))
			if backoff > config.MaxBackoff {
				backoff = config.MaxBackoff
			}

			if config.Logger != nil {
				config.Logger.WithFields(logrus.Fields{
					"attempt":     attempt,
					"backoff":     backoff.String(),
					"status_code": statusCode,
				}).Info("retrying HTTP operation")
			}

			time.Sleep(backoff)
		}

		code, err := fn()
		statusCode = code

		// Success case
		if err == nil && (code >= 200 && code < 300) {
			if attempt > 0 && config.Logger != nil {
				config.Logger.WithField("attempts", attempt+1).Info("HTTP operation succeeded after retry")
			}
			return statusCode, nil
		}

		// Don't retry on 4xx errors (client errors) - these won't succeed on retry
		if code >= 400 && code < 500 {
			if config.Logger != nil {
				config.Logger.WithFields(logrus.Fields{
					"status_code": code,
					"error":       err,
				}).Warn("HTTP operation failed with client error - not retrying")
			}
			return statusCode, err
		}

		// Retry on 5xx errors (server errors) or network errors
		lastErr = err
		if config.Logger != nil {
			config.Logger.WithFields(logrus.Fields{
				"attempt":     attempt + 1,
				"status_code": code,
				"error":       err,
			}).Warn("HTTP operation failed - will retry")
		}
	}

	return statusCode, fmt.Errorf("HTTP operation failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

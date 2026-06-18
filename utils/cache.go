package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// CacheClient wraps Redis client with common caching operations
type CacheClient struct {
	client *redis.Client
	logger *logrus.Logger
}

// NewCacheClient creates a new cache client
func NewCacheClient(redisURL string, logger *logrus.Logger) (*CacheClient, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	if logger == nil {
		logger = logrus.New()
	}

	logger.Info("Successfully connected to Redis")

	return &CacheClient{
		client: client,
		logger: logger,
	}, nil
}

// Set stores a value in cache with TTL
func (c *CacheClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	err = c.client.Set(ctx, key, data, ttl).Err()
	if err != nil {
		c.logger.WithFields(logrus.Fields{
			"key":   key,
			"error": err.Error(),
		}).Error("failed to set cache")
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

// Get retrieves a value from cache
func (c *CacheClient) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return fmt.Errorf("cache miss: key not found")
	}
	if err != nil {
		c.logger.WithFields(logrus.Fields{
			"key":   key,
			"error": err.Error(),
		}).Error("failed to get cache")
		return fmt.Errorf("failed to get cache: %w", err)
	}

	err = json.Unmarshal(data, dest)
	if err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// Delete removes a key from cache
func (c *CacheClient) Delete(ctx context.Context, keys ...string) error {
	err := c.client.Del(ctx, keys...).Err()
	if err != nil {
		c.logger.WithFields(logrus.Fields{
			"keys":  keys,
			"error": err.Error(),
		}).Error("failed to delete cache keys")
		return fmt.Errorf("failed to delete cache: %w", err)
	}

	return nil
}

// Exists checks if a key exists in cache
func (c *CacheClient) Exists(ctx context.Context, key string) (bool, error) {
	count, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return count > 0, nil
}

// Increment atomically increments a counter
func (c *CacheClient) Increment(ctx context.Context, key string) (int64, error) {
	val, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment: %w", err)
	}

	return val, nil
}

// SetWithExpiry sets a key with expiry using SET NX (only if not exists)
func (c *CacheClient) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("failed to marshal value: %w", err)
	}

	set, err := c.client.SetNX(ctx, key, data, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to set NX: %w", err)
	}

	return set, nil
}

// GetOrSet retrieves from cache, or sets if not found (lazy loading)
func (c *CacheClient) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetchFunc func() (interface{}, error), dest interface{}) error {
	// Try to get from cache first
	err := c.Get(ctx, key, dest)
	if err == nil {
		// Cache hit
		return nil
	}

	// Cache miss - fetch from source
	value, err := fetchFunc()
	if err != nil {
		return fmt.Errorf("failed to fetch value: %w", err)
	}

	// Store in cache for next time
	if err := c.Set(ctx, key, value, ttl); err != nil {
		// Log error but don't fail - we have the value
		c.logger.WithFields(logrus.Fields{
			"key":   key,
			"error": err.Error(),
		}).Warn("failed to update cache after fetch")
	}

	// Return the fetched value
	return json.Unmarshal(mustMarshal(value), dest)
}

// InvalidatePattern deletes all keys matching a pattern
func (c *CacheClient) InvalidatePattern(ctx context.Context, pattern string) error {
	var cursor uint64
	var keys []string

	for {
		var scanKeys []string
		var err error

		scanKeys, cursor, err = c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan keys: %w", err)
		}

		keys = append(keys, scanKeys...)

		if cursor == 0 {
			break
		}
	}

	if len(keys) > 0 {
		return c.Delete(ctx, keys...)
	}

	return nil
}

// GetMulti retrieves multiple keys at once
func (c *CacheClient) GetMulti(ctx context.Context, keys []string) (map[string]interface{}, error) {
	if len(keys) == 0 {
		return make(map[string]interface{}), nil
	}

	values, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get multiple keys: %w", err)
	}

	result := make(map[string]interface{})
	for i, key := range keys {
		if values[i] != nil {
			result[key] = values[i]
		}
	}

	return result, nil
}

// SetMulti sets multiple key-value pairs at once
func (c *CacheClient) SetMulti(ctx context.Context, items map[string]interface{}, ttl time.Duration) error {
	pipe := c.client.Pipeline()

	for key, value := range items {
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value for key %s: %w", key, err)
		}

		pipe.Set(ctx, key, data, ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute pipeline: %w", err)
	}

	return nil
}

// Close closes the Redis connection
func (c *CacheClient) Close() error {
	return c.client.Close()
}

// CacheKey generates a standardized cache key
func CacheKey(parts ...string) string {
	key := ""
	for i, part := range parts {
		if i > 0 {
			key += ":"
		}
		key += part
	}
	return key
}

// Common TTL durations
const (
	TTLShort  = 5 * time.Minute // For frequently changing data
	TTLMedium = 1 * time.Hour   // For moderately stable data
	TTLLong   = 24 * time.Hour  // For rarely changing data
	TTLWeek   = 7 * 24 * time.Hour
)

// mustMarshal marshals a value and panics on error (for internal use)
func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// LazyCache provides a simple lazy-loading cache wrapper
type LazyCache struct {
	cache *CacheClient
	ttl   time.Duration
}

// NewLazyCache creates a new lazy cache
func NewLazyCache(cache *CacheClient, ttl time.Duration) *LazyCache {
	return &LazyCache{
		cache: cache,
		ttl:   ttl,
	}
}

// GetOrFetch gets from cache or fetches and stores
func (lc *LazyCache) GetOrFetch(ctx context.Context, key string, fetchFunc func() (interface{}, error), dest interface{}) error {
	return lc.cache.GetOrSet(ctx, key, lc.ttl, fetchFunc, dest)
}

// Invalidate removes a key from cache
func (lc *LazyCache) Invalidate(ctx context.Context, keys ...string) error {
	return lc.cache.Delete(ctx, keys...)
}

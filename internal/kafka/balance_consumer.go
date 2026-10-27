package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	kafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"

	"lazervaultGo/grpcApi/middleware"
)

// BalanceChangedEvent represents a balance change event from accounts-service
type BalanceChangedEvent struct {
	AccountID        string    `json:"account_id"`
	UserID           string    `json:"user_id"`
	AccountNumber    string    `json:"account_number"`
	TransactionID    string    `json:"transaction_id"`
	TransactionType  string    `json:"transaction_type"` // credit, debit
	Amount           float64   `json:"amount"`
	BalanceBefore    float64   `json:"balance_before"`
	BalanceAfter     float64   `json:"balance_after"`
	AvailableBalance float64   `json:"available_balance"`
	Description      string    `json:"description"`
	Timestamp        time.Time `json:"timestamp"`
}

// BalanceCacheInvalidator consumes balance-changed events and invalidates cache
type BalanceCacheInvalidator struct {
	reader      *kafka.Reader
	redisClient *redis.Client
	logger      *zap.Logger
	done        chan struct{}
}

// NewBalanceCacheInvalidator creates a new cache invalidator
func NewBalanceCacheInvalidator(brokers []string, topic string, groupID string, redisClient *redis.Client, logger *zap.Logger) (*BalanceCacheInvalidator, error) {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6, // 10MB
		MaxWait:        1 * time.Second,
		CommitInterval: 1 * time.Second,
		StartOffset:    kafka.LastOffset, // Only process new messages
	})

	return &BalanceCacheInvalidator{
		reader:      reader,
		redisClient: redisClient,
		logger:      logger,
		done:        make(chan struct{}),
	}, nil
}

// Start begins consuming balance-changed events
func (b *BalanceCacheInvalidator) Start(ctx context.Context) error {
	log.Info().Msg("🔄 Starting balance cache invalidator consumer...")

	// Start consuming in a goroutine
	go func() {
		for {
			select {
			case <-b.done:
				b.logger.Info("Balance cache invalidator stopping...")
				return
			case <-ctx.Done():
				b.logger.Info("Balance cache invalidator context cancelled...")
				return
			default:
				msg, err := b.reader.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return // Context cancelled
					}
					b.logger.Debug("Error reading Kafka message", zap.Error(err))
					time.Sleep(1 * time.Second)
					continue
				}

				// Process the message
				if err := b.handleBalanceChanged(ctx, msg.Value); err != nil {
					b.logger.Warn("Failed to handle balance changed event", zap.Error(err))
				}
			}
		}
	}()

	log.Info().Msg("✅ Balance cache invalidator started")
	return nil
}

// Stop gracefully stops the consumer
func (b *BalanceCacheInvalidator) Stop() error {
	close(b.done)
	return b.reader.Close()
}

// handleBalanceChanged processes a balance change event and invalidates cache
func (b *BalanceCacheInvalidator) handleBalanceChanged(ctx context.Context, data []byte) error {
	var event BalanceChangedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		b.logger.Error("Failed to unmarshal balance changed event", zap.Error(err))
		return nil // Don't retry unmarshal errors
	}

	b.logger.Debug("Processing balance change for cache invalidation",
		zap.String("account_id", event.AccountID),
		zap.String("user_id", event.UserID),
		zap.String("transaction_type", event.TransactionType),
		zap.Float64("amount", event.Amount),
		zap.Float64("new_balance", event.BalanceAfter),
	)

	// Invalidate all relevant cache keys
	patterns := b.getInvalidationPatterns(event)

	for _, pattern := range patterns {
		iter := b.redisClient.Scan(ctx, 0, pattern, 100).Iterator()
		keysDeleted := 0
		for iter.Next(ctx) {
			if err := b.redisClient.Del(ctx, iter.Val()).Err(); err != nil {
				b.logger.Warn("Failed to delete cache key",
					zap.String("key", iter.Val()),
					zap.Error(err),
				)
			} else {
				keysDeleted++
			}
		}
		if err := iter.Err(); err != nil {
			b.logger.Warn("Error scanning cache keys",
				zap.String("pattern", pattern),
				zap.Error(err),
			)
		}
		if keysDeleted > 0 {
			b.logger.Debug("Cache keys invalidated",
				zap.String("pattern", pattern),
				zap.Int("keys_deleted", keysDeleted),
			)
		}
	}

	return nil
}

// getInvalidationPatterns returns cache key patterns to invalidate for a balance
// change.
//
// THE PREFIX HAS TO COME FROM THE MIDDLEWARE. These patterns were hardcoded to
// "cache:gateway:GET", the namespace the response cache used before it became
// per-principal on 2026-10-27; the cache now writes under
// middleware.CacheKeyPrefix ("cache:core-gateway:GET"), so every SCAN here
// matched nothing and the consumer went on logging a successful run. This was
// the ONE mechanism that cleared a cached balance after a credit nobody asked
// for over HTTP — an inbound NIP transfer, a provider settlement webhook,
// interest accrual — because all of those arrive over gRPC or Kafka and never
// pass the gin InvalidateCacheMiddleware. Built from the constant now, so the
// two cannot drift again.
//
// Two of the old patterns could never have matched either: "user-accounts" is
// not a route (the list is /api/v1/accounts and /api/v1/accounts/summary), and
// /api/v1/transactions/history carries no account id to match on. The patterns
// below are written against the real paths, with the key's scope segment
// covered by the leading wildcard.
func (b *BalanceCacheInvalidator) getInvalidationPatterns(event BalanceChangedEvent) []string {
	prefix := middleware.CacheKeyPrefix
	patterns := []string{
		// Anything under this account: :balance, :holds, :transactions,
		// :spending-usage, and the account record itself.
		fmt.Sprintf("%s:*:api:v1:accounts:%s*", prefix, event.AccountID),

		// The account LIST and the summary carry aggregated balances and name
		// no account in the path, so they can only be cleared wholesale.
		fmt.Sprintf("%s:*:api:v1:accounts", prefix),
		fmt.Sprintf("%s:*:api:v1:accounts:*", prefix),

		// Transaction history / statistics / analytics are derived from the
		// rows this event just added.
		fmt.Sprintf("%s:*:api:v1:transactions*", prefix),
	}

	// Account-number-addressed lookup (/api/v1/accounts/number/{n}).
	if event.AccountNumber != "" {
		sanitizedNum := strings.ReplaceAll(event.AccountNumber, "-", "")
		patterns = append(patterns,
			fmt.Sprintf("%s:*:api:v1:accounts:number:%s*", prefix, sanitizedNum))
	}

	return patterns
}

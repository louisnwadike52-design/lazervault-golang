package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	kafkaclient "github.com/lazervault/kafka-client"
	"go.uber.org/zap"
)

// ConsumerService handles Kafka message consumption
type ConsumerService struct {
	consumers map[string]*kafkaclient.Consumer
	handlers  map[string]MessageHandler
	logger    *zap.Logger
}

// MessageHandler processes a Kafka message
type MessageHandler func(ctx context.Context, payload json.RawMessage) error

// ConsumerConfig holds consumer configuration
type ConsumerConfig struct {
	Brokers     []string
	GroupID     string
	ServiceName string
	Logger      *zap.Logger
}

// NewConsumerService creates a new Kafka consumer service
func NewConsumerService(config ConsumerConfig) (*ConsumerService, error) {
	if config.Logger == nil {
		config.Logger, _ = zap.NewProduction()
	}

	if config.GroupID == "" {
		config.GroupID = "lazervault-backend-group"
	}

	return &ConsumerService{
		consumers: make(map[string]*kafkaclient.Consumer),
		handlers:  make(map[string]MessageHandler),
		logger:    config.Logger,
	}, nil
}

// RegisterHandler registers a message handler for a specific topic
func (cs *ConsumerService) RegisterHandler(topic string, handler MessageHandler) error {
	cs.handlers[topic] = handler
	cs.logger.Info("registered handler", zap.String("topic", topic))
	return nil
}

// AddConsumer adds a consumer for a specific topic
func (cs *ConsumerService) AddConsumer(config ConsumerConfig, topic string, dlqTopic string) error {
	consumer, err := kafkaclient.NewConsumer(kafkaclient.ConsumerConfig{
		Brokers:     config.Brokers,
		Topic:       topic,
		GroupID:     config.GroupID,
		ServiceName: config.ServiceName,
		DLQTopic:    dlqTopic,
		Logger:      config.Logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create consumer for %s: %w", topic, err)
	}

	// Subscribe with the registered handler
	handler, exists := cs.handlers[topic]
	if !exists {
		return fmt.Errorf("no handler registered for topic: %s", topic)
	}

	consumer.Subscribe(topic, func(ctx context.Context, msg kafkaclient.Message) error {
		// Extract payload
		payload, ok := msg.Value.(json.RawMessage)
		if !ok {
			// Try to marshal if it's not already RawMessage
			payloadBytes, err := json.Marshal(msg.Value)
			if err != nil {
				return fmt.Errorf("failed to marshal message value: %w", err)
			}
			payload = payloadBytes
		}

		// Call the registered handler
		return handler(ctx, payload)
	})

	cs.consumers[topic] = consumer
	cs.logger.Info("added consumer", zap.String("topic", topic))

	return nil
}

// Start begins consuming messages from all registered topics
func (cs *ConsumerService) Start(ctx context.Context) error {
	cs.logger.Info("starting Kafka consumer service", zap.Int("consumers", len(cs.consumers)))

	// Start each consumer in its own goroutine
	for topic, consumer := range cs.consumers {
		topicName := topic // Capture for goroutine
		cons := consumer

		go func() {
			cs.logger.Info("starting consumer", zap.String("topic", topicName))
			if err := cons.Start(ctx); err != nil {
				cs.logger.Error("consumer error",
					zap.String("topic", topicName),
					zap.Error(err),
				)
			}
		}()
	}

	return nil
}

// Close closes all consumers
func (cs *ConsumerService) Close() error {
	cs.logger.Info("closing Kafka consumer service")
	for topic, consumer := range cs.consumers {
		if err := consumer.Close(); err != nil {
			cs.logger.Error("failed to close consumer",
				zap.String("topic", topic),
				zap.Error(err),
			)
		}
	}
	return nil
}

// GetLag returns the current lag for a specific topic consumer
func (cs *ConsumerService) GetLag(topic string) (int64, error) {
	consumer, exists := cs.consumers[topic]
	if !exists {
		return 0, fmt.Errorf("no consumer found for topic: %s", topic)
	}
	return consumer.Lag(), nil
}

// UpdateLag updates lag metrics for all consumers
func (cs *ConsumerService) UpdateLag(ctx context.Context) error {
	for topic, consumer := range cs.consumers {
		if err := consumer.UpdateLag(ctx); err != nil {
			cs.logger.Error("failed to update lag",
				zap.String("topic", topic),
				zap.Error(err),
			)
		}
	}
	return nil
}

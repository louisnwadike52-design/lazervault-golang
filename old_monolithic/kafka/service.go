package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	kafkaclient "github.com/lazervault/kafka-client"
	"go.uber.org/zap"
)

// Service wraps Kafka producers for different topic categories
type Service struct {
	producers map[string]*kafkaclient.Producer
	logger    *zap.Logger
}

// Config holds Kafka service configuration
type Config struct {
	Brokers     []string
	ServiceName string
	Logger      *zap.Logger
}

// TopicConfig defines configuration for each topic category
type TopicConfig struct {
	Topic    string
	DLQTopic string
}

// NewService creates a new Kafka service with all producers
func NewService(config Config) (*Service, error) {
	if config.Logger == nil {
		config.Logger, _ = zap.NewProduction()
	}

	if len(config.Brokers) == 0 {
		return nil, fmt.Errorf("at least one Kafka broker is required")
	}

	// Define all topic configurations
	topicConfigs := map[string]TopicConfig{
		"email.verification":              {Topic: "email.verification", DLQTopic: "dlq.email"},
		"email.generic":                   {Topic: "email.generic", DLQTopic: "dlq.email"},
		"email.deposit-reversal":          {Topic: "email.deposit-reversal", DLQTopic: "dlq.email"},
		"email.withdrawal-confirmation":   {Topic: "email.withdrawal-confirmation", DLQTopic: "dlq.email"},
		"email.withdrawal-failure":        {Topic: "email.withdrawal-failure", DLQTopic: "dlq.email"},
		"email.invoice":                   {Topic: "email.invoice", DLQTopic: "dlq.email"},
		"email.payment-receipt":           {Topic: "email.payment-receipt", DLQTopic: "dlq.email"},
		"email.payment-confirmation":      {Topic: "email.payment-confirmation", DLQTopic: "dlq.email"},
		"transactions.deposits":           {Topic: "transactions.deposits", DLQTopic: "dlq.transactions"},
		"transactions.withdrawals":        {Topic: "transactions.withdrawals", DLQTopic: "dlq.transactions"},
		"transactions.transfers":          {Topic: "transactions.transfers", DLQTopic: "dlq.transactions"},
		"transactions.external-transfers": {Topic: "transactions.external-transfers", DLQTopic: "dlq.transactions"},
		"transactions.bill-payments":      {Topic: "transactions.bill-payments", DLQTopic: "dlq.transactions"},
		"otp.password-reset-sms":          {Topic: "otp.password-reset-sms", DLQTopic: "dlq.general"},
		"otp.password-reset-email":        {Topic: "otp.password-reset-email", DLQTopic: "dlq.general"},
		"scheduled.transfer-check":        {Topic: "scheduled.transfer-check", DLQTopic: "dlq.general"},
		"scheduled.auto-save-check":       {Topic: "scheduled.auto-save-check", DLQTopic: "dlq.general"},
		"scheduled.auto-recharge-check":   {Topic: "scheduled.auto-recharge-check", DLQTopic: "dlq.general"},
		"scheduled.reminders":             {Topic: "scheduled.reminders", DLQTopic: "dlq.general"},
		"data.transaction-files":          {Topic: "data.transaction-files", DLQTopic: "dlq.general"},
		"data.transaction-index-update":   {Topic: "data.transaction-index-update", DLQTopic: "dlq.general"},
		"data.chat-history-index":         {Topic: "data.chat-history-index", DLQTopic: "dlq.general"},
		"sync.providers":                  {Topic: "sync.providers", DLQTopic: "dlq.general"},
	}

	// Create producers for all topics
	producers := make(map[string]*kafkaclient.Producer)
	for name, topicCfg := range topicConfigs {
		producer, err := kafkaclient.NewProducer(kafkaclient.ProducerConfig{
			Brokers:      config.Brokers,
			Topic:        topicCfg.Topic,
			DLQTopic:     topicCfg.DLQTopic,
			ServiceName:  config.ServiceName,
			Logger:       config.Logger,
			MaxRetries:   3,
			RetryBackoff: 1 * time.Second,
		})
		if err != nil {
			// Close any producers that were created before error
			for _, p := range producers {
				p.Close()
			}
			return nil, fmt.Errorf("failed to create producer for %s: %w", name, err)
		}
		producers[name] = producer
	}

	config.Logger.Info("Kafka service initialized", zap.Int("producers", len(producers)))

	return &Service{
		producers: producers,
		logger:    config.Logger,
	}, nil
}

// SendMessage sends a message to the specified topic
func (s *Service) SendMessage(ctx context.Context, topic string, key string, payload interface{}) error {
	producer, exists := s.producers[topic]
	if !exists {
		return fmt.Errorf("no producer found for topic: %s", topic)
	}

	// Marshal payload to ensure it's valid JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	var payloadMap map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payloadMap); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	msg := kafkaclient.Message{
		Key:   key,
		Value: payloadMap,
		Headers: map[string]string{
			"source":  "lazervault-golang",
			"version": "1.0",
		},
	}

	if err := producer.SendMessage(ctx, msg); err != nil {
		s.logger.Error("failed to send Kafka message",
			zap.String("topic", topic),
			zap.String("key", key),
			zap.Error(err),
		)
		return err
	}

	s.logger.Debug("message sent to Kafka",
		zap.String("topic", topic),
		zap.String("key", key),
	)

	return nil
}

// SendBatch sends multiple messages to the specified topic
func (s *Service) SendBatch(ctx context.Context, topic string, messages []kafkaclient.Message) error {
	producer, exists := s.producers[topic]
	if !exists {
		return fmt.Errorf("no producer found for topic: %s", topic)
	}

	return producer.SendBatch(ctx, messages)
}

// Close closes all producers
func (s *Service) Close() error {
	s.logger.Info("closing Kafka service")
	for name, producer := range s.producers {
		if err := producer.Close(); err != nil {
			s.logger.Error("failed to close producer",
				zap.String("topic", name),
				zap.Error(err),
			)
		}
	}
	return nil
}

// GetStats returns statistics for a specific topic producer
func (s *Service) GetStats(topic string) (interface{}, error) {
	producer, exists := s.producers[topic]
	if !exists {
		return nil, fmt.Errorf("no producer found for topic: %s", topic)
	}
	return producer.Stats(), nil
}

// Helper methods for common operations

// SendEmailTask sends an email task to the appropriate topic
func (s *Service) SendEmailTask(ctx context.Context, emailType string, userID uint32, payload interface{}) error {
	var topic string
	switch emailType {
	case "verification":
		topic = "email.verification"
	case "deposit-reversal":
		topic = "email.deposit-reversal"
	case "withdrawal-confirmation":
		topic = "email.withdrawal-confirmation"
	case "withdrawal-failure":
		topic = "email.withdrawal-failure"
	case "invoice":
		topic = "email.invoice"
	case "payment-receipt":
		topic = "email.payment-receipt"
	case "payment-confirmation":
		topic = "email.payment-confirmation"
	default:
		topic = "email.generic"
	}

	return s.SendMessage(ctx, topic, fmt.Sprintf("user_%d", userID), payload)
}

// SendTransactionTask sends a transaction task
func (s *Service) SendTransactionTask(ctx context.Context, txType string, userID uint32, payload interface{}) error {
	var topic string
	switch txType {
	case "deposit":
		topic = "transactions.deposits"
	case "withdrawal":
		topic = "transactions.withdrawals"
	case "transfer":
		topic = "transactions.transfers"
	case "external-transfer":
		topic = "transactions.external-transfers"
	case "bill-payment":
		topic = "transactions.bill-payments"
	default:
		return fmt.Errorf("unknown transaction type: %s", txType)
	}

	return s.SendMessage(ctx, topic, fmt.Sprintf("user_%d", userID), payload)
}

// SendOTPTask sends an OTP task
func (s *Service) SendOTPTask(ctx context.Context, otpType string, userID uint32, payload interface{}) error {
	var topic string
	switch otpType {
	case "sms":
		topic = "otp.password-reset-sms"
	case "email":
		topic = "otp.password-reset-email"
	default:
		return fmt.Errorf("unknown OTP type: %s", otpType)
	}

	return s.SendMessage(ctx, topic, fmt.Sprintf("user_%d", userID), payload)
}

// SendScheduledTask sends a scheduled task
func (s *Service) SendScheduledTask(ctx context.Context, taskType string, payload interface{}) error {
	var topic string
	switch taskType {
	case "transfer-check":
		topic = "scheduled.transfer-check"
	case "auto-save-check":
		topic = "scheduled.auto-save-check"
	case "auto-recharge-check":
		topic = "scheduled.auto-recharge-check"
	case "reminders":
		topic = "scheduled.reminders"
	default:
		return fmt.Errorf("unknown scheduled task type: %s", taskType)
	}

	return s.SendMessage(ctx, topic, fmt.Sprintf("scheduled_%d", time.Now().Unix()), payload)
}

// SendDataTask sends a data generation task
func (s *Service) SendDataTask(ctx context.Context, taskType string, userID uint32, payload interface{}) error {
	var topic string
	switch taskType {
	case "transaction-files":
		topic = "data.transaction-files"
	case "transaction-index-update":
		topic = "data.transaction-index-update"
	case "chat-history-index":
		topic = "data.chat-history-index"
	default:
		return fmt.Errorf("unknown data task type: %s", taskType)
	}

	return s.SendMessage(ctx, topic, fmt.Sprintf("user_%d", userID), payload)
}

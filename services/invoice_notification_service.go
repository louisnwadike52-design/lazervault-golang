package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrNotificationNotFound    = errors.New("notification not found")
	ErrInvalidNotificationData = errors.New("invalid notification data")
	ErrNotificationSendFailed  = errors.New("failed to send notification")
)

// NotificationType represents different types of invoice notifications
type NotificationType string

const (
	NotificationTypeInvoiceCreated   NotificationType = "invoice_created"
	NotificationTypeInvoiceSent      NotificationType = "invoice_sent"
	NotificationTypePaymentDue       NotificationType = "payment_due"
	NotificationTypePaymentOverdue   NotificationType = "payment_overdue"
	NotificationTypePaymentReceived  NotificationType = "payment_received"
	NotificationTypePaymentFailed    NotificationType = "payment_failed"
	NotificationTypePaymentPartial   NotificationType = "payment_partial"
	NotificationTypeInvoiceCancelled NotificationType = "invoice_cancelled"
	NotificationTypePaymentReminder  NotificationType = "payment_reminder"
	NotificationTypeInvoiceViewed    NotificationType = "invoice_viewed"
	NotificationTypePaymentExtension NotificationType = "payment_extension"
	NotificationTypePaymentDispute   NotificationType = "payment_dispute"
)

// NotificationChannel represents different notification delivery channels
type NotificationChannel string

const (
	NotificationChannelEmail   NotificationChannel = "email"
	NotificationChannelSMS     NotificationChannel = "sms"
	NotificationChannelPush    NotificationChannel = "push"
	NotificationChannelInApp   NotificationChannel = "in_app"
	NotificationChannelWebhook NotificationChannel = "webhook"
)

// InvoiceNotification represents an invoice-related notification
type InvoiceNotification struct {
	ID          uuid.UUID           `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	UserID      string              `json:"user_id" gorm:"type:uuid;not null;index"`
	InvoiceID   string              `json:"invoice_id" gorm:"type:uuid;not null;index"`
	Type        NotificationType    `json:"type" gorm:"size:50;not null"`
	Channel     NotificationChannel `json:"channel" gorm:"size:20;not null"`
	Title       string              `json:"title" gorm:"size:255;not null"`
	Message     string              `json:"message" gorm:"type:text;not null"`
	Recipient   string              `json:"recipient" gorm:"size:255;not null"`
	Status      string              `json:"status" gorm:"size:20;not null;default:'pending'"`
	SentAt      *time.Time          `json:"sent_at"`
	ReadAt      *time.Time          `json:"read_at"`
	ScheduledAt time.Time           `json:"scheduled_at" gorm:"index"`
	Retries     int                 `json:"retries" gorm:"default:0"`
	MaxRetries  int                 `json:"max_retries" gorm:"default:3"`
	Metadata    map[string]string   `json:"metadata" gorm:"type:jsonb"`
	CreatedAt   time.Time           `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time           `json:"updated_at" gorm:"autoUpdateTime"`
}

// IInvoiceNotificationService defines the interface for invoice notification operations
type IInvoiceNotificationService interface {
	// Notification Creation
	CreateNotification(ctx context.Context, req *CreateNotificationRequest) (*InvoiceNotification, error)
	SchedulePaymentReminder(ctx context.Context, req *SchedulePaymentReminderRequest) error
	SendImmediateNotification(ctx context.Context, req *SendNotificationRequest) error

	// Notification Management
	GetNotification(ctx context.Context, notificationID string) (*InvoiceNotification, error)
	GetUserNotifications(ctx context.Context, userID string, limit int, offset int) ([]*InvoiceNotification, error)
	GetInvoiceNotifications(ctx context.Context, invoiceID string) ([]*InvoiceNotification, error)
	MarkNotificationAsRead(ctx context.Context, notificationID string) error
	MarkNotificationAsSent(ctx context.Context, notificationID string) error

	// Scheduled Notifications
	ProcessScheduledNotifications(ctx context.Context) error
	RetryFailedNotifications(ctx context.Context) error

	// Notification Templates
	GetNotificationTemplate(notificationType NotificationType) (*NotificationTemplate, error)
	RenderNotificationContent(template *NotificationTemplate, data map[string]interface{}) (string, string, error)

	// Statistics
	GetNotificationStatistics(ctx context.Context, userID string, startDate, endDate time.Time) (*NotificationStatistics, error)
}

// CreateNotificationRequest represents a request to create a notification
type CreateNotificationRequest struct {
	UserID      string                 `json:"user_id"`
	InvoiceID   string                 `json:"invoice_id"`
	Type        NotificationType       `json:"type"`
	Channel     NotificationChannel    `json:"channel"`
	Recipient   string                 `json:"recipient"`
	ScheduledAt time.Time              `json:"scheduled_at"`
	Data        map[string]interface{} `json:"data"`
}

// SchedulePaymentReminderRequest represents a request to schedule a payment reminder
type SchedulePaymentReminderRequest struct {
	UserID        string              `json:"user_id"`
	InvoiceID     string              `json:"invoice_id"`
	ReminderAt    time.Time           `json:"reminder_at"`
	Channel       NotificationChannel `json:"channel"`
	Recipient     string              `json:"recipient"`
	CustomMessage string              `json:"custom_message"`
}

// SendNotificationRequest represents a request to send an immediate notification
type SendNotificationRequest struct {
	UserID    string                 `json:"user_id"`
	InvoiceID string                 `json:"invoice_id"`
	Type      NotificationType       `json:"type"`
	Channel   NotificationChannel    `json:"channel"`
	Recipient string                 `json:"recipient"`
	Data      map[string]interface{} `json:"data"`
}

// NotificationTemplate represents a notification template
type NotificationTemplate struct {
	Type          NotificationType       `json:"type"`
	TitleTemplate string                 `json:"title_template"`
	BodyTemplate  string                 `json:"body_template"`
	DefaultData   map[string]interface{} `json:"default_data"`
}

// NotificationStatistics represents notification statistics
type NotificationStatistics struct {
	TotalSent    int64                         `json:"total_sent"`
	TotalFailed  int64                         `json:"total_failed"`
	TotalPending int64                         `json:"total_pending"`
	TotalRead    int64                         `json:"total_read"`
	TotalUnread  int64                         `json:"total_unread"`
	ByType       map[NotificationType]int64    `json:"by_type"`
	ByChannel    map[NotificationChannel]int64 `json:"by_channel"`
	SuccessRate  float64                       `json:"success_rate"`
}

// InvoiceNotificationService implements the IInvoiceNotificationService
type InvoiceNotificationService struct {
	db *gorm.DB
	// Add email/SMS/push notification clients here
}

// NewInvoiceNotificationService creates a new InvoiceNotificationService
func NewInvoiceNotificationService(db *gorm.DB) IInvoiceNotificationService {
	return &InvoiceNotificationService{db: db}
}

// CreateNotification creates a new notification
func (s *InvoiceNotificationService) CreateNotification(ctx context.Context, req *CreateNotificationRequest) (*InvoiceNotification, error) {
	if req.UserID == "" || req.InvoiceID == "" || req.Recipient == "" {
		return nil, ErrInvalidNotificationData
	}

	// Get notification template
	template, err := s.GetNotificationTemplate(req.Type)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification template: %w", err)
	}

	// Render notification content
	title, message, err := s.RenderNotificationContent(template, req.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to render notification content: %w", err)
	}

	notification := &InvoiceNotification{
		UserID:      req.UserID,
		InvoiceID:   req.InvoiceID,
		Type:        req.Type,
		Channel:     req.Channel,
		Title:       title,
		Message:     message,
		Recipient:   req.Recipient,
		Status:      "pending",
		ScheduledAt: req.ScheduledAt,
		Metadata:    convertToStringMap(req.Data),
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.db.WithContext(ctx).Create(notification).Error; err != nil {
		return nil, fmt.Errorf("failed to create notification: %w", err)
	}

	return notification, nil
}

// SchedulePaymentReminder schedules a payment reminder notification
func (s *InvoiceNotificationService) SchedulePaymentReminder(ctx context.Context, req *SchedulePaymentReminderRequest) error {
	data := map[string]interface{}{
		"user_id":        req.UserID,
		"invoice_id":     req.InvoiceID,
		"custom_message": req.CustomMessage,
	}

	createReq := &CreateNotificationRequest{
		UserID:      req.UserID,
		InvoiceID:   req.InvoiceID,
		Type:        NotificationTypePaymentReminder,
		Channel:     req.Channel,
		Recipient:   req.Recipient,
		ScheduledAt: req.ReminderAt,
		Data:        data,
	}

	_, err := s.CreateNotification(ctx, createReq)
	return err
}

// SendImmediateNotification sends a notification immediately
func (s *InvoiceNotificationService) SendImmediateNotification(ctx context.Context, req *SendNotificationRequest) error {
	createReq := &CreateNotificationRequest{
		UserID:      req.UserID,
		InvoiceID:   req.InvoiceID,
		Type:        req.Type,
		Channel:     req.Channel,
		Recipient:   req.Recipient,
		ScheduledAt: time.Now().UTC(),
		Data:        req.Data,
	}

	notification, err := s.CreateNotification(ctx, createReq)
	if err != nil {
		return err
	}

	// Send the notification immediately
	return s.sendNotification(ctx, notification)
}

// GetNotification retrieves a specific notification
func (s *InvoiceNotificationService) GetNotification(ctx context.Context, notificationID string) (*InvoiceNotification, error) {
	var notification InvoiceNotification
	err := s.db.WithContext(ctx).Where("id = ?", notificationID).First(&notification).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotificationNotFound
		}
		return nil, fmt.Errorf("database error: %w", err)
	}
	return &notification, nil
}

// GetUserNotifications retrieves notifications for a user
func (s *InvoiceNotificationService) GetUserNotifications(ctx context.Context, userID string, limit int, offset int) ([]*InvoiceNotification, error) {
	var notifications []*InvoiceNotification
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifications).Error
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	return notifications, nil
}

// GetInvoiceNotifications retrieves notifications for a specific invoice
func (s *InvoiceNotificationService) GetInvoiceNotifications(ctx context.Context, invoiceID string) ([]*InvoiceNotification, error) {
	var notifications []*InvoiceNotification
	err := s.db.WithContext(ctx).
		Where("invoice_id = ?", invoiceID).
		Order("created_at DESC").
		Find(&notifications).Error
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}
	return notifications, nil
}

// MarkNotificationAsRead marks a notification as read
func (s *InvoiceNotificationService) MarkNotificationAsRead(ctx context.Context, notificationID string) error {
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).
		Model(&InvoiceNotification{}).
		Where("id = ?", notificationID).
		Update("read_at", now)

	if result.Error != nil {
		return fmt.Errorf("failed to mark notification as read: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}

	return nil
}

// MarkNotificationAsSent marks a notification as sent
func (s *InvoiceNotificationService) MarkNotificationAsSent(ctx context.Context, notificationID string) error {
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).
		Model(&InvoiceNotification{}).
		Where("id = ?", notificationID).
		Updates(map[string]interface{}{
			"status":  "sent",
			"sent_at": now,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to mark notification as sent: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}

	return nil
}

// ProcessScheduledNotifications processes notifications that are scheduled to be sent
func (s *InvoiceNotificationService) ProcessScheduledNotifications(ctx context.Context) error {
	var notifications []*InvoiceNotification
	now := time.Now().UTC()

	err := s.db.WithContext(ctx).
		Where("status = ? AND scheduled_at <= ?", "pending", now).
		Find(&notifications).Error
	if err != nil {
		return fmt.Errorf("failed to fetch scheduled notifications: %w", err)
	}

	for _, notification := range notifications {
		if err := s.sendNotification(ctx, notification); err != nil {
			// Log error but continue processing other notifications
			fmt.Printf("Failed to send notification %s: %v\n", notification.ID, err)

			// Update retry count
			s.db.WithContext(ctx).
				Model(notification).
				Updates(map[string]interface{}{
					"retries": notification.Retries + 1,
					"status":  "failed",
				})
		}
	}

	return nil
}

// RetryFailedNotifications retries failed notifications
func (s *InvoiceNotificationService) RetryFailedNotifications(ctx context.Context) error {
	var notifications []*InvoiceNotification

	err := s.db.WithContext(ctx).
		Where("status = ? AND retries < max_retries", "failed").
		Find(&notifications).Error
	if err != nil {
		return fmt.Errorf("failed to fetch failed notifications: %w", err)
	}

	for _, notification := range notifications {
		if err := s.sendNotification(ctx, notification); err != nil {
			// Log error and update retry count
			fmt.Printf("Retry failed for notification %s: %v\n", notification.ID, err)

			s.db.WithContext(ctx).
				Model(notification).
				Update("retries", notification.Retries+1)
		}
	}

	return nil
}

// GetNotificationTemplate returns a notification template for the given type
func (s *InvoiceNotificationService) GetNotificationTemplate(notificationType NotificationType) (*NotificationTemplate, error) {
	// In a real implementation, these would be stored in the database or a configuration file
	templates := map[NotificationType]*NotificationTemplate{
		NotificationTypeInvoiceCreated: {
			Type:          NotificationTypeInvoiceCreated,
			TitleTemplate: "New Invoice Created",
			BodyTemplate:  "A new invoice {{.invoice_number}} has been created for {{.amount}} {{.currency}}.",
		},
		NotificationTypePaymentDue: {
			Type:          NotificationTypePaymentDue,
			TitleTemplate: "Payment Due",
			BodyTemplate:  "Payment for invoice {{.invoice_number}} is due on {{.due_date}}. Amount: {{.amount}} {{.currency}}.",
		},
		NotificationTypePaymentOverdue: {
			Type:          NotificationTypePaymentOverdue,
			TitleTemplate: "Payment Overdue",
			BodyTemplate:  "Payment for invoice {{.invoice_number}} is overdue. Please pay {{.amount}} {{.currency}} as soon as possible.",
		},
		NotificationTypePaymentReceived: {
			Type:          NotificationTypePaymentReceived,
			TitleTemplate: "Payment Received",
			BodyTemplate:  "Payment of {{.amount}} {{.currency}} has been received for invoice {{.invoice_number}}.",
		},
		NotificationTypePaymentReminder: {
			Type:          NotificationTypePaymentReminder,
			TitleTemplate: "Payment Reminder",
			BodyTemplate:  "This is a reminder that payment for invoice {{.invoice_number}} is due. {{.custom_message}}",
		},
	}

	template, exists := templates[notificationType]
	if !exists {
		return nil, fmt.Errorf("template not found for notification type: %s", notificationType)
	}

	return template, nil
}

// RenderNotificationContent renders the notification title and message using the template and data
func (s *InvoiceNotificationService) RenderNotificationContent(template *NotificationTemplate, data map[string]interface{}) (string, string, error) {
	// Simple template rendering - in a real implementation, you might use a more sophisticated template engine
	title := template.TitleTemplate
	message := template.BodyTemplate

	// Replace template variables with actual data
	for key, value := range data {
		placeholder := fmt.Sprintf("{{.%s}}", key)
		strValue := fmt.Sprintf("%v", value)
		title = replaceAll(title, placeholder, strValue)
		message = replaceAll(message, placeholder, strValue)
	}

	return title, message, nil
}

// GetNotificationStatistics retrieves notification statistics for a user
func (s *InvoiceNotificationService) GetNotificationStatistics(ctx context.Context, userID string, startDate, endDate time.Time) (*NotificationStatistics, error) {
	// TODO: Implement comprehensive statistics gathering
	stats := &NotificationStatistics{
		ByType:    make(map[NotificationType]int64),
		ByChannel: make(map[NotificationChannel]int64),
	}

	// Get total counts
	var totalSent, totalFailed, totalPending, totalRead, totalUnread int64

	s.db.WithContext(ctx).Model(&InvoiceNotification{}).
		Where("user_id = ? AND created_at BETWEEN ? AND ? AND status = ?", userID, startDate, endDate, "sent").
		Count(&totalSent)

	s.db.WithContext(ctx).Model(&InvoiceNotification{}).
		Where("user_id = ? AND created_at BETWEEN ? AND ? AND status = ?", userID, startDate, endDate, "failed").
		Count(&totalFailed)

	s.db.WithContext(ctx).Model(&InvoiceNotification{}).
		Where("user_id = ? AND created_at BETWEEN ? AND ? AND status = ?", userID, startDate, endDate, "pending").
		Count(&totalPending)

	s.db.WithContext(ctx).Model(&InvoiceNotification{}).
		Where("user_id = ? AND created_at BETWEEN ? AND ? AND read_at IS NOT NULL", userID, startDate, endDate).
		Count(&totalRead)

	s.db.WithContext(ctx).Model(&InvoiceNotification{}).
		Where("user_id = ? AND created_at BETWEEN ? AND ? AND read_at IS NULL", userID, startDate, endDate).
		Count(&totalUnread)

	stats.TotalSent = totalSent
	stats.TotalFailed = totalFailed
	stats.TotalPending = totalPending
	stats.TotalRead = totalRead
	stats.TotalUnread = totalUnread

	if totalSent+totalFailed > 0 {
		stats.SuccessRate = float64(totalSent) / float64(totalSent+totalFailed) * 100
	}

	return stats, nil
}

// Helper functions

func (s *InvoiceNotificationService) sendNotification(ctx context.Context, notification *InvoiceNotification) error {
	// TODO: Implement actual notification sending based on channel
	switch notification.Channel {
	case NotificationChannelEmail:
		return s.sendEmailNotification(ctx, notification)
	case NotificationChannelSMS:
		return s.sendSMSNotification(ctx, notification)
	case NotificationChannelPush:
		return s.sendPushNotification(ctx, notification)
	case NotificationChannelInApp:
		return s.sendInAppNotification(ctx, notification)
	case NotificationChannelWebhook:
		return s.sendWebhookNotification(ctx, notification)
	default:
		return fmt.Errorf("unsupported notification channel: %s", notification.Channel)
	}
}

func (s *InvoiceNotificationService) sendEmailNotification(ctx context.Context, notification *InvoiceNotification) error {
	// TODO: Implement email sending logic
	// For now, just mark as sent
	return s.MarkNotificationAsSent(ctx, notification.ID.String())
}

func (s *InvoiceNotificationService) sendSMSNotification(ctx context.Context, notification *InvoiceNotification) error {
	// TODO: Implement SMS sending logic
	return s.MarkNotificationAsSent(ctx, notification.ID.String())
}

func (s *InvoiceNotificationService) sendPushNotification(ctx context.Context, notification *InvoiceNotification) error {
	// TODO: Implement push notification logic
	return s.MarkNotificationAsSent(ctx, notification.ID.String())
}

func (s *InvoiceNotificationService) sendInAppNotification(ctx context.Context, notification *InvoiceNotification) error {
	// For in-app notifications, just mark as sent since it's stored in the database
	return s.MarkNotificationAsSent(ctx, notification.ID.String())
}

func (s *InvoiceNotificationService) sendWebhookNotification(ctx context.Context, notification *InvoiceNotification) error {
	// TODO: Implement webhook sending logic
	return s.MarkNotificationAsSent(ctx, notification.ID.String())
}

func convertToStringMap(data map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range data {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

func replaceAll(text, old, new string) string {
	// Simple string replacement - in a real implementation, you might use regex or a template engine
	result := text
	for i := 0; i < len(text); i++ {
		if len(result) >= len(old) && result[i:i+len(old)] == old {
			result = result[:i] + new + result[i+len(old):]
			i += len(new) - 1
		}
	}
	return result
}

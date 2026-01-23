package services

import (
	"context"
	"fmt"
	"lazervaultGo/models"
	"log"

	"gorm.io/gorm"
)

// NotificationService handles sending notifications to users
type NotificationService struct {
	db *gorm.DB
}

// NewNotificationService creates a new notification service
func NewNotificationService(db *gorm.DB) *NotificationService {
	return &NotificationService{db: db}
}

// SendTagNotification sends a notification when a user is tagged
func (s *NotificationService) SendTagNotification(ctx context.Context, tag *models.UserTag) error {
	log.Printf("[NotificationService] Sending tag notification to user %s", tag.TaggedUserID)
	log.Printf("[NotificationService] Tag details: %s tagged you for %s %.2f", tag.TaggerName, tag.Currency, tag.Amount)

	// TODO: Implement actual notification storage
	// This would require creating a notifications table and model
	// For now, we're just logging the notification

	// notification := &models.Notification{
	// 	UserID:  tag.TaggedUserID,
	// 	Type:    "tag_received",
	// 	Title:   "You've been tagged!",
	// 	Message: fmt.Sprintf("%s tagged you for %s %.2f", tag.TaggerName, tag.Currency, tag.Amount),
	// 	Data: map[string]interface{}{
	// 		"tag_id":      tag.ID,
	// 		"amount":      tag.Amount,
	// 		"currency":    tag.Currency,
	// 		"tagger_name": tag.TaggerName,
	// 	},
	// 	IsRead: false,
	// }

	// if err := s.db.Create(notification).Error; err != nil {
	// 	log.Printf("[NotificationService] Failed to create notification: %v", err)
	// 	return err
	// }

	// Send push notification via FCM (async)
	go s.sendPushNotification(tag.TaggedUserID, tag)

	return nil
}

// sendPushNotification sends a push notification via Firebase Cloud Messaging
func (s *NotificationService) sendPushNotification(userID string, tag *models.UserTag) {
	// TODO: Implement FCM push notification
	// This would require:
	// 1. Firebase Admin SDK setup
	// 2. User FCM token storage
	// 3. FCM message sending

	message := fmt.Sprintf("%s tagged you for %s %.2f", tag.TaggerName, tag.Currency, tag.Amount)
	log.Printf("[NotificationService] TODO: Send push notification to user %s: %s", userID, message)
}

// SendTagPaidNotification sends a notification when a tag is paid
func (s *NotificationService) SendTagPaidNotification(ctx context.Context, tag *models.UserTag) error {
	log.Printf("[NotificationService] Sending tag paid notification to user %s", tag.TaggerID)
	log.Printf("[NotificationService] Tag details: %s paid your tag for %s %.2f", tag.TaggedUserName, tag.Currency, tag.Amount)

	// TODO: Implement actual notification storage
	// Similar to SendTagNotification

	// Send push notification via FCM (async)
	go s.sendTagPaidPushNotification(tag.TaggerID, tag)

	return nil
}

// sendTagPaidPushNotification sends a push notification when tag is paid
func (s *NotificationService) sendTagPaidPushNotification(userID string, tag *models.UserTag) {
	message := fmt.Sprintf("%s paid your tag for %s %.2f", tag.TaggedUserName, tag.Currency, tag.Amount)
	log.Printf("[NotificationService] TODO: Send push notification to user %s: %s", userID, message)
}

// SendInvoiceTaggedNotification sends a notification when a user is tagged to pay an invoice
func (s *NotificationService) SendInvoiceTaggedNotification(ctx context.Context, invoice *models.Invoice, taggedUserID string, taggerName string) error {
	log.Printf("[NotificationService] Sending invoice tagged notification to user %s", taggedUserID)
	log.Printf("[NotificationService] Invoice details: %s tagged you to pay invoice %s (%s %.2f)",
		taggerName, invoice.Title, invoice.Currency, invoice.TotalAmount)

	// TODO: Implement actual notification storage
	// notification := &models.Notification{
	// 	UserID:  taggedUserID,
	// 	Type:    "invoice_tagged",
	// 	Title:   "New Invoice Payment Request",
	// 	Message: fmt.Sprintf("%s tagged you to pay invoice: %s", taggerName, invoice.Title),
	// 	Data: map[string]interface{}{
	// 		"invoice_id":    invoice.ID,
	// 		"amount":        invoice.TotalAmount,
	// 		"currency":      invoice.Currency,
	// 		"tagger_name":   taggerName,
	// 		"invoice_title": invoice.Title,
	// 	},
	// 	IsRead: false,
	// }

	// Send push notification via FCM (async)
	go s.sendInvoiceTaggedPushNotification(taggedUserID, invoice, taggerName)

	return nil
}

// sendInvoiceTaggedPushNotification sends a push notification when user is tagged to an invoice
func (s *NotificationService) sendInvoiceTaggedPushNotification(userID string, invoice *models.Invoice, taggerName string) {
	message := fmt.Sprintf("%s tagged you to pay invoice: %s (%s %.2f)",
		taggerName, invoice.Title, invoice.Currency, invoice.TotalAmount)
	log.Printf("[NotificationService] TODO: Send push notification to user %s: %s", userID, message)
}

// SendInvoicePaidNotification sends a notification when an invoice is paid
func (s *NotificationService) SendInvoicePaidNotification(ctx context.Context, invoice *models.Invoice, payerName string) error {
	log.Printf("[NotificationService] Sending invoice paid notification to user %s", invoice.UserID)
	log.Printf("[NotificationService] Invoice details: %s paid invoice %s (%s %.2f)",
		payerName, invoice.Title, invoice.Currency, invoice.TotalAmount)

	// TODO: Implement actual notification storage
	// notification := &models.Notification{
	// 	UserID:  invoice.UserID,
	// 	Type:    "invoice_paid",
	// 	Title:   "Invoice Paid",
	// 	Message: fmt.Sprintf("%s paid your invoice: %s", payerName, invoice.Title),
	// 	Data: map[string]interface{}{
	// 		"invoice_id":    invoice.ID,
	// 		"amount":        invoice.TotalAmount,
	// 		"currency":      invoice.Currency,
	// 		"payer_name":    payerName,
	// 		"invoice_title": invoice.Title,
	// 	},
	// 	IsRead: false,
	// }

	// Send push notification via FCM (async)
	go s.sendInvoicePaidPushNotification(invoice.UserID, invoice, payerName)

	return nil
}

// sendInvoicePaidPushNotification sends a push notification when invoice is paid
func (s *NotificationService) sendInvoicePaidPushNotification(userID string, invoice *models.Invoice, payerName string) {
	message := fmt.Sprintf("%s paid your invoice: %s (%s %.2f)",
		payerName, invoice.Title, invoice.Currency, invoice.TotalAmount)
	log.Printf("[NotificationService] TODO: Send push notification to user %s: %s", userID, message)
}

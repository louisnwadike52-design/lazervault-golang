package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/tasks"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	ReminderCheckInterval = 1 * time.Hour // Check every hour
)

// ScheduledReminderProcessor handles payment reminder notifications
type ScheduledReminderProcessor struct {
	db              *gorm.DB
	taskDistributor tasks.TaskDistributor
}

func NewScheduledReminderProcessor(db *gorm.DB, taskDistributor tasks.TaskDistributor) *ScheduledReminderProcessor {
	return &ScheduledReminderProcessor{
		db:              db,
		taskDistributor: taskDistributor,
	}
}

// ProcessReminderNotifications finds and sends notifications for due reminders
func (p *ScheduledReminderProcessor) ProcessReminderNotifications(ctx context.Context, t *asynq.Task) error {
	var payload tasks.ReminderNotificationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		// If payload unmarshal fails, treat as a check-all task
		log.Info().Msg("Checking all due reminders")
	}

	// If a specific reminder is provided, process only that one
	if payload.ReminderID != "" {
		return p.sendReminderNotification(ctx, payload.ReminderID)
	}

	// Otherwise, find all active reminders that are due
	var reminders []models.PaymentReminder
	now := time.Now()

	err := p.db.WithContext(ctx).
		Where("status = ?", models.ReminderStatusActive).
		Where("reminder_date <= ?", now).
		Where("(notified_at IS NULL OR is_recurring = ?)", true).
		Find(&reminders).Error

	if err != nil {
		log.Error().Err(err).Msg("Failed to query reminders")
		return fmt.Errorf("failed to query reminders: %w", err)
	}

	log.Info().
		Int("count", len(reminders)).
		Msg("Found reminders to send")

	successCount := 0
	failedCount := 0

	for _, reminder := range reminders {
		if err := p.sendReminderNotification(ctx, reminder.ID); err != nil {
			log.Error().
				Err(err).
				Str("reminder_id", reminder.ID).
				Msg("Failed to send reminder notification")
			failedCount++
		} else {
			successCount++
		}
	}

	log.Info().
		Int("success", successCount).
		Int("failed", failedCount).
		Msg("Reminder notification batch completed")

	// Schedule next check
	return p.ScheduleNextCheck(ctx)
}

// sendReminderNotification sends a notification for a specific reminder
func (p *ScheduledReminderProcessor) sendReminderNotification(ctx context.Context, reminderID string) error {
	var reminder models.PaymentReminder
	if err := p.db.WithContext(ctx).Where("id = ?", reminderID).First(&reminder).Error; err != nil {
		return fmt.Errorf("failed to get reminder: %w", err)
	}

	// Skip if already notified (and not recurring)
	if reminder.NotifiedAt != nil && !reminder.IsRecurring {
		log.Info().
			Str("reminder_id", reminderID).
			Msg("Reminder already notified, skipping")
		return nil
	}

	// Get user details for notification
	var user models.User
	if err := p.db.WithContext(ctx).Where("uuid = ?", reminder.UserID).First(&user).Error; err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// TODO: Send actual notification via email/SMS/push
	// For now, just log the notification
	log.Info().
		Str("reminder_id", reminderID).
		Str("user_email", user.Email).
		Str("title", reminder.Title).
		Str("description", reminder.Description).
		Float64("amount", *reminder.Amount).
		Msg("Sending reminder notification")

	// Example notification message:
	notificationMessage := fmt.Sprintf(
		"Reminder: %s\n%s\n",
		reminder.Title,
		reminder.Description,
	)
	if reminder.Amount != nil {
		notificationMessage += fmt.Sprintf("Amount: ₦%.2f\n", *reminder.Amount)
	}

	// TODO: Implement actual notification sending
	// - Email via mailer service
	// - Push notification via FCM/APNS
	// - SMS via Twilio
	log.Info().
		Str("message", notificationMessage).
		Msg("Would send notification here")

	// Update reminder with notification timestamp
	now := time.Now()
	reminder.NotifiedAt = &now

	// If recurring, calculate next reminder date
	if reminder.IsRecurring && reminder.RecurrenceType != nil {
		nextReminderDate := calculateNextReminderDate(reminder.ReminderDate, *reminder.RecurrenceType)
		reminder.ReminderDate = nextReminderDate
		reminder.NotifiedAt = nil // Reset for next occurrence

		log.Info().
			Str("reminder_id", reminderID).
			Str("next_reminder_date", nextReminderDate.Format(time.RFC3339)).
			Msg("Calculated next reminder date for recurring reminder")
	} else {
		// Mark as completed for non-recurring reminders
		reminder.Status = models.ReminderStatusCompleted
	}

	if err := p.db.WithContext(ctx).Save(&reminder).Error; err != nil {
		return fmt.Errorf("failed to update reminder: %w", err)
	}

	log.Info().
		Str("reminder_id", reminderID).
		Msg("Reminder notification sent successfully")

	return nil
}

// calculateNextReminderDate computes the next reminder date based on recurrence type
func calculateNextReminderDate(currentDate time.Time, recurrenceType string) time.Time {
	switch recurrenceType {
	case "daily":
		return currentDate.Add(24 * time.Hour)
	case "weekly":
		return currentDate.Add(7 * 24 * time.Hour)
	case "monthly":
		return currentDate.AddDate(0, 1, 0)
	default:
		return currentDate.Add(24 * time.Hour)
	}
}

// ScheduleNextCheck schedules the next reminder check
func (p *ScheduledReminderProcessor) ScheduleNextCheck(ctx context.Context) error {
	payloadBytes, err := tasks.NewReminderNotificationTask("", "")
	if err != nil {
		return fmt.Errorf("failed to create reminder check payload: %w", err)
	}

	err = p.taskDistributor.DistributeTask(
		ctx,
		tasks.TaskSendReminderNotification,
		payloadBytes,
		asynq.Queue(tasks.QueueDefault),
		asynq.ProcessIn(ReminderCheckInterval),
	)

	if err != nil {
		log.Error().Err(err).Msg("Failed to schedule next reminder check")
		return err
	}

	log.Info().
		Dur("interval", ReminderCheckInterval).
		Msg("Scheduled next reminder check")

	return nil
}

// StartReminderScheduler initiates the periodic reminder checking
func (p *ScheduledReminderProcessor) StartReminderScheduler(ctx context.Context) error {
	log.Info().Msg("Starting reminder notification scheduler")
	return p.ScheduleNextCheck(ctx)
}

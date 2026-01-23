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
	TypeScheduledTransferCheck = "scheduled:transfer:check"
	ScheduledCheckInterval     = 1 * time.Minute // Check every minute
)

// ScheduledTransferProcessor handles scheduled transfer processing
type ScheduledTransferProcessor struct {
	db              *gorm.DB
	taskDistributor tasks.TaskDistributor
}

func NewScheduledTransferProcessor(db *gorm.DB, taskDistributor tasks.TaskDistributor) *ScheduledTransferProcessor {
	return &ScheduledTransferProcessor{
		db:              db,
		taskDistributor: taskDistributor,
	}
}

// ProcessScheduledTransferCheck finds and processes scheduled transfers that are due
func (p *ScheduledTransferProcessor) ProcessScheduledTransferCheck(ctx context.Context, t *asynq.Task) error {
	var payload tasks.ScheduledTransferCheckPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %w: %w", err, asynq.SkipRetry)
	}

	log.Info().
		Str("checked_at", payload.CheckedAt).
		Msg("Checking for scheduled transfers")

	// Find all scheduled transfers that are due (scheduled_at <= now and status = scheduled)
	var scheduledTransfers []models.Transfer
	now := time.Now().Format(time.RFC3339)

	err := p.db.WithContext(ctx).
		Where("status = ?", models.TransferStatusScheduled).
		Where("scheduled_at IS NOT NULL").
		Where("scheduled_at <= ?", now).
		Find(&scheduledTransfers).Error

	if err != nil {
		log.Error().Err(err).Msg("Failed to query scheduled transfers")
		return fmt.Errorf("failed to query scheduled transfers: %w", err)
	}

	log.Info().
		Int("count", len(scheduledTransfers)).
		Msg("Found scheduled transfers to process")

	// Process each scheduled transfer
	processedCount := 0
	failedCount := 0

	for _, transfer := range scheduledTransfers {
		log.Info().
			Uint("transfer_id", transfer.ID).
			Str("scheduled_at", *transfer.ScheduledAt).
			Msg("Processing scheduled transfer")

		// Update transfer status to pending
		updateResult := p.db.WithContext(ctx).
			Model(&transfer).
			Where("id = ? AND status = ?", transfer.ID, models.TransferStatusScheduled).
			Update("status", models.TransferStatusPending)

		if updateResult.Error != nil {
			log.Error().
				Err(updateResult.Error).
				Uint("transfer_id", transfer.ID).
				Msg("Failed to update scheduled transfer status")
			failedCount++
			continue
		}

		if updateResult.RowsAffected == 0 {
			log.Warn().
				Uint("transfer_id", transfer.ID).
				Msg("Scheduled transfer already processed by another worker")
			continue
		}

		// Queue the transfer for immediate processing
		taskPayload := &tasks.ProcessTransferPayload{
			TransferID: transfer.ID,
		}

		taskOpts := []asynq.Option{
			asynq.MaxRetry(5),
			asynq.ProcessIn(1 * time.Second), // Process immediately
			asynq.Queue("critical"),
			asynq.Retention(24 * time.Hour),
		}

		err := p.taskDistributor.DistributeProcessTransferTask(ctx, taskPayload, taskOpts...)
		if err != nil {
			log.Error().
				Err(err).
				Uint("transfer_id", transfer.ID).
				Msg("Failed to queue scheduled transfer for processing")

			// Revert status back to scheduled
			p.db.WithContext(ctx).
				Model(&transfer).
				Where("id = ?", transfer.ID).
				Update("status", models.TransferStatusScheduled)

			failedCount++
			continue
		}

		processedCount++
		log.Info().
			Uint("transfer_id", transfer.ID).
			Msg("Successfully queued scheduled transfer for processing")
	}

	log.Info().
		Int("total", len(scheduledTransfers)).
		Int("processed", processedCount).
		Int("failed", failedCount).
		Msg("Scheduled transfer check completed")

	return nil
}

// ScheduleNextCheck schedules the next scheduled transfer check
func (p *ScheduledTransferProcessor) ScheduleNextCheck(ctx context.Context) error {
	payload := &tasks.ScheduledTransferCheckPayload{
		CheckedAt: time.Now().Format(time.RFC3339),
	}

	taskOpts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.ProcessIn(ScheduledCheckInterval),
		asynq.Queue("default"),
		asynq.Unique(ScheduledCheckInterval), // Prevent duplicate checks
	}

	err := p.taskDistributor.DistributeScheduledTransferCheckTask(ctx, payload, taskOpts...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to schedule next scheduled transfer check")
		return err
	}

	log.Info().
		Dur("interval", ScheduledCheckInterval).
		Msg("Scheduled next transfer check")

	return nil
}

// StartScheduledChecks initializes the scheduled transfer checking loop
func (p *ScheduledTransferProcessor) StartScheduledChecks(ctx context.Context) error {
	log.Info().Msg("Starting scheduled transfer processor")

	// Schedule the first check
	if err := p.ScheduleNextCheck(ctx); err != nil {
		return fmt.Errorf("failed to start scheduled checks: %w", err)
	}

	log.Info().Msg("Scheduled transfer processor started successfully")
	return nil
}

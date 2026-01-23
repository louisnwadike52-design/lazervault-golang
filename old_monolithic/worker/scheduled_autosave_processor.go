package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"lazervaultGo/services"
	"lazervaultGo/tasks"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	TypeScheduledAutoSaveCheck = "scheduled:autosave:check"
	ScheduledAutoSaveInterval  = 1 * time.Minute // Check every minute
)

// ScheduledAutoSaveProcessor handles scheduled auto-save processing
type ScheduledAutoSaveProcessor struct {
	db              *gorm.DB
	autoSaveService services.IAutoSaveService
	taskDistributor tasks.TaskDistributor
}

func NewScheduledAutoSaveProcessor(db *gorm.DB, autoSaveService services.IAutoSaveService, taskDistributor tasks.TaskDistributor) *ScheduledAutoSaveProcessor {
	return &ScheduledAutoSaveProcessor{
		db:              db,
		autoSaveService: autoSaveService,
		taskDistributor: taskDistributor,
	}
}

// ProcessScheduledAutoSaveCheck finds and processes scheduled auto-save rules that are due
func (p *ScheduledAutoSaveProcessor) ProcessScheduledAutoSaveCheck(ctx context.Context, t *asynq.Task) error {
	var payload tasks.ScheduledAutoSaveCheckPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %w: %w", err, asynq.SkipRetry)
	}

	log.Info().
		Str("checked_at", payload.CheckedAt).
		Msg("Checking for scheduled auto-save rules")

	// Process scheduled rules using the service method
	processedCount, failedCount, err := p.autoSaveService.ProcessScheduledRules(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to process scheduled auto-save rules")
		return fmt.Errorf("failed to process scheduled auto-save rules: %w", err)
	}

	log.Info().
		Int("processed", processedCount).
		Int("failed", failedCount).
		Msg("Scheduled auto-save check completed")

	return nil
}

// ScheduleNextCheck schedules the next scheduled auto-save check
func (p *ScheduledAutoSaveProcessor) ScheduleNextCheck(ctx context.Context) error {
	payload := &tasks.ScheduledAutoSaveCheckPayload{
		CheckedAt: time.Now().Format(time.RFC3339),
	}

	taskOpts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.ProcessIn(ScheduledAutoSaveInterval),
		asynq.Queue("default"),
		asynq.Unique(ScheduledAutoSaveInterval), // Prevent duplicate checks
	}

	err := p.taskDistributor.DistributeScheduledAutoSaveCheckTask(ctx, payload, taskOpts...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to schedule next scheduled auto-save check")
		return err
	}

	log.Info().
		Dur("interval", ScheduledAutoSaveInterval).
		Msg("Scheduled next auto-save check")

	return nil
}

// StartScheduledChecks initializes the scheduled auto-save checking loop
func (p *ScheduledAutoSaveProcessor) StartScheduledChecks(ctx context.Context) error {
	log.Info().Msg("Starting scheduled auto-save processor")

	// Schedule the first check
	if err := p.ScheduleNextCheck(ctx); err != nil {
		return fmt.Errorf("failed to start scheduled checks: %w", err)
	}

	log.Info().Msg("Scheduled auto-save processor started successfully")
	return nil
}

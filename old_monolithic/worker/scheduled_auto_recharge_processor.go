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
	AutoRechargeCheckInterval = 1 * time.Hour // Check every hour
)

// ScheduledAutoRechargeProcessor handles scheduled auto-recharge processing
type ScheduledAutoRechargeProcessor struct {
	db              *gorm.DB
	taskDistributor tasks.TaskDistributor
}

func NewScheduledAutoRechargeProcessor(db *gorm.DB, taskDistributor tasks.TaskDistributor) *ScheduledAutoRechargeProcessor {
	return &ScheduledAutoRechargeProcessor{
		db:              db,
		taskDistributor: taskDistributor,
	}
}

// ProcessAutoRechargeCheck finds and processes auto-recharges that are due
func (p *ScheduledAutoRechargeProcessor) ProcessAutoRechargeCheck(ctx context.Context, t *asynq.Task) error {
	var payload tasks.AutoRechargeCheckPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %w: %w", err, asynq.SkipRetry)
	}

	log.Info().
		Int64("check_time", payload.CheckTime).
		Msg("Checking for auto-recharges due for processing")

	// Find all active auto-recharges that are due (next_run_date <= now and status = active)
	var autoRecharges []models.AutoRecharge
	now := time.Now()

	err := p.db.WithContext(ctx).
		Preload("Beneficiary").
		Where("status = ?", models.AutoRechargeStatusActive).
		Where("next_run_date <= ?", now).
		Find(&autoRecharges).Error

	if err != nil {
		log.Error().Err(err).Msg("Failed to query auto-recharges")
		return fmt.Errorf("failed to query auto-recharges: %w", err)
	}

	log.Info().
		Int("count", len(autoRecharges)).
		Msg("Found auto-recharges to process")

	// Process each auto-recharge
	processedCount := 0
	failedCount := 0

	for _, autoRecharge := range autoRecharges {
		log.Info().
			Str("auto_recharge_id", autoRecharge.ID).
			Str("next_run_date", autoRecharge.NextRunDate.Format(time.RFC3339)).
			Float64("amount", autoRecharge.Amount).
			Msg("Processing auto-recharge")

		// Create bill payment task
		paymentPayload := tasks.BillPaymentProcessPayload{
			PaymentID:       "", // Will be created in the payment service
			ProviderCode:    autoRecharge.Beneficiary.ProviderCode,
			MeterNumber:     autoRecharge.MeterNumber,
			Amount:          autoRecharge.Amount,
			PaymentGateway:  "flutterwave", // Default, could be configurable
			ReferenceNumber: fmt.Sprintf("AUTO-%s-%d", autoRecharge.ID, time.Now().Unix()),
			IsAutoRecharge:  true,
			AutoRechargeID:  autoRecharge.ID,
		}

		// First create a payment record in the database
		payment := &models.BillPayment{
			UserID:          autoRecharge.UserID,
			ProviderID:      autoRecharge.ProviderID,
			ProviderCode:    autoRecharge.Beneficiary.ProviderCode,
			ProviderName:    autoRecharge.Beneficiary.ProviderName,
			MeterNumber:     autoRecharge.MeterNumber,
			Amount:          autoRecharge.Amount,
			Currency:        autoRecharge.Currency,
			Status:          models.BillPaymentStatusPending,
			PaymentGateway:  "flutterwave",
			ReferenceNumber: paymentPayload.ReferenceNumber,
			MeterType:       autoRecharge.Beneficiary.MeterType,
		}

		// Calculate service fee (simplified - should fetch from provider)
		payment.ServiceFee = 100 // Default fee
		payment.TotalAmount = payment.Amount + payment.ServiceFee

		if err := p.db.WithContext(ctx).Create(payment).Error; err != nil {
			log.Error().
				Err(err).
				Str("auto_recharge_id", autoRecharge.ID).
				Msg("Failed to create payment record for auto-recharge")

			// Increment failure count
			autoRecharge.FailureCount++
			if autoRecharge.FailureCount >= autoRecharge.MaxRetries {
				autoRecharge.Status = models.AutoRechargeStatusExpired
				log.Warn().
					Str("auto_recharge_id", autoRecharge.ID).
					Msg("Auto-recharge expired after max retries")
			}
			p.db.WithContext(ctx).Save(&autoRecharge)
			failedCount++
			continue
		}

		// Update payload with payment ID
		paymentPayload.PaymentID = payment.ID

		// Enqueue payment processing task
		jsonPayload, err := json.Marshal(paymentPayload)
		if err != nil {
			log.Error().
				Err(err).
				Str("auto_recharge_id", autoRecharge.ID).
				Msg("Failed to marshal payment payload")
			failedCount++
			continue
		}

		err = p.taskDistributor.DistributeTask(
			ctx,
			tasks.TaskProcessBillPayment,
			jsonPayload,
			asynq.Queue(tasks.QueueCritical),
		)

		if err != nil {
			log.Error().
				Err(err).
				Str("auto_recharge_id", autoRecharge.ID).
				Msg("Failed to enqueue bill payment task")

			// Increment failure count
			autoRecharge.FailureCount++
			if autoRecharge.FailureCount >= autoRecharge.MaxRetries {
				autoRecharge.Status = models.AutoRechargeStatusExpired
			}
			p.db.WithContext(ctx).Save(&autoRecharge)
			failedCount++
		} else {
			log.Info().
				Str("auto_recharge_id", autoRecharge.ID).
				Msg("Successfully enqueued auto-recharge payment")
			processedCount++

			// Note: The payment processor will update last_run_date and next_run_date
			// upon successful payment completion
		}
	}

	log.Info().
		Int("processed", processedCount).
		Int("failed", failedCount).
		Msg("Auto-recharge check completed")

	// Schedule next check
	return p.ScheduleNextCheck(ctx)
}

// ScheduleNextCheck schedules the next auto-recharge check
func (p *ScheduledAutoRechargeProcessor) ScheduleNextCheck(ctx context.Context) error {
	payloadBytes, err := tasks.NewAutoRechargeCheckTask(time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to create auto-recharge check payload: %w", err)
	}

	err = p.taskDistributor.DistributeTask(
		ctx,
		tasks.TaskCheckAutoRecharges,
		payloadBytes,
		asynq.Queue(tasks.QueueDefault),
		asynq.ProcessIn(AutoRechargeCheckInterval),
	)

	if err != nil {
		log.Error().Err(err).Msg("Failed to schedule next auto-recharge check")
		return err
	}

	log.Info().
		Dur("interval", AutoRechargeCheckInterval).
		Msg("Scheduled next auto-recharge check")

	return nil
}

// StartAutoRechargeScheduler initiates the periodic auto-recharge checking
func (p *ScheduledAutoRechargeProcessor) StartAutoRechargeScheduler(ctx context.Context) error {
	log.Info().Msg("Starting auto-recharge scheduler")
	return p.ScheduleNextCheck(ctx)
}

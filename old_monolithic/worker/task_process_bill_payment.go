package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/services"
	"lazervaultGo/tasks"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// HandleBillPaymentProcessTask processes electricity bill payment
func HandleBillPaymentProcessTask(
	ctx context.Context,
	t *asynq.Task,
	db *gorm.DB,
	providerFactory *services.BillPaymentProviderFactory,
	distributor tasks.TaskDistributor,
) error {
	var payload tasks.BillPaymentProcessPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal bill payment payload: %w", asynq.SkipRetry)
	}

	paymentID := payload.PaymentID
	fmt.Printf("Processing bill payment task for ID: %s\n", paymentID)

	// Get payment record
	var payment models.BillPayment
	if err := db.WithContext(ctx).Where("id = ?", paymentID).First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fmt.Printf("Bill payment record %s not found: %v\n", paymentID, err)
			return asynq.SkipRetry
		}
		return fmt.Errorf("failed to get bill payment record %s: %w", paymentID, err)
	}

	// Check if already processed
	if payment.Status != models.BillPaymentStatusPending {
		fmt.Printf("Bill payment %s already processed, status: %s\n", paymentID, payment.Status)
		return asynq.SkipRetry
	}

	// Mark as processing
	payment.Status = models.BillPaymentStatusProcessing
	if err := db.WithContext(ctx).Save(&payment).Error; err != nil {
		return fmt.Errorf("failed to mark payment %s as processing: %w", paymentID, err)
	}

	// Get payment gateway client
	client := providerFactory.GetProvider(payload.PaymentGateway)
	if client == nil {
		return handlePaymentFailure(ctx, db, &payment, "Invalid payment gateway")
	}

	// Validate meter number before payment (optional double-check)
	validateReq := services.MeterValidationRequest{
		ProviderCode: payload.ProviderCode,
		MeterNumber:  payload.MeterNumber,
		MeterType:    payment.MeterType,
	}

	validateResp, err := client.ValidateMeter(ctx, validateReq)
	if err != nil {
		return handlePaymentFailure(ctx, db, &payment, fmt.Sprintf("Meter validation failed: %v", err))
	}

	if !validateResp.IsValid {
		return handlePaymentFailure(ctx, db, &payment, "Invalid meter number")
	}

	// Update customer details from validation
	payment.CustomerName = validateResp.CustomerName
	payment.CustomerAddress = validateResp.CustomerAddress

	// Initiate payment with provider
	paymentReq := services.BillPaymentRequest{
		ProviderCode: payload.ProviderCode,
		MeterNumber:  payload.MeterNumber,
		Amount:       payload.Amount,
		Currency:     payment.Currency,
		MeterType:    payment.MeterType,
		CustomerName: validateResp.CustomerName,
		Reference:    payload.ReferenceNumber,
	}

	paymentResp, err := client.InitiatePayment(ctx, paymentReq)
	if err != nil {
		return handlePaymentFailure(ctx, db, &payment, fmt.Sprintf("Payment failed: %v", err))
	}

	if !paymentResp.Success {
		return handlePaymentFailure(ctx, db, &payment, paymentResp.Message)
	}

	// Payment successful - update record
	now := time.Now()
	payment.Status = models.BillPaymentStatusCompleted
	payment.GatewayReference = paymentResp.GatewayReference
	payment.Token = paymentResp.Token
	payment.Units = paymentResp.Units
	payment.CompletedAt = &now

	if err := db.WithContext(ctx).Save(&payment).Error; err != nil {
		return fmt.Errorf("failed to update payment %s as completed: %w", paymentID, err)
	}

	fmt.Printf("Bill payment %s completed successfully. Token: %s, Units: %.2f\n",
		paymentID, payment.Token, payment.Units)

	// TODO: Send notification to user (email/push)
	// TODO: Update transaction history

	// If this was from auto-recharge, update the auto-recharge record
	if payload.IsAutoRecharge && payload.AutoRechargeID != "" {
		updateAutoRechargeStatus(ctx, db, payload.AutoRechargeID, true)
	}

	return nil
}

// handlePaymentFailure marks payment as failed and returns SkipRetry
func handlePaymentFailure(ctx context.Context, db *gorm.DB, payment *models.BillPayment, reason string) error {
	now := time.Now()
	payment.Status = models.BillPaymentStatusFailed
	payment.FailureReason = reason
	payment.FailedAt = &now

	if err := db.WithContext(ctx).Save(payment).Error; err != nil {
		return fmt.Errorf("failed to mark payment as failed: %w", err)
	}

	fmt.Printf("Bill payment %s failed: %s\n", payment.ID, reason)
	return asynq.SkipRetry
}

// updateAutoRechargeStatus updates auto-recharge after execution
func updateAutoRechargeStatus(ctx context.Context, db *gorm.DB, autoRechargeID string, success bool) {
	var autoRecharge models.AutoRecharge
	if err := db.WithContext(ctx).Where("id = ?", autoRechargeID).First(&autoRecharge).Error; err != nil {
		fmt.Printf("Failed to get auto-recharge %s: %v\n", autoRechargeID, err)
		return
	}

	now := time.Now()
	autoRecharge.LastRunDate = &now

	if success {
		// Calculate next run date
		autoRecharge.NextRunDate = calculateNextRunDate(autoRecharge.Frequency, autoRecharge.DayOfWeek, autoRecharge.DayOfMonth)
		autoRecharge.FailureCount = 0
	} else {
		autoRecharge.FailureCount++
		if autoRecharge.FailureCount >= autoRecharge.MaxRetries {
			autoRecharge.Status = models.AutoRechargeStatusExpired
		}
	}

	if err := db.WithContext(ctx).Save(&autoRecharge).Error; err != nil {
		fmt.Printf("Failed to update auto-recharge %s: %v\n", autoRechargeID, err)
	}
}

// calculateNextRunDate computes the next run date based on frequency
func calculateNextRunDate(frequency models.RechargeFrequency, dayOfWeek, dayOfMonth *int) time.Time {
	now := time.Now()

	switch frequency {
	case models.FrequencyDaily:
		return now.Add(24 * time.Hour)

	case models.FrequencyWeekly:
		if dayOfWeek == nil {
			return now.Add(7 * 24 * time.Hour)
		}
		targetDay := *dayOfWeek
		currentDay := int(now.Weekday())
		daysUntil := (targetDay - currentDay + 7) % 7
		if daysUntil == 0 {
			daysUntil = 7
		}
		return now.Add(time.Duration(daysUntil) * 24 * time.Hour)

	case models.FrequencyMonthly:
		if dayOfMonth == nil {
			return now.AddDate(0, 1, 0)
		}
		nextMonth := now.AddDate(0, 1, 0)
		day := *dayOfMonth
		// Ensure day is valid for the month
		daysInMonth := time.Date(nextMonth.Year(), nextMonth.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if day > daysInMonth {
			day = daysInMonth
		}
		return time.Date(nextMonth.Year(), nextMonth.Month(), day, now.Hour(), now.Minute(), 0, 0, now.Location())

	default:
		return now.Add(24 * time.Hour)
	}
}

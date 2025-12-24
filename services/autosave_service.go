package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/tasks"
	"math"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// Auto-Save Service Errors
var (
	ErrAutoSaveRuleNotFound       = errors.New("autosave service: rule not found")
	ErrAutoSaveRuleAccessDenied   = errors.New("autosave service: access denied to rule")
	ErrAutoSaveInvalidTriggerType = errors.New("autosave service: invalid trigger type")
	ErrAutoSaveInvalidAmountType  = errors.New("autosave service: invalid amount type")
	ErrAutoSaveInvalidStatus      = errors.New("autosave service: invalid status")
	ErrAutoSaveInvalidAction      = errors.New("autosave service: invalid action")
	ErrAutoSaveGoalReached        = errors.New("autosave service: savings goal already reached")
	ErrAutoSaveInsufficientFunds  = errors.New("autosave service: insufficient funds in source account")
	ErrAutoSaveMinBalanceViolation = errors.New("autosave service: operation would violate minimum balance")
)

// IAutoSaveService defines the interface for auto-save operations
type IAutoSaveService interface {
	CreateAutoSaveRule(ctx context.Context, userID uint, req *CreateAutoSaveRuleRequest) (*models.AutoSaveRule, error)
	GetAutoSaveRules(ctx context.Context, userID uint, accountID *uint, status *models.AutoSaveStatus) ([]*models.AutoSaveRule, error)
	GetAutoSaveRuleByID(ctx context.Context, ruleID uint, userID uint) (*models.AutoSaveRule, error)
	UpdateAutoSaveRule(ctx context.Context, ruleID uint, userID uint, req *UpdateAutoSaveRuleRequest) (*models.AutoSaveRule, error)
	ToggleAutoSaveRule(ctx context.Context, ruleID uint, userID uint, action string) (*models.AutoSaveRule, error)
	DeleteAutoSaveRule(ctx context.Context, ruleID uint, userID uint) error
	GetAutoSaveTransactions(ctx context.Context, userID uint, ruleID *uint, accountID *uint, limit, offset int) ([]*models.AutoSaveTransaction, int64, error)
	GetAutoSaveStatistics(ctx context.Context, userID uint) (*AutoSaveStatistics, error)
	TriggerAutoSave(ctx context.Context, ruleID uint, userID uint, customAmount *float64) (*models.AutoSaveTransaction, error)
	ProcessOnDepositTrigger(ctx context.Context, userID uint, accountID uint, depositAmount float64) error
	ProcessScheduledRules(ctx context.Context) (processedCount int, failedCount int, err error)
	ProcessRoundUpTransaction(ctx context.Context, userID uint, accountID uint, transactionAmount float64) error
}

// AutoSaveService handles business logic for auto-save operations
type AutoSaveService struct {
	db             *gorm.DB
	distributor    tasks.TaskDistributor
	accountService IAccountService
	transferService ITransferService
}

// Request/Response types
type CreateAutoSaveRuleRequest struct {
	Name                 string
	Description          string
	TriggerType          models.TriggerType
	AmountType           models.AmountType
	AmountValue          float64
	SourceAccountID      uint
	DestinationAccountID uint
	Frequency            *models.ScheduleFrequency
	ScheduleTime         *string
	ScheduleDay          *int
	RoundUpTo            *int
	TargetAmount         *float64
	MinimumBalance       *float64
	MaximumPerSave       *float64
}

type UpdateAutoSaveRuleRequest struct {
	Name           *string
	Description    *string
	AmountType     *models.AmountType
	AmountValue    *float64
	Frequency      *models.ScheduleFrequency
	ScheduleTime   *string
	ScheduleDay    *int
	RoundUpTo      *int
	TargetAmount   *float64
	MinimumBalance *float64
	MaximumPerSave *float64
}

type AutoSaveStatistics struct {
	UserID              uint
	ActiveRulesCount    int64
	TotalSavedAllTime   float64
	TotalSavedThisMonth float64
	TotalSavedThisWeek  float64
	TotalTransactions   int64
	AverageSaveAmount   float64
	MostActiveRule      *models.AutoSaveRule
}

// NewAutoSaveService creates a new AutoSaveService
func NewAutoSaveService(db *gorm.DB, distributor tasks.TaskDistributor, accountService IAccountService, transferService ITransferService) IAutoSaveService {
	return &AutoSaveService{
		db:             db,
		distributor:    distributor,
		accountService: accountService,
		transferService: transferService,
	}
}

// CreateAutoSaveRule creates a new auto-save rule
func (s *AutoSaveService) CreateAutoSaveRule(ctx context.Context, userID uint, req *CreateAutoSaveRuleRequest) (*models.AutoSaveRule, error) {
	// Validate accounts ownership
	if err := s.accountService.CheckAccountOwnership(ctx, req.SourceAccountID, userID); err != nil {
		return nil, err
	}
	if err := s.accountService.CheckAccountOwnership(ctx, req.DestinationAccountID, userID); err != nil {
		return nil, err
	}

	// Validate trigger type specific fields
	if req.TriggerType == models.TriggerTypeScheduled && req.Frequency == nil {
		return nil, errors.New("frequency is required for scheduled triggers")
	}
	if req.TriggerType == models.TriggerTypeRoundUp && req.RoundUpTo == nil {
		return nil, errors.New("round_up_to is required for round-up triggers")
	}

	rule := &models.AutoSaveRule{
		UserID:               userID,
		Name:                 req.Name,
		Description:          req.Description,
		TriggerType:          req.TriggerType,
		AmountType:           req.AmountType,
		AmountValue:          req.AmountValue,
		SourceAccountID:      req.SourceAccountID,
		DestinationAccountID: req.DestinationAccountID,
		Status:               models.AutoSaveStatusActive,
		Frequency:            req.Frequency,
		ScheduleTime:         req.ScheduleTime,
		ScheduleDay:          req.ScheduleDay,
		RoundUpTo:            req.RoundUpTo,
		TargetAmount:         req.TargetAmount,
		MinimumBalance:       req.MinimumBalance,
		MaximumPerSave:       req.MaximumPerSave,
		TriggerCount:         0,
		TotalSaved:           0,
	}

	if err := s.db.Create(rule).Error; err != nil {
		return nil, fmt.Errorf("failed to create auto-save rule: %w", err)
	}

	// Preload associations
	if err := s.db.Preload("SourceAccount").Preload("DestinationAccount").First(rule, rule.ID).Error; err != nil {
		return nil, err
	}

	return rule, nil
}

// GetAutoSaveRules retrieves auto-save rules with optional filters
func (s *AutoSaveService) GetAutoSaveRules(ctx context.Context, userID uint, accountID *uint, status *models.AutoSaveStatus) ([]*models.AutoSaveRule, error) {
	var rules []*models.AutoSaveRule
	query := s.db.Where("user_id = ?", userID)

	if accountID != nil {
		query = query.Where("source_account_id = ? OR destination_account_id = ?", *accountID, *accountID)
	}

	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if err := query.Preload("SourceAccount").Preload("DestinationAccount").Order("created_at DESC").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch auto-save rules: %w", err)
	}

	return rules, nil
}

// GetAutoSaveRuleByID retrieves a specific auto-save rule
func (s *AutoSaveService) GetAutoSaveRuleByID(ctx context.Context, ruleID uint, userID uint) (*models.AutoSaveRule, error) {
	var rule models.AutoSaveRule
	if err := s.db.Preload("SourceAccount").Preload("DestinationAccount").Where("id = ? AND user_id = ?", ruleID, userID).First(&rule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAutoSaveRuleNotFound
		}
		return nil, err
	}
	return &rule, nil
}

// UpdateAutoSaveRule updates an existing auto-save rule
func (s *AutoSaveService) UpdateAutoSaveRule(ctx context.Context, ruleID uint, userID uint, req *UpdateAutoSaveRuleRequest) (*models.AutoSaveRule, error) {
	rule, err := s.GetAutoSaveRuleByID(ctx, ruleID, userID)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.AmountType != nil {
		updates["amount_type"] = *req.AmountType
	}
	if req.AmountValue != nil {
		updates["amount_value"] = *req.AmountValue
	}
	if req.Frequency != nil {
		updates["frequency"] = *req.Frequency
	}
	if req.ScheduleTime != nil {
		updates["schedule_time"] = *req.ScheduleTime
	}
	if req.ScheduleDay != nil {
		updates["schedule_day"] = *req.ScheduleDay
	}
	if req.RoundUpTo != nil {
		updates["round_up_to"] = *req.RoundUpTo
	}
	if req.TargetAmount != nil {
		updates["target_amount"] = *req.TargetAmount
	}
	if req.MinimumBalance != nil {
		updates["minimum_balance"] = *req.MinimumBalance
	}
	if req.MaximumPerSave != nil {
		updates["maximum_per_save"] = *req.MaximumPerSave
	}

	if err := s.db.Model(rule).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update auto-save rule: %w", err)
	}

	// Reload with associations
	if err := s.db.Preload("SourceAccount").Preload("DestinationAccount").First(rule, ruleID).Error; err != nil {
		return nil, err
	}

	return rule, nil
}

// ToggleAutoSaveRule pauses or resumes an auto-save rule
func (s *AutoSaveService) ToggleAutoSaveRule(ctx context.Context, ruleID uint, userID uint, action string) (*models.AutoSaveRule, error) {
	rule, err := s.GetAutoSaveRuleByID(ctx, ruleID, userID)
	if err != nil {
		return nil, err
	}

	var newStatus models.AutoSaveStatus
	switch action {
	case "pause":
		if rule.Status == models.AutoSaveStatusActive {
			newStatus = models.AutoSaveStatusPaused
		} else {
			return nil, errors.New("can only pause active rules")
		}
	case "resume":
		if rule.Status == models.AutoSaveStatusPaused {
			newStatus = models.AutoSaveStatusActive
		} else {
			return nil, errors.New("can only resume paused rules")
		}
	case "complete":
		newStatus = models.AutoSaveStatusCompleted
	case "cancel":
		newStatus = models.AutoSaveStatusCancelled
	default:
		return nil, ErrAutoSaveInvalidAction
	}

	if err := s.db.Model(rule).Update("status", newStatus).Error; err != nil {
		return nil, fmt.Errorf("failed to toggle auto-save rule: %w", err)
	}

	rule.Status = newStatus
	return rule, nil
}

// DeleteAutoSaveRule deletes an auto-save rule
func (s *AutoSaveService) DeleteAutoSaveRule(ctx context.Context, ruleID uint, userID uint) error {
	rule, err := s.GetAutoSaveRuleByID(ctx, ruleID, userID)
	if err != nil {
		return err
	}

	if err := s.db.Delete(rule).Error; err != nil {
		return fmt.Errorf("failed to delete auto-save rule: %w", err)
	}

	return nil
}

// GetAutoSaveTransactions retrieves transaction history
func (s *AutoSaveService) GetAutoSaveTransactions(ctx context.Context, userID uint, ruleID *uint, accountID *uint, limit, offset int) ([]*models.AutoSaveTransaction, int64, error) {
	var transactions []*models.AutoSaveTransaction
	var total int64

	query := s.db.Where("user_id = ?", userID)

	if ruleID != nil {
		query = query.Where("rule_id = ?", *ruleID)
	}

	if accountID != nil {
		query = query.Where("source_account_id = ? OR destination_account_id = ?", *accountID, *accountID)
	}

	// Get total count
	if err := query.Model(&models.AutoSaveTransaction{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	if err := query.Preload("Rule").Preload("SourceAccount").Preload("DestinationAccount").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch auto-save transactions: %w", err)
	}

	return transactions, total, nil
}

// GetAutoSaveStatistics calculates and returns auto-save statistics
func (s *AutoSaveService) GetAutoSaveStatistics(ctx context.Context, userID uint) (*AutoSaveStatistics, error) {
	stats := &AutoSaveStatistics{
		UserID: userID,
	}

	// Count active rules
	if err := s.db.Model(&models.AutoSaveRule{}).Where("user_id = ? AND status = ?", userID, models.AutoSaveStatusActive).Count(&stats.ActiveRulesCount).Error; err != nil {
		return nil, err
	}

	// Calculate total saved all time
	if err := s.db.Model(&models.AutoSaveRule{}).Where("user_id = ?", userID).Select("COALESCE(SUM(total_saved), 0)").Scan(&stats.TotalSavedAllTime).Error; err != nil {
		return nil, err
	}

	// Calculate total saved this month
	startOfMonth := time.Now().AddDate(0, 0, -time.Now().Day()+1)
	if err := s.db.Model(&models.AutoSaveTransaction{}).
		Where("user_id = ? AND success = ? AND created_at >= ?", userID, true, startOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&stats.TotalSavedThisMonth).Error; err != nil {
		return nil, err
	}

	// Calculate total saved this week
	startOfWeek := time.Now().AddDate(0, 0, -int(time.Now().Weekday()))
	if err := s.db.Model(&models.AutoSaveTransaction{}).
		Where("user_id = ? AND success = ? AND created_at >= ?", userID, true, startOfWeek).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&stats.TotalSavedThisWeek).Error; err != nil {
		return nil, err
	}

	// Count total transactions
	if err := s.db.Model(&models.AutoSaveTransaction{}).Where("user_id = ? AND success = ?", userID, true).Count(&stats.TotalTransactions).Error; err != nil {
		return nil, err
	}

	// Calculate average save amount
	if stats.TotalTransactions > 0 {
		stats.AverageSaveAmount = stats.TotalSavedAllTime / float64(stats.TotalTransactions)
	}

	// Find most active rule
	var mostActiveRule models.AutoSaveRule
	if err := s.db.Where("user_id = ?", userID).Order("trigger_count DESC").First(&mostActiveRule).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else {
		stats.MostActiveRule = &mostActiveRule
	}

	return stats, nil
}

// TriggerAutoSave manually triggers an auto-save rule
func (s *AutoSaveService) TriggerAutoSave(ctx context.Context, ruleID uint, userID uint, customAmount *float64) (*models.AutoSaveTransaction, error) {
	rule, err := s.GetAutoSaveRuleByID(ctx, ruleID, userID)
	if err != nil {
		return nil, err
	}

	if rule.Status != models.AutoSaveStatusActive {
		return nil, errors.New("can only trigger active rules")
	}

	// Check if goal is reached
	if rule.TargetAmount != nil && rule.TotalSaved >= *rule.TargetAmount {
		return nil, ErrAutoSaveGoalReached
	}

	// Calculate amount to save
	amount := rule.AmountValue
	if customAmount != nil {
		amount = *customAmount
	}

	// Apply maximum per save limit
	if rule.MaximumPerSave != nil && amount > *rule.MaximumPerSave {
		amount = *rule.MaximumPerSave
	}

	// Execute the save
	transaction, err := s.executeSave(ctx, rule, amount, "manual_trigger")
	if err != nil {
		return nil, err
	}

	return transaction, nil
}

// ProcessOnDepositTrigger processes auto-save rules triggered by deposits
func (s *AutoSaveService) ProcessOnDepositTrigger(ctx context.Context, userID uint, accountID uint, depositAmount float64) error {
	log.Info().
		Uint("user_id", userID).
		Uint("account_id", accountID).
		Float64("deposit_amount", depositAmount).
		Msg("Processing auto-save triggers for deposit")

	var rules []*models.AutoSaveRule
	if err := s.db.Where("user_id = ? AND source_account_id = ? AND trigger_type = ? AND status = ?",
		userID, accountID, models.TriggerTypeOnDeposit, models.AutoSaveStatusActive).
		Preload("SourceAccount").Preload("DestinationAccount").
		Find(&rules).Error; err != nil {
		log.Error().Err(err).
			Uint("user_id", userID).
			Uint("account_id", accountID).
			Msg("Failed to query auto-save rules for deposit trigger")
		return err
	}

	log.Info().
		Int("rules_count", len(rules)).
		Uint("user_id", userID).
		Msg("Found active auto-save rules for deposit trigger")

	processedCount := 0
	failedCount := 0

	for _, rule := range rules {
		// Check if goal is reached
		if rule.TargetAmount != nil && rule.TotalSaved >= *rule.TargetAmount {
			log.Info().
				Uint("rule_id", rule.ID).
				Str("rule_name", rule.Name).
				Float64("target_amount", *rule.TargetAmount).
				Float64("total_saved", rule.TotalSaved).
				Msg("Auto-save rule goal reached, marking as completed")
			// Mark as completed
			s.db.Model(rule).Update("status", models.AutoSaveStatusCompleted)
			continue
		}

		// Calculate amount based on type
		var amount float64
		if rule.AmountType == models.AmountTypeFixed {
			amount = rule.AmountValue
		} else if rule.AmountType == models.AmountTypePercentage {
			amount = depositAmount * (rule.AmountValue / 100.0)
		}

		// Apply maximum per save limit
		if rule.MaximumPerSave != nil && amount > *rule.MaximumPerSave {
			log.Info().
				Uint("rule_id", rule.ID).
				Float64("calculated_amount", amount).
				Float64("maximum_per_save", *rule.MaximumPerSave).
				Msg("Auto-save amount capped at maximum per save limit")
			amount = *rule.MaximumPerSave
		}

		log.Info().
			Uint("rule_id", rule.ID).
			Str("rule_name", rule.Name).
			Float64("save_amount", amount).
			Msg("Executing auto-save for deposit trigger")

		// Execute the save
		_, err := s.executeSave(ctx, rule, amount, fmt.Sprintf("deposit of %.2f triggered", depositAmount))
		if err != nil {
			log.Error().Err(err).
				Uint("rule_id", rule.ID).
				Str("rule_name", rule.Name).
				Float64("amount", amount).
				Msg("Failed to execute auto-save for rule")
			failedCount++
		} else {
			log.Info().
				Uint("rule_id", rule.ID).
				Str("rule_name", rule.Name).
				Float64("amount", amount).
				Msg("Auto-save executed successfully")
			processedCount++
		}
	}

	log.Info().
		Int("processed", processedCount).
		Int("failed", failedCount).
		Int("total_rules", len(rules)).
		Uint("user_id", userID).
		Msg("Completed processing auto-save deposit triggers")

	return nil
}

// ProcessScheduledRules processes all scheduled auto-save rules
func (s *AutoSaveService) ProcessScheduledRules(ctx context.Context) (processedCount int, failedCount int, err error) {
	now := time.Now()

	log.Info().
		Time("check_time", now).
		Msg("Starting scheduled auto-save rules processing")

	var rules []*models.AutoSaveRule

	// Get all active scheduled rules
	if err := s.db.Where("trigger_type = ? AND status = ?", models.TriggerTypeScheduled, models.AutoSaveStatusActive).
		Preload("SourceAccount").Preload("DestinationAccount").
		Find(&rules).Error; err != nil {
		log.Error().Err(err).Msg("Failed to query scheduled auto-save rules")
		return 0, 0, err
	}

	log.Info().
		Int("rules_count", len(rules)).
		Msg("Found active scheduled auto-save rules")

	processedCount = 0
	failedCount = 0
	skippedCount := 0

	for _, rule := range rules {
		// Check if it's time to trigger
		if !s.shouldTriggerScheduledRule(rule, now) {
			skippedCount++
			continue
		}

		// Check if goal is reached
		if rule.TargetAmount != nil && rule.TotalSaved >= *rule.TargetAmount {
			log.Info().
				Uint("rule_id", rule.ID).
				Str("rule_name", rule.Name).
				Float64("target_amount", *rule.TargetAmount).
				Float64("total_saved", rule.TotalSaved).
				Msg("Scheduled auto-save rule goal reached, marking as completed")
			s.db.Model(rule).Update("status", models.AutoSaveStatusCompleted)
			continue
		}

		log.Info().
			Uint("rule_id", rule.ID).
			Str("rule_name", rule.Name).
			Float64("amount", rule.AmountValue).
			Msg("Executing scheduled auto-save")

		// Execute the save
		_, err := s.executeSave(ctx, rule, rule.AmountValue, "scheduled trigger")
		if err != nil {
			log.Error().Err(err).
				Uint("rule_id", rule.ID).
				Str("rule_name", rule.Name).
				Float64("amount", rule.AmountValue).
				Msg("Failed to execute scheduled auto-save for rule")
			failedCount++
		} else {
			log.Info().
				Uint("rule_id", rule.ID).
				Str("rule_name", rule.Name).
				Float64("amount", rule.AmountValue).
				Msg("Scheduled auto-save executed successfully")
			processedCount++
		}
	}

	log.Info().
		Int("processed", processedCount).
		Int("failed", failedCount).
		Int("skipped", skippedCount).
		Int("total_rules", len(rules)).
		Msg("Completed processing scheduled auto-save rules")

	return processedCount, failedCount, nil
}

// ProcessRoundUpTransaction processes round-up for a transaction
func (s *AutoSaveService) ProcessRoundUpTransaction(ctx context.Context, userID uint, accountID uint, transactionAmount float64) error {
	var rules []*models.AutoSaveRule
	if err := s.db.Where("user_id = ? AND source_account_id = ? AND trigger_type = ? AND status = ?",
		userID, accountID, models.TriggerTypeRoundUp, models.AutoSaveStatusActive).
		Preload("SourceAccount").Preload("DestinationAccount").
		Find(&rules).Error; err != nil {
		return err
	}

	for _, rule := range rules {
		if rule.RoundUpTo == nil {
			continue
		}

		// Calculate round-up amount
		roundUpTo := float64(*rule.RoundUpTo)
		roundedAmount := math.Ceil(transactionAmount/roundUpTo) * roundUpTo
		roundUpAmount := roundedAmount - transactionAmount

		if roundUpAmount <= 0 {
			continue
		}

		// Check if goal is reached
		if rule.TargetAmount != nil && rule.TotalSaved >= *rule.TargetAmount {
			s.db.Model(rule).Update("status", models.AutoSaveStatusCompleted)
			continue
		}

		// Execute the save
		_, err := s.executeSave(ctx, rule, roundUpAmount, fmt.Sprintf("round-up from %.2f to %.2f", transactionAmount, roundedAmount))
		if err != nil {
			fmt.Printf("Failed to execute round-up auto-save for rule %d: %v\n", rule.ID, err)
		}
	}

	return nil
}

// executeSave performs the actual save operation
func (s *AutoSaveService) executeSave(ctx context.Context, rule *models.AutoSaveRule, amount float64, triggerReason string) (*models.AutoSaveTransaction, error) {
	transaction := &models.AutoSaveTransaction{
		RuleID:               rule.ID,
		UserID:               rule.UserID,
		SourceAccountID:      rule.SourceAccountID,
		DestinationAccountID: rule.DestinationAccountID,
		Amount:               amount,
		TriggerType:          rule.TriggerType,
		TriggerReason:        triggerReason,
		Success:              false,
	}

	// Start database transaction
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Check minimum balance
		if rule.MinimumBalance != nil {
			var sourceAccount models.Account
			if err := tx.First(&sourceAccount, rule.SourceAccountID).Error; err != nil {
				return err
			}

			balanceAfterSave := float64(sourceAccount.Balance) - amount
			if balanceAfterSave < *rule.MinimumBalance {
				return ErrAutoSaveMinBalanceViolation
			}
		}

		// Check sufficient funds
		if err := s.accountService.CheckSufficientBalance(ctx, tx, rule.SourceAccountID, int64(amount)); err != nil {
			return ErrAutoSaveInsufficientFunds
		}

		// Deduct from source account
		if err := tx.Model(&models.Account{}).Where("id = ?", rule.SourceAccountID).
			Update("balance", gorm.Expr("balance - ?", int64(amount))).Error; err != nil {
			return err
		}

		// Add to destination account
		if err := tx.Model(&models.Account{}).Where("id = ?", rule.DestinationAccountID).
			Update("balance", gorm.Expr("balance + ?", int64(amount))).Error; err != nil {
			return err
		}

		// Update rule statistics
		now := time.Now()
		if err := tx.Model(rule).Updates(map[string]interface{}{
			"trigger_count":      gorm.Expr("trigger_count + ?", 1),
			"total_saved":        gorm.Expr("total_saved + ?", amount),
			"last_triggered_at":  &now,
		}).Error; err != nil {
			return err
		}

		// Mark transaction as successful
		transaction.Success = true

		// Save transaction record
		if err := tx.Create(transaction).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		// Save failed transaction record
		errorMsg := err.Error()
		transaction.ErrorMessage = &errorMsg
		s.db.Create(transaction)
		return nil, err
	}

	return transaction, nil
}

// shouldTriggerScheduledRule checks if a scheduled rule should be triggered now
func (s *AutoSaveService) shouldTriggerScheduledRule(rule *models.AutoSaveRule, now time.Time) bool {
	if rule.Frequency == nil {
		return false
	}

	// Check if already triggered recently
	if rule.LastTriggeredAt != nil {
		switch *rule.Frequency {
		case models.ScheduleFrequencyDaily:
			if now.Sub(*rule.LastTriggeredAt) < 24*time.Hour {
				return false
			}
		case models.ScheduleFrequencyWeekly:
			if now.Sub(*rule.LastTriggeredAt) < 7*24*time.Hour {
				return false
			}
		case models.ScheduleFrequencyBiweekly:
			if now.Sub(*rule.LastTriggeredAt) < 14*24*time.Hour {
				return false
			}
		case models.ScheduleFrequencyMonthly:
			if now.Sub(*rule.LastTriggeredAt) < 30*24*time.Hour {
				return false
			}
		}
	}

	// Check schedule time if specified
	if rule.ScheduleTime != nil {
		// Parse schedule time (HH:MM format)
		// Compare with current time
		// This is a simplified check - you may want to use a more robust time comparison
	}

	// Check schedule day if specified
	if rule.ScheduleDay != nil {
		switch *rule.Frequency {
		case models.ScheduleFrequencyWeekly, models.ScheduleFrequencyBiweekly:
			// *rule.ScheduleDay should be 1-7 (Monday-Sunday)
			if int(now.Weekday()) != *rule.ScheduleDay {
				return false
			}
		case models.ScheduleFrequencyMonthly:
			// *rule.ScheduleDay should be 1-31
			if now.Day() != *rule.ScheduleDay {
				return false
			}
		}
	}

	return true
}

// ConvertAutoSaveRuleToProto converts a GORM model to Protobuf
func ConvertAutoSaveRuleToProto(rule *models.AutoSaveRule) *pb.AutoSaveRule {
	protoRule := &pb.AutoSaveRule{
		Id:                   fmt.Sprintf("%d", rule.ID),
		UserId:               fmt.Sprintf("%d", rule.UserID),
		Name:                 rule.Name,
		Description:          rule.Description,
		TriggerType:          convertTriggerTypeToProto(rule.TriggerType),
		AmountType:           convertAmountTypeToProto(rule.AmountType),
		AmountValue:          rule.AmountValue,
		SourceAccountId:      fmt.Sprintf("%d", rule.SourceAccountID),
		DestinationAccountId: fmt.Sprintf("%d", rule.DestinationAccountID),
		Status:               convertAutoSaveStatusToProto(rule.Status),
		TriggerCount:         int32(rule.TriggerCount),
		TotalSaved:           rule.TotalSaved,
		CreatedAt:            timestamppb.New(rule.CreatedAt),
		UpdatedAt:            timestamppb.New(rule.UpdatedAt),
	}

	if rule.Frequency != nil {
		protoRule.Frequency = convertScheduleFrequencyToProto(*rule.Frequency)
	}
	if rule.ScheduleTime != nil {
		protoRule.ScheduleTime = *rule.ScheduleTime
	}
	if rule.ScheduleDay != nil {
		protoRule.ScheduleDay = int32(*rule.ScheduleDay)
	}
	if rule.RoundUpTo != nil {
		protoRule.RoundUpTo = int32(*rule.RoundUpTo)
	}
	if rule.TargetAmount != nil {
		protoRule.TargetAmount = *rule.TargetAmount
	}
	if rule.MinimumBalance != nil {
		protoRule.MinimumBalance = *rule.MinimumBalance
	}
	if rule.MaximumPerSave != nil {
		protoRule.MaximumPerSave = *rule.MaximumPerSave
	}
	if rule.LastTriggeredAt != nil {
		protoRule.LastTriggeredAt = timestamppb.New(*rule.LastTriggeredAt)
	}

	return protoRule
}

// Helper functions for enum conversion
func convertTriggerTypeToProto(t models.TriggerType) pb.TriggerType {
	switch t {
	case models.TriggerTypeOnDeposit:
		return pb.TriggerType_TRIGGER_ON_DEPOSIT
	case models.TriggerTypeScheduled:
		return pb.TriggerType_TRIGGER_SCHEDULED
	case models.TriggerTypeRoundUp:
		return pb.TriggerType_TRIGGER_ROUND_UP
	default:
		return pb.TriggerType_TRIGGER_UNKNOWN
	}
}

func convertAmountTypeToProto(t models.AmountType) pb.AmountType {
	switch t {
	case models.AmountTypeFixed:
		return pb.AmountType_AMOUNT_FIXED
	case models.AmountTypePercentage:
		return pb.AmountType_AMOUNT_PERCENTAGE
	default:
		return pb.AmountType_AMOUNT_UNKNOWN
	}
}

func convertAutoSaveStatusToProto(s models.AutoSaveStatus) pb.AutoSaveStatus {
	switch s {
	case models.AutoSaveStatusActive:
		return pb.AutoSaveStatus_STATUS_ACTIVE
	case models.AutoSaveStatusPaused:
		return pb.AutoSaveStatus_STATUS_PAUSED
	case models.AutoSaveStatusCompleted:
		return pb.AutoSaveStatus_STATUS_COMPLETED
	case models.AutoSaveStatusCancelled:
		return pb.AutoSaveStatus_STATUS_CANCELLED
	default:
		return pb.AutoSaveStatus_STATUS_UNKNOWN
	}
}

func convertScheduleFrequencyToProto(f models.ScheduleFrequency) pb.ScheduleFrequency {
	switch f {
	case models.ScheduleFrequencyDaily:
		return pb.ScheduleFrequency_FREQUENCY_DAILY
	case models.ScheduleFrequencyWeekly:
		return pb.ScheduleFrequency_FREQUENCY_WEEKLY
	case models.ScheduleFrequencyBiweekly:
		return pb.ScheduleFrequency_FREQUENCY_BIWEEKLY
	case models.ScheduleFrequencyMonthly:
		return pb.ScheduleFrequency_FREQUENCY_MONTHLY
	default:
		return pb.ScheduleFrequency_FREQUENCY_UNKNOWN
	}
}

// ConvertAutoSaveTransactionToProto converts a GORM model to Protobuf
func ConvertAutoSaveTransactionToProto(tx *models.AutoSaveTransaction) *pb.AutoSaveTransaction {
	protoTx := &pb.AutoSaveTransaction{
		Id:                   fmt.Sprintf("%d", tx.ID),
		RuleId:               fmt.Sprintf("%d", tx.RuleID),
		UserId:               fmt.Sprintf("%d", tx.UserID),
		SourceAccountId:      fmt.Sprintf("%d", tx.SourceAccountID),
		DestinationAccountId: fmt.Sprintf("%d", tx.DestinationAccountID),
		Amount:               tx.Amount,
		TriggerType:          convertTriggerTypeToProto(tx.TriggerType),
		TriggerReason:        tx.TriggerReason,
		Success:              tx.Success,
		CreatedAt:            timestamppb.New(tx.CreatedAt),
	}

	if tx.ErrorMessage != nil {
		protoTx.ErrorMessage = *tx.ErrorMessage
	}

	return protoTx
}

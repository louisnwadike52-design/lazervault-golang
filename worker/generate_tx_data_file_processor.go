package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/services"
	"lazervaultGo/tasks"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause" // Import for OnConflict
)

// GenerateTxDataFileProcessor handles tasks of type tasks.TypeGenerateTxDataFile
type GenerateTxDataFileProcessor struct {
	db     *gorm.DB
	config configs.Config
	// Service is embedded directly for now. Could inject an interface later.
	txFileService services.GenerateTxDataService
}

// NewGenerateTxDataFileProcessor creates a new processor for generating tx data files.
func NewGenerateTxDataFileProcessor(db *gorm.DB, config configs.Config) *GenerateTxDataFileProcessor {
	return &GenerateTxDataFileProcessor{
		db:     db,
		config: config,
		// Initialize the embedded service
		txFileService: *services.NewGenerateTxDataService(db, config),
	}
}

// ProcessTask implements the tasks.TaskProcessor interface.
func (p *GenerateTxDataFileProcessor) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload tasks.GenerateTxDataFilePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		// Use standard logger if zerolog isn't setup here
		fmt.Printf("ERROR: Failed to unmarshal payload for task type %s: %v\n", task.Type(), err)
		// Don't retry if payload is invalid
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}

	userID := payload.UserID // Get the uint UserID
	userIDStr := fmt.Sprintf("%d", userID)
	log.Info().Str("task_type", task.Type()).Str("user_id", userIDStr).Msg("processing generate tx data file task")

	// --- Core Logic: Fetch, Format, Upload --- //

	// 1. Find user accounts (needed by fetchAllUserTransactions)
	var userAccounts []models.Account
	if err := p.db.WithContext(ctx).Where("owner_user_id = ?", payload.UserID).Find(&userAccounts).Error; err != nil {
		// Log error but potentially continue if user *might* exist without accounts? Unlikely.
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("failed to query user accounts for tx file generation")
		// Retryable error
		return fmt.Errorf("failed to query user accounts for user %d: %w", payload.UserID, err)
	}
	if len(userAccounts) == 0 {
		log.Warn().Uint("user_id", payload.UserID).Msg("user has no accounts, skipping tx file generation task")
		return nil // No accounts, nothing to generate, task successful
	}
	var accountIDs []uint
	for _, acc := range userAccounts {
		accountIDs = append(accountIDs, acc.ID)
	}

	// 2. Fetch all relevant transactions using the service's method
	allRecords, err := p.txFileService.FetchAllUserTransactions(ctx, accountIDs)
	if err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("failed to fetch transactions for tx file generation")
		// Retryable error
		return fmt.Errorf("failed to fetch transactions for user %d: %w", payload.UserID, err)
	}

	if len(allRecords) == 0 {
		log.Info().Uint("user_id", payload.UserID).Msg("no transactions found, creating empty tx file")
	}

	// 3. Format data as CSV using the service's method
	csvData, err := p.txFileService.FormatTransactionsToCSV(allRecords)
	if err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("failed to format transactions to CSV for tx file generation")
		// Don't retry if formatting fails
		return fmt.Errorf("failed to format data to CSV: %w", asynq.SkipRetry)
	}

	// 4. Upload/Overwrite to GCS using the service's method
	objectPath := fmt.Sprintf("user-tx-data/%s/transactions.csv", userIDStr) // Relative path
	if err := p.txFileService.UploadOrOverwriteTxFile(ctx, userIDStr, csvData); err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("failed to upload tx file to GCS")
		return fmt.Errorf("failed to upload file to GCS for user %d: %w", payload.UserID, err)
	}

	// 5. Construct Public HTTPS URL (ASSUMES OBJECT IS PUBLICLY READABLE IN GCP)
	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", p.config.GCSBucketName, objectPath)

	// 6. Store/Update the Public URL in the database
	fileRecord := models.UserTransactionFile{
		UserID:   userID,
		FilePath: publicURL, // Store the public URL
	}

	// Use Clauses(clause.OnConflict) for upsert
	if errDb := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"file_path", "updated_at"}),
	}).Create(&fileRecord).Error; errDb != nil {
		log.Error().Err(errDb).Uint("user_id", userID).Str("public_url", publicURL).Msg("failed to save/update user transaction file public URL in DB")
	}

	log.Info().Str("task_type", task.Type()).Str("user_id", userIDStr).Msg("generate tx data file task completed successfully")
	return nil
}

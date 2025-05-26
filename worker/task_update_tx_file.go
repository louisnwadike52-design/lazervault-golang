package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"lazervaultGo/services"
	"lazervaultGo/tasks"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
)

// HandleUpdateTxFileTask processes the transaction file update task
func HandleUpdateTxFileTask(ctx context.Context, t *asynq.Task, txFileService *services.GenerateTxDataService) error {
	var payload tasks.UpdateTxFileAndIndexPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal UpdateTxFileAndIndex payload: %w", asynq.SkipRetry)
	}

	log.Info().Uint("user_id", payload.UserID).Msg("Processing transaction file update task")

	// Parse the transaction data
	var txHistoryList []map[string]interface{}
	if err := json.Unmarshal([]byte(payload.TxData), &txHistoryList); err != nil {
		return fmt.Errorf("failed to unmarshal transaction data: %w", asynq.SkipRetry)
	}

	// Convert transactions to TransactionRecord format
	var records []services.TransactionRecord
	for _, tx := range txHistoryList {
		record := services.TransactionRecord{
			UserID:      uint64(payload.UserID),
			Type:        tx["type"].(string),
			Amount:      tx["amount"].(float64),
			Currency:    tx["currency"].(string),
			Status:      tx["status"].(string),
			Description: tx["description"].(string),
		}
		records = append(records, record)
	}

	// Format transactions as CSV
	csvData, err := txFileService.FormatTransactionsToCSV(records)
	if err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("Failed to format transactions to CSV")
		return fmt.Errorf("failed to format transactions to CSV: %w", err)
	}

	// Upload to GCS
	if err := txFileService.UploadOrOverwriteTxFile(ctx, fmt.Sprint(payload.UserID), csvData); err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("Failed to upload transaction file to GCS")
		return fmt.Errorf("failed to upload transaction file: %w", err)
	}

	// Trigger indexing
	if err := txFileService.TriggerTransactionFileIndexing(ctx, payload.UserID); err != nil {
		log.Error().Err(err).Uint("user_id", payload.UserID).Msg("Failed to trigger transaction file indexing")
		// Don't return error as indexing failure shouldn't invalidate the update
	}

	log.Info().Uint("user_id", payload.UserID).Msg("Successfully processed transaction file update task")
	return nil
}

package services

import (
	"context"
	"fmt"
	"lazervaultGo/configs"
	"lazervaultGo/models"
	"lazervaultGo/tasks"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type TransferService struct {
	db          *gorm.DB
	config      *configs.Config
	redisWorker tasks.TaskDistributor
}

type TransferRequest struct {
	FromUserID  uint       `json:"from_user_id" validate:"required"`
	ToUserID    uint       `json:"to_user_id" validate:"required"`
	Amount      float64    `json:"amount" validate:"required,gt=0"`
	Reference   string     `json:"reference"`
	Category    string     `json:"category"`
	ScheduledAt *time.Time `json:"scheduled_at"`
}

type TransferResponse struct {
	TransferID  uint      `json:"transfer_id"`
	Status      string    `json:"status"`
	Amount      float64   `json:"amount"`
	Fee         float64   `json:"fee"`
	TotalAmount float64   `json:"total_amount"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewTransferService(db *gorm.DB, config *configs.Config, redisWorker tasks.TaskDistributor) *TransferService {
	return &TransferService{
		db:          db,
		config:      config,
		redisWorker: redisWorker,
	}
}

func (s *TransferService) InitiateTransfer(ctx context.Context, fromUserID uint, req TransferRequest) (*TransferResponse, error) {
	// Start a transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		} else if tx.Error != nil {
			tx.Rollback()
		}
	}()

	// Calculate fee (1.5% of amount)
	fee := req.Amount * 0.015
	totalAmount := req.Amount + fee

	// Create transfer record
	transfer := &models.Transfer{
		FromUserID:  fromUserID,
		ToUserID:    req.ToUserID,
		Amount:      req.Amount,
		Fee:         fee,
		TotalAmount: totalAmount,
		Status:      models.TransferStatusPending,
		Reference:   req.Reference,
		Category:    req.Category,
		ScheduledAt: req.ScheduledAt,
	}

	if err := tx.Create(transfer).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create transfer record: %w", err)
	}

	// Queue the transfer task
	payload := &tasks.PayloadProcessTransfer{
		TransferID: transfer.ID,
	}

	// If scheduled, set the process time
	var opts []asynq.Option
	if req.ScheduledAt != nil {
		opts = append(opts, asynq.ProcessAt(*req.ScheduledAt))
	}

	if err := s.redisWorker.DistributeTaskProcessTransfer(ctx, payload, opts...); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to distribute transfer task: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transfer initiation: %w", err)
	}

	return &TransferResponse{
		TransferID:  transfer.ID,
		Status:      string(transfer.Status),
		Amount:      transfer.Amount,
		Fee:         transfer.Fee,
		TotalAmount: transfer.TotalAmount,
		CreatedAt:   transfer.CreatedAt,
	}, nil
}

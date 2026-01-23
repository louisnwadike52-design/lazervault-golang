package services

import (
	"context"
	"fmt"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type TransactionService struct {
	pb.UnimplementedTransactionServiceServer
	db *gorm.DB
}

func NewTransactionService(db *gorm.DB) *TransactionService {
	return &TransactionService{db: db}
}

// CreateTransaction creates a new transaction record
func (s *TransactionService) CreateTransaction(ctx context.Context, record TransactionRecord) error {
	tx := models.Transaction{
		UserID:                        record.UserID,
		Timestamp:                     record.Timestamp,
		Type:                          record.Type,
		Amount:                        record.Amount,
		Currency:                      record.Currency,
		Status:                        record.Status,
		Description:                   record.Description,
		Reference:                     record.Reference,
		FailureReason:                 record.FailureReason,
		CompletedAt:                   record.CompletedAt,
		ProcessingAt:                  record.ProcessingAt,
		FailedAt:                      record.FailedAt,
		ExternalTransactionID:         record.ExternalTransactionID,
		FromAccountID:                 record.FromAccountID,
		ToAccountID:                   record.ToAccountID,
		DepositSourceBankName:         record.DepositSourceBankName,
		WithdrawalTargetBankName:      record.WithdrawalTargetBankName,
		WithdrawalTargetAccountNumber: record.WithdrawalTargetAccountNumber,
		WithdrawalTargetSortCode:      record.WithdrawalTargetSortCode,
		TransferFee:                   record.TransferFee,
		TransferTotalAmount:           record.TransferTotalAmount,
		TransferCategory:              record.TransferCategory,
		TransferScheduledAt:           record.TransferScheduledAt,
		SenderInfo:                    record.SenderInfo,
		RecipientInfo:                 record.RecipientInfo,
		RecipientID:                   record.RecipientID,
		ExchangeFromCurrency:          record.ExchangeFromCurrency,
		ExchangeToCurrency:            record.ExchangeToCurrency,
		ExchangeAmountFrom:            record.ExchangeAmountFrom,
		ExchangeAmountTo:              record.ExchangeAmountTo,
		ExchangeRate:                  record.ExchangeRate,
		ExchangeFees:                  record.ExchangeFees,
		ExchangeReceiverDetails:       record.ExchangeReceiverDetails,
		RelatedParty:                  record.RelatedParty,
	}

	if err := s.db.WithContext(ctx).Create(&tx).Error; err != nil {
		return fmt.Errorf("failed to create transaction record: %w", err)
	}

	return nil
}

// GetTransactions retrieves transactions based on filters
func (s *TransactionService) GetTransactions(ctx context.Context, req *pb.GetTransactionsRequest) (*pb.GetTransactionsResponse, error) {
	userID, err := strconv.ParseUint(req.UserId, 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user ID format: %v", err)
	}

	// Build query
	query := s.db.WithContext(ctx).Model(&models.Transaction{}).Where("user_id = ?", userID)

	// Apply filters
	if req.Type != nil {
		query = query.Where("type = ?", *req.Type)
	}
	if req.Status != nil {
		query = query.Where("status = ?", *req.Status)
	}
	if req.StartDate != nil {
		startDate, err := time.Parse(time.RFC3339, *req.StartDate)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid start date format: %v", err)
		}
		query = query.Where("timestamp >= ?", startDate)
	}
	if req.EndDate != nil {
		endDate, err := time.Parse(time.RFC3339, *req.EndDate)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid end date format: %v", err)
		}
		query = query.Where("timestamp <= ?", endDate)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count transactions: %v", err)
	}

	// Apply pagination
	limit := 50 // default limit
	if req.Limit != nil && *req.Limit > 0 {
		limit = int(*req.Limit)
	}
	offset := 0
	if req.Offset != nil && *req.Offset > 0 {
		offset = int(*req.Offset)
	}

	// Get transactions
	var transactions []models.Transaction
	if err := query.Order("timestamp DESC").Limit(limit).Offset(offset).Find(&transactions).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch transactions: %v", err)
	}

	// Convert to proto response
	protoTransactions := make([]*pb.Transaction, len(transactions))
	for i, tx := range transactions {
		protoTransactions[i] = &pb.Transaction{
			Id:                            uint64(tx.ID),
			UserId:                        tx.UserID,
			Timestamp:                     tx.Timestamp.Format(time.RFC3339),
			Type:                          tx.Type,
			Amount:                        tx.Amount,
			Currency:                      tx.Currency,
			Status:                        tx.Status,
			Description:                   tx.Description,
			Reference:                     tx.Reference,
			FailureReason:                 tx.FailureReason,
			CompletedAt:                   formatTime(tx.CompletedAt),
			ProcessingAt:                  formatTime(tx.ProcessingAt),
			FailedAt:                      formatTime(tx.FailedAt),
			ExternalTransactionId:         tx.ExternalTransactionID,
			FromAccountId:                 formatUint(tx.FromAccountID),
			ToAccountId:                   formatUint(tx.ToAccountID),
			DepositSourceBankName:         tx.DepositSourceBankName,
			WithdrawalTargetBankName:      tx.WithdrawalTargetBankName,
			WithdrawalTargetAccountNumber: tx.WithdrawalTargetAccountNumber,
			WithdrawalTargetSortCode:      tx.WithdrawalTargetSortCode,
			TransferFee:                   tx.TransferFee,
			TransferTotalAmount:           tx.TransferTotalAmount,
			TransferCategory:              tx.TransferCategory,
			TransferScheduledAt:           tx.TransferScheduledAt,
			SenderInfo:                    tx.SenderInfo,
			RecipientInfo:                 tx.RecipientInfo,
			RecipientId:                   formatUint(tx.RecipientID),
			ExchangeFromCurrency:          tx.ExchangeFromCurrency,
			ExchangeToCurrency:            tx.ExchangeToCurrency,
			ExchangeAmountFrom:            tx.ExchangeAmountFrom,
			ExchangeAmountTo:              tx.ExchangeAmountTo,
			ExchangeRate:                  tx.ExchangeRate,
			ExchangeFees:                  tx.ExchangeFees,
			ExchangeReceiverDetails:       tx.ExchangeReceiverDetails,
			RelatedParty:                  tx.RelatedParty,
			CreatedAt:                     tx.CreatedAt.Format(time.RFC3339),
			UpdatedAt:                     tx.UpdatedAt.Format(time.RFC3339),
		}
	}

	return &pb.GetTransactionsResponse{
		Transactions: protoTransactions,
		Total:        total,
		Limit:        int32(limit),
		Offset:       int32(offset),
	}, nil
}

// Helper functions
func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}

func formatUint(u *uint) *uint32 {
	if u == nil {
		return nil
	}
	v := uint32(*u)
	return &v
}

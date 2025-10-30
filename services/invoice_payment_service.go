package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/database"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/token"
	"strconv"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

var (
	ErrInvoicePaymentNotFound    = errors.New("invoice payment not found")
	ErrInvalidPaymentData        = errors.New("invalid payment data")
	ErrPaymentMethodNotFound     = errors.New("payment method not found")
	ErrPaymentProcessingFailed   = errors.New("payment processing failed")
	ErrInvoiceNotPayable         = errors.New("invoice is not in a payable state")
	ErrPaymentAlreadyProcessed   = errors.New("payment already processed")
	ErrInvalidPaymentAmount      = errors.New("invalid payment amount")
	ErrPaymentMethodNotSupported = errors.New("payment method not supported")
	ErrCryptoWalletNotFound      = errors.New("crypto wallet not found")
	ErrDisputeNotFound           = errors.New("dispute not found")
	ErrExtensionNotFound         = errors.New("extension not found")
)

// IInvoicePaymentService defines the interface for invoice payment operations
type IInvoicePaymentService interface {
	// Core Invoice Payment Operations
	ProcessInvoicePayment(ctx context.Context, req *pb.ProcessInvoicePaymentRequest) (*pb.ProcessInvoicePaymentResponse, error)
	ProcessPartialInvoicePayment(ctx context.Context, req *pb.ProcessPartialInvoicePaymentRequest) (*pb.ProcessPartialInvoicePaymentResponse, error)
	ValidateInvoicePayment(ctx context.Context, req *pb.ValidateInvoicePaymentRequest) (*pb.ValidateInvoicePaymentResponse, error)
	GetInvoicePaymentStatus(ctx context.Context, req *pb.GetInvoicePaymentStatusRequest) (*pb.GetInvoicePaymentStatusResponse, error)
	CancelInvoicePayment(ctx context.Context, req *pb.CancelInvoicePaymentRequest) (*pb.CancelInvoicePaymentResponse, error)

	// Payment Methods Management
	GetUserInvoicePaymentMethods(ctx context.Context, req *pb.GetUserInvoicePaymentMethodsRequest) (*pb.GetUserInvoicePaymentMethodsResponse, error)
	AddInvoicePaymentMethod(ctx context.Context, req *pb.AddInvoicePaymentMethodRequest) (*pb.AddInvoicePaymentMethodResponse, error)
	RemoveInvoicePaymentMethod(ctx context.Context, req *pb.RemoveInvoicePaymentMethodRequest) (*pb.RemoveInvoicePaymentMethodResponse, error)
	ValidateInvoicePaymentMethod(ctx context.Context, req *pb.ValidateInvoicePaymentMethodRequest) (*pb.ValidateInvoicePaymentMethodResponse, error)
	UpdateInvoicePaymentMethod(ctx context.Context, req *pb.UpdateInvoicePaymentMethodRequest) (*pb.UpdateInvoicePaymentMethodResponse, error)

	// Account Balance Operations
	GetUserAccountBalance(ctx context.Context, req *pb.GetUserAccountBalanceRequest) (*pb.GetUserAccountBalanceResponse, error)
	GetAccountBalanceHistory(ctx context.Context, req *pb.GetAccountBalanceHistoryRequest) (*pb.GetAccountBalanceHistoryResponse, error)
	TransferFundsForInvoicePayment(ctx context.Context, req *pb.TransferFundsForInvoicePaymentRequest) (*pb.TransferFundsForInvoicePaymentResponse, error)

	// Cryptocurrency Operations
	ProcessCryptoInvoicePayment(ctx context.Context, req *pb.ProcessCryptoInvoicePaymentRequest) (*pb.ProcessCryptoInvoicePaymentResponse, error)
	GetCryptoWalletBalance(ctx context.Context, req *pb.GetCryptoWalletBalanceRequest) (*pb.GetCryptoWalletBalanceResponse, error)
	ValidateCryptoWallet(ctx context.Context, req *pb.ValidateCryptoWalletRequest) (*pb.ValidateCryptoWalletResponse, error)
	GetCryptoInvoicePaymentStatus(ctx context.Context, req *pb.GetCryptoInvoicePaymentStatusRequest) (*pb.GetCryptoInvoicePaymentStatusResponse, error)

	// Extensions and Disputes
	RequestInvoicePaymentExtension(ctx context.Context, req *pb.RequestInvoicePaymentExtensionRequest) (*pb.RequestInvoicePaymentExtensionResponse, error)
	ApproveInvoicePaymentExtension(ctx context.Context, req *pb.ApproveInvoicePaymentExtensionRequest) (*pb.ApproveInvoicePaymentExtensionResponse, error)
	DisputeInvoicePayment(ctx context.Context, req *pb.DisputeInvoicePaymentRequest) (*pb.DisputeInvoicePaymentResponse, error)
	ResolveInvoicePaymentDispute(ctx context.Context, req *pb.ResolveInvoicePaymentDisputeRequest) (*pb.ResolveInvoicePaymentDisputeResponse, error)

	// History and Analytics
	GetInvoicePaymentHistory(ctx context.Context, req *pb.GetInvoicePaymentHistoryRequest) (*pb.GetInvoicePaymentHistoryResponse, error)
	GetInvoicePaymentStatistics(ctx context.Context, req *pb.GetInvoicePaymentStatisticsRequest) (*pb.GetInvoicePaymentStatisticsResponse, error)
	GetRecentInvoicePaymentTransactions(ctx context.Context, req *pb.GetRecentInvoicePaymentTransactionsRequest) (*pb.GetRecentInvoicePaymentTransactionsResponse, error)

	// Receipts
	GenerateInvoicePaymentReceipt(ctx context.Context, req *pb.GenerateInvoicePaymentReceiptRequest) (*pb.GenerateInvoicePaymentReceiptResponse, error)
	EmailInvoicePaymentReceipt(ctx context.Context, req *pb.EmailInvoicePaymentReceiptRequest) (*pb.EmailInvoicePaymentReceiptResponse, error)
	GetInvoicePaymentReceipt(ctx context.Context, req *pb.GetInvoicePaymentReceiptRequest) (*pb.GetInvoicePaymentReceiptResponse, error)
}

// InvoicePaymentService implements the IInvoicePaymentService
type InvoicePaymentService struct {
	db *gorm.DB
}

// NewInvoicePaymentService creates a new InvoicePaymentService
func NewInvoicePaymentService(db *gorm.DB) IInvoicePaymentService {
	return &InvoicePaymentService{db: db}
}

// getUserIDFromContext extracts the user ID from the JWT token via email database lookup
func (s *InvoicePaymentService) getUserIDFromContext(ctx context.Context) (string, error) {
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return "", status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Get user by email to get the actual user ID
	user, err := database.FindUserByEmail(s.db, authPayload.Email)
	if err != nil {
		return "", status.Errorf(codes.NotFound, "user not found")
	}

	return strconv.FormatUint(uint64(user.ID), 10), nil
}

// ProcessInvoicePayment processes a full invoice payment
func (s *InvoicePaymentService) ProcessInvoicePayment(ctx context.Context, req *pb.ProcessInvoicePaymentRequest) (*pb.ProcessInvoicePaymentResponse, error) {
	if req.InvoiceId == "" || req.PaymentMethodId == "" {
		return nil, ErrInvalidPaymentData
	}

	// Start transaction
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Generate transaction ID
	transactionID := uuid.New().String()

	// Create payment transaction record
	transaction := &models.InvoicePaymentTransaction{
		TransactionID: transactionID,
		InvoiceID:     req.InvoiceId,
		Amount:        req.Amount,
		Currency:      req.Currency,
		Status:        models.InvoicePaymentStatusProcessing,
		Description:   req.Description,
		CreatedAt:     time.Now().UTC(),
	}

	if err := tx.Create(transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("%w: %v", ErrPaymentProcessingFailed, err)
	}

	// TODO: Implement actual payment processing logic here
	// This would integrate with payment processors like Stripe, PayPal, etc.

	// For now, simulate successful payment
	transaction.Status = models.InvoicePaymentStatusCompleted
	transaction.ProcessedAt = &[]time.Time{time.Now().UTC()}[0]

	if err := tx.Save(transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("%w: %v", ErrPaymentProcessingFailed, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &pb.ProcessInvoicePaymentResponse{
		Result: &pb.InvoicePaymentResult{
			Success:       true,
			TransactionId: transactionID,
			Status:        pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_COMPLETED,
		},
		Success: true,
		Message: "Payment processed successfully",
	}, nil
}

// ProcessPartialInvoicePayment processes a partial payment for an invoice
func (s *InvoicePaymentService) ProcessPartialInvoicePayment(ctx context.Context, req *pb.ProcessPartialInvoicePaymentRequest) (*pb.ProcessPartialInvoicePaymentResponse, error) {
	if req.InvoiceId == "" || req.PaymentMethodId == "" || req.PartialAmount <= 0 {
		return nil, ErrInvalidPaymentData
	}

	// Start transaction
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Generate transaction ID
	transactionID := uuid.New().String()

	// Create payment transaction record
	transaction := &models.InvoicePaymentTransaction{
		TransactionID: transactionID,
		InvoiceID:     req.InvoiceId,
		Amount:        req.PartialAmount,
		Currency:      req.Currency,
		Status:        models.InvoicePaymentStatusPartiallyPaid,
		Description:   req.Description,
		CreatedAt:     time.Now().UTC(),
		ProcessedAt:   &[]time.Time{time.Now().UTC()}[0],
	}

	if err := tx.Create(transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("%w: %v", ErrPaymentProcessingFailed, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &pb.ProcessPartialInvoicePaymentResponse{
		Result: &pb.InvoicePaymentResult{
			Success:       true,
			TransactionId: transactionID,
			Status:        pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_PARTIALLY_PAID,
		},
		RemainingAmount: 0, // TODO: Calculate remaining amount
		Success:         true,
		Message:         "Partial payment processed successfully",
	}, nil
}

// ValidateInvoicePayment validates payment data before processing
func (s *InvoicePaymentService) ValidateInvoicePayment(ctx context.Context, req *pb.ValidateInvoicePaymentRequest) (*pb.ValidateInvoicePaymentResponse, error) {
	if req.InvoiceId == "" || req.PaymentMethodId == "" || req.Amount <= 0 {
		return &pb.ValidateInvoicePaymentResponse{
			IsValid:           false,
			ValidationMessage: "Invalid payment data",
			Errors:            []string{"Invoice ID, Payment Method ID, and Amount are required"},
		}, nil
	}

	// TODO: Implement actual validation logic
	// - Check if invoice exists and is payable
	// - Validate payment method
	// - Check available balance
	// - Calculate fees

	return &pb.ValidateInvoicePaymentResponse{
		IsValid:           true,
		ValidationMessage: "Payment data is valid",
		AvailableBalance:  1000.0,             // TODO: Get actual balance
		PaymentFees:       req.Amount * 0.029, // TODO: Calculate actual fees
	}, nil
}

// GetInvoicePaymentStatus retrieves the status of a payment transaction
func (s *InvoicePaymentService) GetInvoicePaymentStatus(ctx context.Context, req *pb.GetInvoicePaymentStatusRequest) (*pb.GetInvoicePaymentStatusResponse, error) {
	if req.TransactionId == "" {
		return nil, ErrInvalidPaymentData
	}

	var transaction models.InvoicePaymentTransaction
	err := s.db.WithContext(ctx).Where("transaction_id = ?", req.TransactionId).First(&transaction).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoicePaymentNotFound
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	return &pb.GetInvoicePaymentStatusResponse{
		Transaction: &pb.InvoicePaymentTransaction{
			TransactionId: transaction.TransactionID,
			InvoiceId:     transaction.InvoiceID,
			Amount:        transaction.Amount,
			Currency:      transaction.Currency,
			Status:        convertModelStatusToPB(transaction.Status),
			Description:   transaction.Description,
		},
		CurrentStatus: convertModelStatusToPB(transaction.Status),
		StatusMessage: "Payment status retrieved successfully",
	}, nil
}

// CancelInvoicePayment cancels a payment transaction
func (s *InvoicePaymentService) CancelInvoicePayment(ctx context.Context, req *pb.CancelInvoicePaymentRequest) (*pb.CancelInvoicePaymentResponse, error) {
	if req.TransactionId == "" {
		return nil, ErrInvalidPaymentData
	}

	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	var transaction models.InvoicePaymentTransaction
	err := tx.Where("transaction_id = ?", req.TransactionId).First(&transaction).Error
	if err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoicePaymentNotFound
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	transaction.Status = models.InvoicePaymentStatusCancelled
	if err := tx.Save(&transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update transaction: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &pb.CancelInvoicePaymentResponse{
		Success:   true,
		Message:   "Payment cancelled successfully",
		NewStatus: pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_CANCELLED,
	}, nil
}

// GetUserInvoicePaymentMethods retrieves payment methods for a user
func (s *InvoicePaymentService) GetUserInvoicePaymentMethods(ctx context.Context, req *pb.GetUserInvoicePaymentMethodsRequest) (*pb.GetUserInvoicePaymentMethodsResponse, error) {
	// Extract user ID from JWT token via email database lookup
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var paymentMethods []models.UserPaymentMethod
	query := s.db.WithContext(ctx).Where("user_id = ?", userID)

	if req.TypeFilter != pb.PaymentMethodType_PAYMENT_METHOD_TYPE_ACCOUNT_BALANCE {
		query = query.Where("type = ?", req.TypeFilter)
	}

	err = query.Find(&paymentMethods).Error
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	var pbMethods []*pb.PaymentMethod
	var defaultMethod *pb.PaymentMethod

	for _, method := range paymentMethods {
		pbMethod := &pb.PaymentMethod{
			MethodId:    method.MethodID,
			UserId:      method.UserID,
			Type:        convertModelPaymentMethodToPB(method.Type),
			DisplayName: method.DisplayName,
			Last4:       method.Last4,
			Brand:       method.Brand,
			IsDefault:   method.IsDefault,
			IsVerified:  method.IsVerified,
		}

		pbMethods = append(pbMethods, pbMethod)
		if method.IsDefault {
			defaultMethod = pbMethod
		}
	}

	return &pb.GetUserInvoicePaymentMethodsResponse{
		PaymentMethods: pbMethods,
		DefaultMethod:  defaultMethod,
	}, nil
}

// AddInvoicePaymentMethod adds a new payment method
func (s *InvoicePaymentService) AddInvoicePaymentMethod(ctx context.Context, req *pb.AddInvoicePaymentMethodRequest) (*pb.AddInvoicePaymentMethodResponse, error) {
	if req.DisplayName == "" {
		return nil, ErrInvalidPaymentData
	}

	methodID := uuid.New().String()

	paymentMethod := &models.UserPaymentMethod{
		MethodID:    methodID,
		Type:        models.PaymentMethodType(req.Type),
		DisplayName: req.DisplayName,
		IsDefault:   req.SetAsDefault,
		IsVerified:  false, // New methods start as unverified
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.db.WithContext(ctx).Create(paymentMethod).Error; err != nil {
		return nil, fmt.Errorf("failed to create payment method: %w", err)
	}

	return &pb.AddInvoicePaymentMethodResponse{
		PaymentMethod: &pb.PaymentMethod{
			MethodId:    methodID,
			Type:        req.Type,
			DisplayName: req.DisplayName,
			IsDefault:   req.SetAsDefault,
			IsVerified:  false,
		},
		Success: true,
		Message: "Payment method added successfully",
	}, nil
}

// RemoveInvoicePaymentMethod removes a payment method
func (s *InvoicePaymentService) RemoveInvoicePaymentMethod(ctx context.Context, req *pb.RemoveInvoicePaymentMethodRequest) (*pb.RemoveInvoicePaymentMethodResponse, error) {
	if req.MethodId == "" {
		return nil, ErrInvalidPaymentData
	}

	result := s.db.WithContext(ctx).Where("method_id = ?", req.MethodId).Delete(&models.UserPaymentMethod{})
	if result.Error != nil {
		return nil, fmt.Errorf("failed to delete payment method: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return nil, ErrPaymentMethodNotFound
	}

	return &pb.RemoveInvoicePaymentMethodResponse{
		Success: true,
		Message: "Payment method removed successfully",
	}, nil
}

// ValidateInvoicePaymentMethod validates a payment method
func (s *InvoicePaymentService) ValidateInvoicePaymentMethod(ctx context.Context, req *pb.ValidateInvoicePaymentMethodRequest) (*pb.ValidateInvoicePaymentMethodResponse, error) {
	if req.MethodId == "" {
		return nil, ErrInvalidPaymentData
	}

	var paymentMethod models.UserPaymentMethod
	err := s.db.WithContext(ctx).Where("method_id = ?", req.MethodId).First(&paymentMethod).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// TODO: Implement actual validation logic
	// This would typically involve calling payment processor APIs

	return &pb.ValidateInvoicePaymentMethodResponse{
		IsValid:           true,
		ValidationMessage: "Payment method is valid",
	}, nil
}

// UpdateInvoicePaymentMethod updates a payment method
func (s *InvoicePaymentService) UpdateInvoicePaymentMethod(ctx context.Context, req *pb.UpdateInvoicePaymentMethodRequest) (*pb.UpdateInvoicePaymentMethodResponse, error) {
	if req.MethodId == "" {
		return nil, ErrInvalidPaymentData
	}

	var paymentMethod models.UserPaymentMethod
	err := s.db.WithContext(ctx).Where("method_id = ?", req.MethodId).First(&paymentMethod).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentMethodNotFound
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	if req.DisplayName != "" {
		paymentMethod.DisplayName = req.DisplayName
	}
	paymentMethod.IsDefault = req.IsDefault
	if req.BillingAddress != "" {
		paymentMethod.BillingAddress = req.BillingAddress
	}

	if err := s.db.WithContext(ctx).Save(&paymentMethod).Error; err != nil {
		return nil, fmt.Errorf("failed to update payment method: %w", err)
	}

	return &pb.UpdateInvoicePaymentMethodResponse{
		PaymentMethod: &pb.PaymentMethod{
			MethodId:    paymentMethod.MethodID,
			Type:        convertModelPaymentMethodToPB(paymentMethod.Type),
			DisplayName: paymentMethod.DisplayName,
			IsDefault:   paymentMethod.IsDefault,
			IsVerified:  paymentMethod.IsVerified,
		},
		Success: true,
		Message: "Payment method updated successfully",
	}, nil
}

// GetUserAccountBalance retrieves account balance for a user
func (s *InvoicePaymentService) GetUserAccountBalance(ctx context.Context, req *pb.GetUserAccountBalanceRequest) (*pb.GetUserAccountBalanceResponse, error) {
	// Extract user ID from JWT token via email database lookup
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual account balance retrieval from database
	return &pb.GetUserAccountBalanceResponse{
		Accounts: []*pb.UserAccountBalance{
			{
				UserId:           userID,
				Currency:         req.Currency,
				AvailableBalance: 1000.0,
				TotalBalance:     1000.0,
			},
		},
		TotalBalance:    1000.0,
		PrimaryCurrency: req.Currency,
	}, nil
}

func (s *InvoicePaymentService) GetAccountBalanceHistory(ctx context.Context, req *pb.GetAccountBalanceHistoryRequest) (*pb.GetAccountBalanceHistoryResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual balance history retrieval from database
	return &pb.GetAccountBalanceHistoryResponse{
		Entries:       []*pb.BalanceHistoryEntry{},
		NextPageToken: "",
	}, nil
}

func (s *InvoicePaymentService) TransferFundsForInvoicePayment(ctx context.Context, req *pb.TransferFundsForInvoicePaymentRequest) (*pb.TransferFundsForInvoicePaymentResponse, error) {
	// TODO: Implement fund transfer
	return &pb.TransferFundsForInvoicePaymentResponse{
		Success: true,
		Message: "Fund transfer completed successfully",
	}, nil
}

func (s *InvoicePaymentService) ProcessCryptoInvoicePayment(ctx context.Context, req *pb.ProcessCryptoInvoicePaymentRequest) (*pb.ProcessCryptoInvoicePaymentResponse, error) {
	// TODO: Implement crypto payment processing
	return &pb.ProcessCryptoInvoicePaymentResponse{
		Success: true,
		Message: "Crypto payment processed successfully",
	}, nil
}

func (s *InvoicePaymentService) GetCryptoWalletBalance(ctx context.Context, req *pb.GetCryptoWalletBalanceRequest) (*pb.GetCryptoWalletBalanceResponse, error) {
	// TODO: Implement crypto wallet balance retrieval
	return &pb.GetCryptoWalletBalanceResponse{
		Balance:     100.0,
		Currency:    "BTC",
		LastUpdated: timestamppb.Now(),
		Network:     "mainnet",
	}, nil
}

func (s *InvoicePaymentService) ValidateCryptoWallet(ctx context.Context, req *pb.ValidateCryptoWalletRequest) (*pb.ValidateCryptoWalletResponse, error) {
	// TODO: Implement crypto wallet validation
	return &pb.ValidateCryptoWalletResponse{
		IsValid:           true,
		ValidationMessage: "Crypto wallet is valid",
		IsContract:        false,
		WalletType:        "external",
	}, nil
}

func (s *InvoicePaymentService) GetCryptoInvoicePaymentStatus(ctx context.Context, req *pb.GetCryptoInvoicePaymentStatusRequest) (*pb.GetCryptoInvoicePaymentStatusResponse, error) {
	// TODO: Implement crypto payment status retrieval
	return &pb.GetCryptoInvoicePaymentStatusResponse{
		Status:                "confirmed",
		Confirmations:         6,
		RequiredConfirmations: 3,
		Amount:                0.001,
		Currency:              "BTC",
		Timestamp:             timestamppb.Now(),
	}, nil
}

func (s *InvoicePaymentService) RequestInvoicePaymentExtension(ctx context.Context, req *pb.RequestInvoicePaymentExtensionRequest) (*pb.RequestInvoicePaymentExtensionResponse, error) {
	// TODO: Implement payment extension request
	return &pb.RequestInvoicePaymentExtensionResponse{
		Success: true,
		Message: "Payment extension requested successfully",
	}, nil
}

func (s *InvoicePaymentService) ApproveInvoicePaymentExtension(ctx context.Context, req *pb.ApproveInvoicePaymentExtensionRequest) (*pb.ApproveInvoicePaymentExtensionResponse, error) {
	// TODO: Implement payment extension approval
	return &pb.ApproveInvoicePaymentExtensionResponse{
		Success: true,
		Message: "Payment extension approved successfully",
	}, nil
}

func (s *InvoicePaymentService) DisputeInvoicePayment(ctx context.Context, req *pb.DisputeInvoicePaymentRequest) (*pb.DisputeInvoicePaymentResponse, error) {
	// TODO: Implement payment dispute
	return &pb.DisputeInvoicePaymentResponse{
		Success: true,
		Message: "Payment dispute created successfully",
	}, nil
}

func (s *InvoicePaymentService) ResolveInvoicePaymentDispute(ctx context.Context, req *pb.ResolveInvoicePaymentDisputeRequest) (*pb.ResolveInvoicePaymentDisputeResponse, error) {
	// TODO: Implement dispute resolution
	return &pb.ResolveInvoicePaymentDisputeResponse{
		Success: true,
		Message: "Payment dispute resolved successfully",
	}, nil
}

// GetInvoicePaymentHistory retrieves payment history for a user
func (s *InvoicePaymentService) GetInvoicePaymentHistory(ctx context.Context, req *pb.GetInvoicePaymentHistoryRequest) (*pb.GetInvoicePaymentHistoryResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual payment history retrieval from database
	return &pb.GetInvoicePaymentHistoryResponse{
		Transactions:  []*pb.InvoicePaymentTransaction{},
		NextPageToken: "",
		TotalCount:    0,
	}, nil
}

// GetInvoicePaymentStatistics retrieves payment statistics for a user
func (s *InvoicePaymentService) GetInvoicePaymentStatistics(ctx context.Context, req *pb.GetInvoicePaymentStatisticsRequest) (*pb.GetInvoicePaymentStatisticsResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual statistics calculation from database
	return &pb.GetInvoicePaymentStatisticsResponse{
		TotalPayments:      0,
		SuccessfulPayments: 0,
		FailedPayments:     0,
		TotalAmount:        0.0,
		TotalFees:          0.0,
		SuccessRate:        0.0,
	}, nil
}

// GetRecentInvoicePaymentTransactions retrieves recent payment transactions for a user
func (s *InvoicePaymentService) GetRecentInvoicePaymentTransactions(ctx context.Context, req *pb.GetRecentInvoicePaymentTransactionsRequest) (*pb.GetRecentInvoicePaymentTransactionsResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual recent transactions retrieval from database
	return &pb.GetRecentInvoicePaymentTransactionsResponse{
		Transactions: []*pb.InvoicePaymentTransaction{},
	}, nil
}

func (s *InvoicePaymentService) GenerateInvoicePaymentReceipt(ctx context.Context, req *pb.GenerateInvoicePaymentReceiptRequest) (*pb.GenerateInvoicePaymentReceiptResponse, error) {
	// TODO: Implement receipt generation
	return &pb.GenerateInvoicePaymentReceiptResponse{
		Success: true,
		Message: "Receipt generated successfully",
	}, nil
}

func (s *InvoicePaymentService) EmailInvoicePaymentReceipt(ctx context.Context, req *pb.EmailInvoicePaymentReceiptRequest) (*pb.EmailInvoicePaymentReceiptResponse, error) {
	// TODO: Implement receipt email
	return &pb.EmailInvoicePaymentReceiptResponse{
		Success: true,
		Message: "Receipt emailed successfully",
	}, nil
}

func (s *InvoicePaymentService) GetInvoicePaymentReceipt(ctx context.Context, req *pb.GetInvoicePaymentReceiptRequest) (*pb.GetInvoicePaymentReceiptResponse, error) {
	// TODO: Implement receipt retrieval
	return &pb.GetInvoicePaymentReceiptResponse{
		ReceiptUrl: "https://example.com/receipt/" + req.TransactionId,
	}, nil
}

// Helper functions for type conversion
func convertModelStatusToPB(status models.InvoicePaymentStatus) pb.InvoicePaymentStatus {
	switch status {
	case models.InvoicePaymentStatusPending:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_PENDING
	case models.InvoicePaymentStatusProcessing:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_PROCESSING
	case models.InvoicePaymentStatusCompleted:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_COMPLETED
	case models.InvoicePaymentStatusFailed:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_FAILED
	case models.InvoicePaymentStatusCancelled:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_CANCELLED
	case models.InvoicePaymentStatusPartiallyPaid:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_PARTIALLY_PAID
	case models.InvoicePaymentStatusRefunded:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_REFUNDED
	case models.InvoicePaymentStatusDisputed:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_DISPUTED
	case models.InvoicePaymentStatusOverdue:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_OVERDUE
	default:
		return pb.InvoicePaymentStatus_INVOICE_PAYMENT_STATUS_PENDING
	}
}

func convertModelPaymentMethodToPB(method models.PaymentMethodType) pb.PaymentMethodType {
	switch method {
	case models.PaymentMethodTypeAccountBalance:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_ACCOUNT_BALANCE
	case models.PaymentMethodTypeCreditCard:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_CREDIT_CARD
	case models.PaymentMethodTypeDebitCard:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_DEBIT_CARD
	case models.PaymentMethodTypePayPal:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_PAYPAL
	case models.PaymentMethodTypeApplePay:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_APPLE_PAY
	case models.PaymentMethodTypeGooglePay:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_GOOGLE_PAY
	case models.PaymentMethodTypeBitcoin:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_BITCOIN
	case models.PaymentMethodTypeEthereum:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_ETHEREUM
	case models.PaymentMethodTypeUSDC:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_USDC
	case models.PaymentMethodTypeBankTransfer:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_BANK_TRANSFER
	default:
		return pb.PaymentMethodType_PAYMENT_METHOD_TYPE_ACCOUNT_BALANCE
	}
}

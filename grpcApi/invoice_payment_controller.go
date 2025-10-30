package grpcApi

import (
	"context"
	"lazervaultGo/database"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// InvoicePaymentController handles gRPC requests for invoice payment operations
type InvoicePaymentController struct {
	pb.UnimplementedInvoicePaymentServiceServer
	invoicePaymentService services.IInvoicePaymentService
	userService           services.IUserService
	db                    *gorm.DB
}

// NewInvoicePaymentController creates a new InvoicePaymentController
func NewInvoicePaymentController(invoicePaymentService services.IInvoicePaymentService, userService services.IUserService, db *gorm.DB) *InvoicePaymentController {
	return &InvoicePaymentController{
		invoicePaymentService: invoicePaymentService,
		userService:           userService,
		db:                    db,
	}
}

// getUserFromAuthPayload extracts the user ID from the auth payload
func (c *InvoicePaymentController) getUserFromAuthPayload(ctx context.Context) (string, error) {
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return "", status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Get user by email to get the actual user ID
	user, err := database.FindUserByEmail(c.db, authPayload.Email)
	if err != nil {
		return "", status.Errorf(codes.NotFound, "user not found")
	}

	return strconv.FormatUint(uint64(user.ID), 10), nil
}

// Core Invoice Payment Operations

// ProcessInvoicePayment processes a full invoice payment
func (c *InvoicePaymentController) ProcessInvoicePayment(ctx context.Context, req *pb.ProcessInvoicePaymentRequest) (*pb.ProcessInvoicePaymentResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.ProcessInvoicePayment(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to process invoice payment: %v", err)
	}

	return response, nil
}

// ProcessPartialInvoicePayment processes a partial payment for an invoice
func (c *InvoicePaymentController) ProcessPartialInvoicePayment(ctx context.Context, req *pb.ProcessPartialInvoicePaymentRequest) (*pb.ProcessPartialInvoicePaymentResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.ProcessPartialInvoicePayment(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to process partial invoice payment: %v", err)
	}

	return response, nil
}

// ValidateInvoicePayment validates payment data before processing
func (c *InvoicePaymentController) ValidateInvoicePayment(ctx context.Context, req *pb.ValidateInvoicePaymentRequest) (*pb.ValidateInvoicePaymentResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.ValidateInvoicePayment(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to validate invoice payment: %v", err)
	}

	return response, nil
}

// GetInvoicePaymentStatus retrieves the status of a payment transaction
func (c *InvoicePaymentController) GetInvoicePaymentStatus(ctx context.Context, req *pb.GetInvoicePaymentStatusRequest) (*pb.GetInvoicePaymentStatusResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.GetInvoicePaymentStatus(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get invoice payment status: %v", err)
	}

	return response, nil
}

// CancelInvoicePayment cancels a payment transaction
func (c *InvoicePaymentController) CancelInvoicePayment(ctx context.Context, req *pb.CancelInvoicePaymentRequest) (*pb.CancelInvoicePaymentResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.CancelInvoicePayment(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to cancel invoice payment: %v", err)
	}

	return response, nil
}

// Invoice Payment Methods Management

// GetUserInvoicePaymentMethods retrieves payment methods for a user
func (c *InvoicePaymentController) GetUserInvoicePaymentMethods(ctx context.Context, req *pb.GetUserInvoicePaymentMethodsRequest) (*pb.GetUserInvoicePaymentMethodsResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.invoicePaymentService.GetUserInvoicePaymentMethods(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user invoice payment methods: %v", err)
	}

	return response, nil
}

// AddInvoicePaymentMethod adds a new payment method
func (c *InvoicePaymentController) AddInvoicePaymentMethod(ctx context.Context, req *pb.AddInvoicePaymentMethodRequest) (*pb.AddInvoicePaymentMethodResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.AddInvoicePaymentMethod(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to add invoice payment method: %v", err)
	}

	return response, nil
}

// RemoveInvoicePaymentMethod removes a payment method
func (c *InvoicePaymentController) RemoveInvoicePaymentMethod(ctx context.Context, req *pb.RemoveInvoicePaymentMethodRequest) (*pb.RemoveInvoicePaymentMethodResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.RemoveInvoicePaymentMethod(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove invoice payment method: %v", err)
	}

	return response, nil
}

// ValidateInvoicePaymentMethod validates a payment method
func (c *InvoicePaymentController) ValidateInvoicePaymentMethod(ctx context.Context, req *pb.ValidateInvoicePaymentMethodRequest) (*pb.ValidateInvoicePaymentMethodResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.ValidateInvoicePaymentMethod(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to validate invoice payment method: %v", err)
	}

	return response, nil
}

// UpdateInvoicePaymentMethod updates a payment method
func (c *InvoicePaymentController) UpdateInvoicePaymentMethod(ctx context.Context, req *pb.UpdateInvoicePaymentMethodRequest) (*pb.UpdateInvoicePaymentMethodResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.UpdateInvoicePaymentMethod(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update invoice payment method: %v", err)
	}

	return response, nil
}

// Account Balance for Invoice Payments

// GetUserAccountBalance retrieves account balance for a user
func (c *InvoicePaymentController) GetUserAccountBalance(ctx context.Context, req *pb.GetUserAccountBalanceRequest) (*pb.GetUserAccountBalanceResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.invoicePaymentService.GetUserAccountBalance(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user account balance: %v", err)
	}

	return response, nil
}

// GetAccountBalanceHistory retrieves account balance history
func (c *InvoicePaymentController) GetAccountBalanceHistory(ctx context.Context, req *pb.GetAccountBalanceHistoryRequest) (*pb.GetAccountBalanceHistoryResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.invoicePaymentService.GetAccountBalanceHistory(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get account balance history: %v", err)
	}

	return response, nil
}

// TransferFundsForInvoicePayment transfers funds for invoice payment
func (c *InvoicePaymentController) TransferFundsForInvoicePayment(ctx context.Context, req *pb.TransferFundsForInvoicePaymentRequest) (*pb.TransferFundsForInvoicePaymentResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.TransferFundsForInvoicePayment(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to transfer funds for invoice payment: %v", err)
	}

	return response, nil
}

// Cryptocurrency Invoice Payments

// ProcessCryptoInvoicePayment processes a cryptocurrency invoice payment
func (c *InvoicePaymentController) ProcessCryptoInvoicePayment(ctx context.Context, req *pb.ProcessCryptoInvoicePaymentRequest) (*pb.ProcessCryptoInvoicePaymentResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.ProcessCryptoInvoicePayment(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to process crypto invoice payment: %v", err)
	}

	return response, nil
}

// GetCryptoWalletBalance retrieves crypto wallet balance
func (c *InvoicePaymentController) GetCryptoWalletBalance(ctx context.Context, req *pb.GetCryptoWalletBalanceRequest) (*pb.GetCryptoWalletBalanceResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.GetCryptoWalletBalance(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get crypto wallet balance: %v", err)
	}

	return response, nil
}

// ValidateCryptoWallet validates a crypto wallet
func (c *InvoicePaymentController) ValidateCryptoWallet(ctx context.Context, req *pb.ValidateCryptoWalletRequest) (*pb.ValidateCryptoWalletResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.ValidateCryptoWallet(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to validate crypto wallet: %v", err)
	}

	return response, nil
}

// GetCryptoInvoicePaymentStatus retrieves crypto invoice payment status
func (c *InvoicePaymentController) GetCryptoInvoicePaymentStatus(ctx context.Context, req *pb.GetCryptoInvoicePaymentStatusRequest) (*pb.GetCryptoInvoicePaymentStatusResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.GetCryptoInvoicePaymentStatus(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get crypto invoice payment status: %v", err)
	}

	return response, nil
}

// Invoice Payment Extensions and Disputes

// RequestInvoicePaymentExtension requests a payment extension
func (c *InvoicePaymentController) RequestInvoicePaymentExtension(ctx context.Context, req *pb.RequestInvoicePaymentExtensionRequest) (*pb.RequestInvoicePaymentExtensionResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.RequestInvoicePaymentExtension(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to request invoice payment extension: %v", err)
	}

	return response, nil
}

// ApproveInvoicePaymentExtension approves a payment extension
func (c *InvoicePaymentController) ApproveInvoicePaymentExtension(ctx context.Context, req *pb.ApproveInvoicePaymentExtensionRequest) (*pb.ApproveInvoicePaymentExtensionResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.ApproveInvoicePaymentExtension(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to approve invoice payment extension: %v", err)
	}

	return response, nil
}

// DisputeInvoicePayment creates a payment dispute
func (c *InvoicePaymentController) DisputeInvoicePayment(ctx context.Context, req *pb.DisputeInvoicePaymentRequest) (*pb.DisputeInvoicePaymentResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.DisputeInvoicePayment(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to dispute invoice payment: %v", err)
	}

	return response, nil
}

// ResolveInvoicePaymentDispute resolves a payment dispute
func (c *InvoicePaymentController) ResolveInvoicePaymentDispute(ctx context.Context, req *pb.ResolveInvoicePaymentDisputeRequest) (*pb.ResolveInvoicePaymentDisputeResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.ResolveInvoicePaymentDispute(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to resolve invoice payment dispute: %v", err)
	}

	return response, nil
}

// Invoice Payment History and Analytics

// GetInvoicePaymentHistory retrieves payment history for a user
func (c *InvoicePaymentController) GetInvoicePaymentHistory(ctx context.Context, req *pb.GetInvoicePaymentHistoryRequest) (*pb.GetInvoicePaymentHistoryResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.invoicePaymentService.GetInvoicePaymentHistory(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get invoice payment history: %v", err)
	}

	return response, nil
}

// GetInvoicePaymentStatistics retrieves payment statistics for a user
func (c *InvoicePaymentController) GetInvoicePaymentStatistics(ctx context.Context, req *pb.GetInvoicePaymentStatisticsRequest) (*pb.GetInvoicePaymentStatisticsResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.invoicePaymentService.GetInvoicePaymentStatistics(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get invoice payment statistics: %v", err)
	}

	return response, nil
}

// GetRecentInvoicePaymentTransactions retrieves recent payment transactions for a user
func (c *InvoicePaymentController) GetRecentInvoicePaymentTransactions(ctx context.Context, req *pb.GetRecentInvoicePaymentTransactionsRequest) (*pb.GetRecentInvoicePaymentTransactionsResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.invoicePaymentService.GetRecentInvoicePaymentTransactions(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get recent invoice payment transactions: %v", err)
	}

	return response, nil
}

// Transaction Receipts

// GenerateInvoicePaymentReceipt generates a payment receipt
func (c *InvoicePaymentController) GenerateInvoicePaymentReceipt(ctx context.Context, req *pb.GenerateInvoicePaymentReceiptRequest) (*pb.GenerateInvoicePaymentReceiptResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.GenerateInvoicePaymentReceipt(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate invoice payment receipt: %v", err)
	}

	return response, nil
}

// EmailInvoicePaymentReceipt emails a payment receipt
func (c *InvoicePaymentController) EmailInvoicePaymentReceipt(ctx context.Context, req *pb.EmailInvoicePaymentReceiptRequest) (*pb.EmailInvoicePaymentReceiptResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.EmailInvoicePaymentReceipt(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to email invoice payment receipt: %v", err)
	}

	return response, nil
}

// GetInvoicePaymentReceipt retrieves a payment receipt
func (c *InvoicePaymentController) GetInvoicePaymentReceipt(ctx context.Context, req *pb.GetInvoicePaymentReceiptRequest) (*pb.GetInvoicePaymentReceiptResponse, error) {
	// Extract and validate user from context
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Call service
	response, err := c.invoicePaymentService.GetInvoicePaymentReceipt(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get invoice payment receipt: %v", err)
	}

	return response, nil
}

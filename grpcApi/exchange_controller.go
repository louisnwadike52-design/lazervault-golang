package grpcApi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ExchangeController handles gRPC requests for currency exchange operations
type ExchangeController struct {
	pb.UnimplementedExchangeServiceServer // Embed for forward compatibility
	exchangeService                       services.IExchangeService
	userService                           services.IUserService // Needed to get UserID from email
}

// NewExchangeController creates a new ExchangeController
func NewExchangeController(exchangeService services.IExchangeService, userService services.IUserService) *ExchangeController {
	return &ExchangeController{
		exchangeService: exchangeService,
		userService:     userService,
	}
}

// Helper to convert model ReceiverDetails to proto ReceiverDetails
func convertReceiverDetailsToProto(detailsJSON []byte) (*pb.ReceiverDetails, error) {
	var modelDetails models.ReceiverDetails
	if err := json.Unmarshal(detailsJSON, &modelDetails); err != nil {
		return nil, fmt.Errorf("failed to unmarshal receiver details: %w", err)
	}
	return &pb.ReceiverDetails{
		FullName:      modelDetails.FullName,
		AccountNumber: modelDetails.AccountNumber,
		BankName:      modelDetails.BankName,
		SwiftBicCode:  modelDetails.SwiftBicCode,
	}, nil
}

// Helper to convert model ExchangeTransaction to proto ExchangeTransaction
func convertExchangeTransactionToProto(tx *models.ExchangeTransaction) (*pb.ExchangeTransaction, error) {
	if tx == nil {
		return nil, nil
	}

	receiverProto, err := convertReceiverDetailsToProto(tx.ReceiverDetails)
	if err != nil {
		// Log the error but potentially continue, returning partial data or nil receiver
		fmt.Printf("Error converting receiver details for tx %s: %v\n", tx.ID, err)
		// return nil, fmt.Errorf("failed to convert receiver details: %w", err)
	}

	statusEnum, _ := pb.ExchangeStatus_value[tx.Status]

	return &pb.ExchangeTransaction{
		TransactionId:   tx.ID,
		UserId:          tx.UserID,
		FromCurrency:    tx.FromCurrency,
		ToCurrency:      tx.ToCurrency,
		AmountFrom:      tx.AmountFrom,
		AmountTo:        tx.AmountTo,
		ExchangeRate:    tx.ExchangeRate,
		Fees:            tx.Fees,
		ReceiverDetails: receiverProto, // Assign potentially nil if conversion failed
		Status:          pb.ExchangeStatus(statusEnum),
		CreatedAt:       timestamppb.New(tx.CreatedAt),
		UpdatedAt:       timestamppb.New(tx.UpdatedAt),
	}, nil
}

// GetExchangeRate handles the gRPC request
func (controller *ExchangeController) GetExchangeRate(ctx context.Context, req *pb.GetExchangeRateRequest) (*pb.GetExchangeRateResponse, error) {
	// No authentication needed for this specific endpoint usually, but check requirements.

	if req.GetFromCurrency() == "" || req.GetToCurrency() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "from_currency and to_currency are required")
	}

	rate, err := controller.exchangeService.GetExchangeRate(ctx, req.GetFromCurrency(), req.GetToCurrency())
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidCurrency):
			return nil, status.Errorf(codes.InvalidArgument, "invalid currency code: %v", err)
		case errors.Is(err, services.ErrRateNotFound):
			return nil, status.Errorf(codes.NotFound, "exchange rate not found: %v", err)
		default:
			return nil, status.Errorf(codes.Internal, "failed to get exchange rate: %v", err)
		}
	}

	resp := &pb.GetExchangeRateResponse{
		FromCurrency: req.GetFromCurrency(),
		ToCurrency:   req.GetToCurrency(),
		Rate:         rate,
		Timestamp:    timestamppb.Now(), // Use current time as rate fetch time
	}

	return resp, nil
}

// InitiateInternationalTransfer handles the gRPC request
func (controller *ExchangeController) InitiateInternationalTransfer(ctx context.Context, req *pb.InitiateInternationalTransferRequest) (*pb.InitiateInternationalTransferResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// --- Input Validation ---
	if req.GetFromCurrency() == "" || req.GetToCurrency() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "from_currency and to_currency are required")
	}
	if req.GetAmountFrom() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount_from must be positive")
	}
	if req.ReceiverDetails == nil {
		return nil, status.Errorf(codes.InvalidArgument, "receiver_details are required")
	}
	if req.ReceiverDetails.GetFullName() == "" || req.ReceiverDetails.GetAccountNumber() == "" || req.ReceiverDetails.GetBankName() == "" || req.ReceiverDetails.GetSwiftBicCode() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "receiver full_name, account_number, bank_name, and swift_bic_code are required")
	}

	// --- Get User from Email ---
	user, err := controller.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) { // Use the defined error
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	userID := user.ID // Assuming user model has ID field (uint)

	// --- Prepare Service Request ---
	serviceReq := &services.InitiateTransferServiceRequest{
		UserID:       fmt.Sprint(userID),
		FromCurrency: req.GetFromCurrency(),
		ToCurrency:   req.GetToCurrency(),
		AmountFrom:   req.GetAmountFrom(),
		ReceiverDetails: models.ReceiverDetails{ // Convert proto to model
			FullName:      req.ReceiverDetails.GetFullName(),
			AccountNumber: req.ReceiverDetails.GetAccountNumber(),
			BankName:      req.ReceiverDetails.GetBankName(),
			SwiftBicCode:  req.ReceiverDetails.GetSwiftBicCode(),
		},
	}

	// --- Call Service ---
	createdTx, err := controller.exchangeService.InitiateInternationalTransfer(ctx, serviceReq)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidUserID):
			return nil, status.Errorf(codes.Internal, "internal user ID error: %v", err) // Should not happen if fetched above
		case errors.Is(err, services.ErrInvalidCurrency):
			return nil, status.Errorf(codes.InvalidArgument, "invalid currency: %v", err)
		case errors.Is(err, services.ErrInvalidAmount):
			return nil, status.Errorf(codes.InvalidArgument, "invalid amount: %v", err)
		case errors.Is(err, services.ErrInvalidReceiver):
			return nil, status.Errorf(codes.InvalidArgument, "invalid receiver details: %v", err)
		case errors.Is(err, services.ErrRateNotFound):
			return nil, status.Errorf(codes.FailedPrecondition, "exchange rate not available: %v", err)
		case errors.Is(err, services.ErrInsufficientFunds):
			return nil, status.Errorf(codes.FailedPrecondition, "insufficient funds: %v", err)
		// Add other specific errors like debit failure
		default:
			return nil, status.Errorf(codes.Internal, "failed to initiate transfer: %v", err)
		}
	}

	// --- Convert Response ---
	protoTx, err := convertExchangeTransactionToProto(createdTx)
	if err != nil {
		fmt.Printf("Error converting successful transaction to proto: %v\n", err)
	}

	resp := &pb.InitiateInternationalTransferResponse{
		Transaction: protoTx,
	}

	return resp, nil
}

// GetRecentExchanges handles the gRPC request
func (controller *ExchangeController) GetRecentExchanges(ctx context.Context, req *pb.GetRecentExchangesRequest) (*pb.GetRecentExchangesResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// --- Get User ID ---
	user, err := controller.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) { // Use the defined error
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	userID := user.ID // Assuming user model has ID field (uint)

	serviceReq := &services.GetRecentExchangesServiceRequest{
		UserID:    fmt.Sprint(userID),
		PageSize:  int(req.GetPageSize()),
		PageToken: req.GetPageToken(),
	}

	transactions, nextToken, err := controller.exchangeService.GetRecentExchanges(ctx, serviceReq)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidUserID):
			return nil, status.Errorf(codes.Internal, "internal user ID error: %v", err)
		case errors.Is(err, sql.ErrNoRows):
			return &pb.GetRecentExchangesResponse{Transactions: []*pb.ExchangeTransaction{}, NextPageToken: ""}, nil
		// Handle pagination token error if service returns it
		default:
			return nil, status.Errorf(codes.Internal, "failed to get recent exchanges: %v", err)
		}
	}

	pbTransactions := make([]*pb.ExchangeTransaction, 0, len(transactions))
	for _, tx := range transactions {
		protoTx, err := convertExchangeTransactionToProto(&tx)
		if err != nil {
			fmt.Printf("Error converting transaction %s to proto: %v\n", tx.ID, err)
			continue
		}
		pbTransactions = append(pbTransactions, protoTx)
	}

	resp := &pb.GetRecentExchangesResponse{
		Transactions:  pbTransactions,
		NextPageToken: nextToken,
	}

	return resp, nil
}

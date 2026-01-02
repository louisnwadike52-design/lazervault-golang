package grpcApi

import (
	"context"
	"errors"
	"math"

	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AccountCardController handles gRPC requests for cards
type AccountCardController struct {
	pb.UnimplementedAccountCardServiceServer
	cardService *services.CardService
	userService services.IUserService
}

// NewAccountCardController creates a new controller
func NewAccountCardController(cardService *services.CardService, userService services.IUserService) *AccountCardController {
	return &AccountCardController{
		cardService: cardService,
		userService: userService,
	}
}

// convertToProto converts model to protobuf message
func convertToProto(card *models.AccountCard, includeSecureData bool) *pb.AccountCard {
	if card == nil {
		return nil
	}

	pbCard := &pb.AccountCard{
		Id:             uint64(card.ID),
		Uuid:           card.UUID,
		AccountId:      uint64(card.AccountID),
		UserId:         uint64(card.UserID),
		CardHolderName: card.CardHolderName,
		Brand:          card.Brand,
		Last4:          card.Last4,
		CardExpiry:     card.CardExpiry,
		IsActive:       card.IsActive,
		IsDefault:      card.IsDefault,
		CardType:       card.CardType,
		CardNickname:   card.CardNickname,
		SpendingLimit:  card.SpendingLimit,
		RemainingLimit: card.RemainingLimit,
		UsageCount:     int32(card.UsageCount),
		MaxUsageCount:  int32(card.MaxUsageCount),
		Currency:       card.Currency,
		BillingAddress: card.BillingAddress,
		Status:         card.Status,
		FrozenReason:   card.FrozenReason,
		CreatedAt:      timestamppb.New(card.CreatedAt),
		UpdatedAt:      timestamppb.New(card.UpdatedAt),
	}

	if card.ExpiresAt != nil {
		pbCard.ExpiresAt = timestamppb.New(*card.ExpiresAt)
	}
	if card.LastUsedAt != nil {
		pbCard.LastUsedAt = timestamppb.New(*card.LastUsedAt)
	}

	// Only include sensitive data if explicitly requested
	if includeSecureData {
		pbCard.CardNumber = card.CardNumber
		pbCard.Cvv = card.CVV
	}

	return pbCard
}

// getUserIDFromContext extracts user ID from context
func (c *AccountCardController) getUserIDFromContext(ctx context.Context) (uint, error) {
	// Get auth payload from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok || authPayload == nil {
		return 0, status.Error(codes.Unauthenticated, "missing authentication payload")
	}

	// Get user by email from token
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return 0, status.Errorf(codes.Unauthenticated, "user from token not found: %v", err)
		}
		return 0, status.Errorf(codes.Internal, "failed to retrieve user: %v", err)
	}

	return user.ID, nil
}

// CreateVirtualCard creates a new virtual card
func (c *AccountCardController) CreateVirtualCard(ctx context.Context, req *pb.CreateVirtualCardRequest) (*pb.CreateVirtualCardResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Set defaults
	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}

	card, err := c.cardService.CreateVirtualCard(
		userID,
		uint(req.AccountId),
		"Card Holder", // TODO: Get from user profile
		currency,
		req.BillingAddress,
		req.CardNickname,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create virtual card: %v", err)
	}

	return &pb.CreateVirtualCardResponse{
		Card: convertToProto(card, true), // Include full details for creation
	}, nil
}

// CreateDisposableCard creates a new disposable card
func (c *AccountCardController) CreateDisposableCard(ctx context.Context, req *pb.CreateDisposableCardRequest) (*pb.CreateDisposableCardResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}

	card, err := c.cardService.CreateDisposableCard(
		userID,
		uint(req.AccountId),
		"Card Holder",
		currency,
		req.BillingAddress,
		req.CardNickname,
		req.SpendingLimit,
		int(req.MaxUsageCount),
		int(req.ExpiresInHours),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create disposable card: %v", err)
	}

	return &pb.CreateDisposableCardResponse{
		Card: convertToProto(card, true),
	}, nil
}

// GetUserCards retrieves all cards for a user
func (c *AccountCardController) GetUserCards(ctx context.Context, req *pb.GetUserCardsRequest) (*pb.GetUserCardsResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	cards, err := c.cardService.GetUserCards(userID, req.CardTypeFilter, req.StatusFilter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get cards: %v", err)
	}

	// Get statistics
	stats, err := c.cardService.GetCardStatistics(userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get statistics: %v", err)
	}

	// Convert cards
	pbCards := make([]*pb.AccountCard, len(cards))
	for i, card := range cards {
		pbCards[i] = convertToProto(&card, false) // Don't include sensitive data in list
	}

	return &pb.GetUserCardsResponse{
		Cards: pbCards,
		Statistics: &pb.CardStatistics{
			TotalCards:            int32(stats["total_cards"].(int64)),
			ActiveCards:           int32(stats["active_cards"].(int64)),
			VirtualCards:          int32(stats["virtual_cards"].(int64)),
			DisposableCards:       int32(stats["disposable_cards"].(int64)),
			FrozenCards:           int32(stats["frozen_cards"].(int64)),
			TotalSpendingLimit:    stats["total_spending_limit"].(float64),
			TotalRemainingLimit:   stats["total_remaining_limit"].(float64),
		},
	}, nil
}

// GetCardDetails retrieves detailed card information
func (c *AccountCardController) GetCardDetails(ctx context.Context, req *pb.GetCardDetailsRequest) (*pb.GetCardDetailsResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	card, err := c.cardService.GetCardByUUID(userID, req.CardUuid)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "card not found: %v", err)
	}

	// Get recent transactions
	transactions, _, err := c.cardService.GetCardTransactions(userID, req.CardUuid, 1, 10)
	if err != nil {
		// Don't fail if transactions can't be fetched
		transactions = []models.CardTransaction{}
	}

	pbTransactions := make([]*pb.CardTransaction, len(transactions))
	for i, tx := range transactions {
		pbTransactions[i] = &pb.CardTransaction{
			Id:              uint64(tx.ID),
			Uuid:            tx.UUID,
			CardId:          uint64(tx.CardID),
			UserId:          uint64(tx.UserID),
			AccountId:       uint64(tx.AccountID),
			Amount:          tx.Amount,
			Currency:        tx.Currency,
			MerchantName:    tx.MerchantName,
			MerchantCategory: tx.MerchantCategory,
			TransactionType: tx.TransactionType,
			Status:          tx.Status,
			DeclineReason:   tx.DeclineReason,
			AuthorizationCode: tx.AuthorizationCode,
			Description:     tx.Description,
			TransactionDate: timestamppb.New(tx.TransactionDate),
			CreatedAt:       timestamppb.New(tx.CreatedAt),
		}
		if tx.SettledAt != nil {
			pbTransactions[i].SettledAt = timestamppb.New(*tx.SettledAt)
		}
	}

	return &pb.GetCardDetailsResponse{
		Card:              convertToProto(card, req.IncludeFullDetails),
		RecentTransactions: pbTransactions,
	}, nil
}

// FreezeCard freezes a card
func (c *AccountCardController) FreezeCard(ctx context.Context, req *pb.FreezeCardRequest) (*pb.FreezeCardResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	card, err := c.cardService.FreezeCard(userID, req.CardUuid, req.Reason)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to freeze card: %v", err)
	}

	return &pb.FreezeCardResponse{
		Card: convertToProto(card, false),
	}, nil
}

// UnfreezeCard unfreezes a card
func (c *AccountCardController) UnfreezeCard(ctx context.Context, req *pb.UnfreezeCardRequest) (*pb.UnfreezeCardResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	card, err := c.cardService.UnfreezeCard(userID, req.CardUuid)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unfreeze card: %v", err)
	}

	return &pb.UnfreezeCardResponse{
		Card: convertToProto(card, false),
	}, nil
}

// CancelCard cancels a card
func (c *AccountCardController) CancelCard(ctx context.Context, req *pb.CancelCardRequest) (*pb.CancelCardResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	if err := c.cardService.CancelCard(userID, req.CardUuid, req.Reason); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to cancel card: %v", err)
	}

	return &pb.CancelCardResponse{
		Success: true,
		Message: "Card cancelled successfully",
	}, nil
}

// UpdateCardNickname updates card nickname
func (c *AccountCardController) UpdateCardNickname(ctx context.Context, req *pb.UpdateCardNicknameRequest) (*pb.UpdateCardNicknameResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	card, err := c.cardService.UpdateCardNickname(userID, req.CardUuid, req.Nickname)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update nickname: %v", err)
	}

	return &pb.UpdateCardNicknameResponse{
		Card: convertToProto(card, false),
	}, nil
}

// UpdateCardSpendingLimit updates spending limit
func (c *AccountCardController) UpdateCardSpendingLimit(ctx context.Context, req *pb.UpdateCardSpendingLimitRequest) (*pb.UpdateCardSpendingLimitResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	card, err := c.cardService.UpdateCardSpendingLimit(userID, req.CardUuid, req.NewLimit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update limit: %v", err)
	}

	return &pb.UpdateCardSpendingLimitResponse{
		Card: convertToProto(card, false),
	}, nil
}

// GetCardTransactions retrieves card transactions
func (c *AccountCardController) GetCardTransactions(ctx context.Context, req *pb.GetCardTransactionsRequest) (*pb.GetCardTransactionsResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	page := int(req.Page)
	limit := int(req.Limit)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	transactions, total, err := c.cardService.GetCardTransactions(userID, req.CardUuid, page, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get transactions: %v", err)
	}

	pbTransactions := make([]*pb.CardTransaction, len(transactions))
	for i, tx := range transactions {
		pbTransactions[i] = &pb.CardTransaction{
			Id:              uint64(tx.ID),
			Uuid:            tx.UUID,
			CardId:          uint64(tx.CardID),
			UserId:          uint64(tx.UserID),
			AccountId:       uint64(tx.AccountID),
			Amount:          tx.Amount,
			Currency:        tx.Currency,
			MerchantName:    tx.MerchantName,
			MerchantCategory: tx.MerchantCategory,
			TransactionType: tx.TransactionType,
			Status:          tx.Status,
			DeclineReason:   tx.DeclineReason,
			AuthorizationCode: tx.AuthorizationCode,
			Description:     tx.Description,
			TransactionDate: timestamppb.New(tx.TransactionDate),
			CreatedAt:       timestamppb.New(tx.CreatedAt),
		}
		if tx.SettledAt != nil {
			pbTransactions[i].SettledAt = timestamppb.New(*tx.SettledAt)
		}
	}

	totalPages := int32(math.Ceil(float64(total) / float64(limit)))

	return &pb.GetCardTransactionsResponse{
		Transactions: pbTransactions,
		TotalCount:   int32(total),
		CurrentPage:  int32(page),
		TotalPages:   totalPages,
	}, nil
}

// SetDefaultCard sets a card as default
func (c *AccountCardController) SetDefaultCard(ctx context.Context, req *pb.SetDefaultCardRequest) (*pb.SetDefaultCardResponse, error) {
	userID, err := c.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	card, err := c.cardService.SetDefaultCard(userID, req.CardUuid)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to set default card: %v", err)
	}

	return &pb.SetDefaultCardResponse{
		Card: convertToProto(card, false),
	}, nil
}

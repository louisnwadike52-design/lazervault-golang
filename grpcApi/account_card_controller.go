package grpcApi

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	// Keep gorm import if needed for other methods or future use
)

// AccountCardController handles gRPC requests related to account cards.
type AccountCardController struct {
	pb.UnimplementedAccountCardServiceServer // Embed for forward compatibility
	cardService                              services.IAccountCardService
	userService                              services.IUserService // Add userService dependency
	// db                                       *gorm.DB // Keep db if needed, or remove if service handles all
}

// NewAccountCardController creates a new AccountCardController.
// Note: Removed db from parameters as it's not used directly in the added methods.
// Add it back if other controller methods need direct db access.
func NewAccountCardController(cardService services.IAccountCardService, userService services.IUserService) *AccountCardController {
	return &AccountCardController{
		cardService: cardService,
		userService: userService,
		// db:          db, // Uncomment if db is needed
	}
}

// convertAccountCard converts a models.AccountCard to a pb.AccountCard.
func convertAccountCard(card *models.AccountCard) *pb.AccountCard {
	if card == nil {
		return nil
	}
	return &pb.AccountCard{
		Id:             uint64(card.ID),
		AccountId:      uint64(card.AccountID),
		CardHolderName: card.CardHolderName,
		Brand:          card.Brand,
		Last4:          card.Last4,
		CardExpiry:     card.CardExpiry,
		IsActive:       card.IsActive,
		IsDefault:      card.IsDefault,
		CreatedAt:      timestamppb.New(card.CreatedAt),
		UpdatedAt:      timestamppb.New(card.UpdatedAt),
		CardType:       card.CardType, // Add CardType mapping
	}
}

// AddAccountCard handles the RPC for adding a new card to an account.
func (c *AccountCardController) AddAccountCard(ctx context.Context, req *pb.AddAccountCardRequest) (*pb.AddAccountCardResponse, error) {
	// 1. Get Payload & User ID
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	ownerUserID := user.ID // Use the ID from the fetched user model

	// 2. Validate Request
	if req.GetAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "account_id is required")
	}
	if req.GetCardHolderName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "card_holder_name is required")
	}
	if req.GetCardNumber() == "" { // TODO: Add Luhn check validation in service
		return nil, status.Errorf(codes.InvalidArgument, "card_number is required")
	}
	if req.GetCardExpiry() == "" { // TODO: Add MM/YY format validation in service
		return nil, status.Errorf(codes.InvalidArgument, "card_expiry is required")
	}
	// Validate Card Type (Basic check here, more robust in service)
	if req.GetCardType() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "card_type is required")
	}
	switch req.GetCardType() {
	case models.CardTypeVirtual,
		models.CardTypeDisposable,
		models.CardTypePermanent:
		// Valid type
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid card_type specified: %s", req.GetCardType())
	}

	// 3. Prepare Service Request
	serviceReq := services.AddAccountCardRequest{
		OwnerUserID:    ownerUserID,
		AccountID:      uint(req.GetAccountId()),
		CardHolderName: req.GetCardHolderName(),
		CardNumber:     req.GetCardNumber(),
		CardExpiry:     req.GetCardExpiry(),
		CardType:       req.GetCardType(),
		MakeDefault:    req.GetMakeDefault(),
	}

	// 4. Call Service
	newCard, err := c.cardService.AddAccountCard(ctx, serviceReq)
	if err != nil {
		if err.Error() == "account not found or user mismatch" {
			return nil, status.Errorf(codes.NotFound, "target account not found or permission denied")
		}
		if errors.Is(err, services.ErrInvalidCardNumber) || errors.Is(err, services.ErrInvalidExpiryFormat) || errors.Is(err, services.ErrExpiryDateInPast) || errors.Is(err, services.ErrInvalidCardType) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid card details: %v", err)
		}
		if errors.Is(err, services.ErrEncryptionFailed) {
			fmt.Printf("CRITICAL: Card encryption failed during AddAccountCard: %v\n", err)
			return nil, status.Error(codes.Internal, "failed to process card")
		}
		return nil, status.Errorf(codes.Internal, "failed to add card: %v", err)
	}

	// 5. Convert and Return Response
	resp := &pb.AddAccountCardResponse{
		Card: convertAccountCard(newCard),
	}
	return resp, nil
}

// GetAccountCards handles the RPC for retrieving cards associated with an account.
func (c *AccountCardController) GetAccountCards(ctx context.Context, req *pb.GetAccountCardsRequest) (*pb.GetAccountCardsResponse, error) {
	// 1. Get Payload & User ID
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	ownerUserID := user.ID

	// 2. Validate Request
	if req.GetAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "account_id is required")
	}

	// 3. Prepare Service Request
	serviceReq := services.GetAccountCardsRequest{
		OwnerUserID: ownerUserID,
		AccountID:   uint(req.GetAccountId()),
	}

	// 4. Call Service
	cards, err := c.cardService.GetAccountCards(ctx, serviceReq)
	if err != nil {
		// Handle potential errors like account not found or permission denied
		if err.Error() == "account not found or user mismatch" { // Example error check
			return nil, status.Errorf(codes.NotFound, "account not found or permission denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve cards: %v", err)
	}

	// 5. Convert and Return Response
	pbCards := make([]*pb.AccountCard, len(cards))
	for i, card := range cards {
		pbCards[i] = convertAccountCard(&card)
	}

	resp := &pb.GetAccountCardsResponse{
		Cards: pbCards,
	}
	return resp, nil
}

// UpdateAccountCardDefaultStatus handles setting a card as default.
func (c *AccountCardController) UpdateAccountCardDefaultStatus(ctx context.Context, req *pb.UpdateAccountCardDefaultStatusRequest) (*pb.UpdateAccountCardDefaultStatusResponse, error) {
	// 1. Get Payload & User ID
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	ownerUserID := user.ID

	// 2. Validate Request
	if req.GetAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "account_id is required")
	}
	if req.GetCardId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "card_id is required")
	}

	// 3. Prepare Service Request
	serviceReq := services.UpdateAccountCardDefaultStatusRequest{
		OwnerUserID: ownerUserID,
		AccountID:   uint(req.GetAccountId()),
		CardID:      uint(req.GetCardId()),
	}

	// 4. Call Service
	updatedCard, err := c.cardService.UpdateAccountCardDefaultStatus(ctx, serviceReq)
	if err != nil {
		if errors.Is(err, services.ErrCardNotFound) {
			return nil, status.Errorf(codes.NotFound, "card or account not found, or permission denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to update default card status: %v", err)
	}

	// 5. Convert and Return Response
	resp := &pb.UpdateAccountCardDefaultStatusResponse{
		Card: convertAccountCard(updatedCard),
	}
	return resp, nil
}

// DeleteAccountCard handles removing a card.
func (c *AccountCardController) DeleteAccountCard(ctx context.Context, req *pb.DeleteAccountCardRequest) (*emptypb.Empty, error) {
	// 1. Get Payload & User ID
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing authorization payload")
	}
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	ownerUserID := user.ID

	// 2. Validate Request
	if req.GetAccountId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "account_id is required")
	}
	if req.GetCardId() == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "card_id is required")
	}

	// 3. Prepare Service Request
	serviceReq := services.DeleteAccountCardRequest{
		OwnerUserID: ownerUserID,
		AccountID:   uint(req.GetAccountId()),
		CardID:      uint(req.GetCardId()),
	}

	// 4. Call Service
	err = c.cardService.DeleteAccountCard(ctx, serviceReq)
	if err != nil {
		if errors.Is(err, services.ErrCardNotFound) {
			return nil, status.Errorf(codes.NotFound, "card or account not found, or permission denied")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete card: %v", err)
	}

	// 5. Return Empty response on success
	return &emptypb.Empty{}, nil
}

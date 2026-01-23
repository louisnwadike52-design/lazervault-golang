package grpcApi

import (
	"context"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GiftCardController struct {
	pb.UnimplementedGiftCardServiceServer
	giftCardService services.IGiftCardService
	userService     services.IUserService
}

func NewGiftCardController(giftCardService services.IGiftCardService, userService services.IUserService) *GiftCardController {
	return &GiftCardController{
		giftCardService: giftCardService,
		userService:     userService,
	}
}

// GetGiftCardBrands retrieves list of available gift card brands
func (c *GiftCardController) GetGiftCardBrands(ctx context.Context, req *pb.GetGiftCardBrandsRequest) (*pb.GetGiftCardBrandsResponse, error) {
	resp, err := c.giftCardService.GetGiftCardBrands(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get gift card brands: %v", err)
	}
	return resp, nil
}

// GetGiftCardBrandsByCategory retrieves brands filtered by category
func (c *GiftCardController) GetGiftCardBrandsByCategory(ctx context.Context, req *pb.GetGiftCardBrandsByCategoryRequest) (*pb.GetGiftCardBrandsByCategoryResponse, error) {
	if req.Category == pb.GiftCardCategory_GIFT_CARD_CATEGORY_UNSPECIFIED {
		return nil, status.Errorf(codes.InvalidArgument, "category is required")
	}

	resp, err := c.giftCardService.GetGiftCardBrandsByCategory(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get brands by category: %v", err)
	}
	return resp, nil
}

// SearchGiftCardBrands searches for gift card brands
func (c *GiftCardController) SearchGiftCardBrands(ctx context.Context, req *pb.SearchGiftCardBrandsRequest) (*pb.SearchGiftCardBrandsResponse, error) {
	if req.Query == "" {
		return nil, status.Errorf(codes.InvalidArgument, "search query is required")
	}

	resp, err := c.giftCardService.SearchGiftCardBrands(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search brands: %v", err)
	}
	return resp, nil
}

// GetMyGiftCards retrieves user's purchased gift cards
func (c *GiftCardController) GetMyGiftCards(ctx context.Context, req *pb.GetMyGiftCardsRequest) (*pb.GetMyGiftCardsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	resp, err := c.giftCardService.GetMyGiftCards(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get gift cards: %v", err)
	}
	return resp, nil
}

// GetGiftCardBrandById retrieves specific brand details
func (c *GiftCardController) GetGiftCardBrandById(ctx context.Context, req *pb.GetGiftCardBrandByIdRequest) (*pb.GetGiftCardBrandByIdResponse, error) {
	if req.BrandId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "brand id is required")
	}

	resp, err := c.giftCardService.GetGiftCardBrandById(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "brand not found: %v", err)
	}
	return resp, nil
}

// GetPopularBrands retrieves popular gift card brands
func (c *GiftCardController) GetPopularBrands(ctx context.Context, req *pb.GetPopularBrandsRequest) (*pb.GetPopularBrandsResponse, error) {
	resp, err := c.giftCardService.GetPopularBrands(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get popular brands: %v", err)
	}
	return resp, nil
}

// PurchaseGiftCard processes gift card purchase
func (c *GiftCardController) PurchaseGiftCard(ctx context.Context, req *pb.PurchaseGiftCardRequest) (*pb.PurchaseGiftCardResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.BrandId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "brand id is required")
	}
	if req.Amount <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be greater than 0")
	}
	if req.Currency == "" {
		return nil, status.Errorf(codes.InvalidArgument, "currency is required")
	}

	resp, err := c.giftCardService.PurchaseGiftCard(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to purchase gift card: %v", err)
	}
	return resp, nil
}

// GetGiftCardById retrieves specific gift card
func (c *GiftCardController) GetGiftCardById(ctx context.Context, req *pb.GetGiftCardByIdRequest) (*pb.GetGiftCardByIdResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.GiftCardId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "gift card id is required")
	}

	resp, err := c.giftCardService.GetGiftCardById(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "gift card not found: %v", err)
	}
	return resp, nil
}

// RedeemGiftCard processes gift card redemption
func (c *GiftCardController) RedeemGiftCard(ctx context.Context, req *pb.RedeemGiftCardRequest) (*pb.RedeemGiftCardResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.GiftCardId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "gift card id is required")
	}
	if req.Code == "" {
		return nil, status.Errorf(codes.InvalidArgument, "code is required")
	}
	if req.Pin == "" {
		return nil, status.Errorf(codes.InvalidArgument, "pin is required")
	}

	resp, err := c.giftCardService.RedeemGiftCard(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to redeem gift card: %v", err)
	}
	return resp, nil
}

// GetGiftCardTransactions retrieves transactions for a gift card
func (c *GiftCardController) GetGiftCardTransactions(ctx context.Context, req *pb.GetGiftCardTransactionsRequest) (*pb.GetGiftCardTransactionsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	if req.GiftCardId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "gift card id is required")
	}

	resp, err := c.giftCardService.GetGiftCardTransactions(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get transactions: %v", err)
	}
	return resp, nil
}

// GetUserTransactions retrieves user's gift card transactions
func (c *GiftCardController) GetUserTransactions(ctx context.Context, req *pb.GetUserTransactionsRequest) (*pb.GetUserTransactionsResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	resp, err := c.giftCardService.GetUserTransactions(ctx, user.ID, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user transactions: %v", err)
	}
	return resp, nil
}

package services

import (
	"context"
	"fmt"
	"lazervaultGo/pb"
	"math"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// IGiftCardService defines the interface for gift card services
type IGiftCardService interface {
	GetGiftCardBrands(ctx context.Context, req *pb.GetGiftCardBrandsRequest) (*pb.GetGiftCardBrandsResponse, error)
	GetGiftCardBrandsByCategory(ctx context.Context, req *pb.GetGiftCardBrandsByCategoryRequest) (*pb.GetGiftCardBrandsByCategoryResponse, error)
	SearchGiftCardBrands(ctx context.Context, req *pb.SearchGiftCardBrandsRequest) (*pb.SearchGiftCardBrandsResponse, error)
	GetMyGiftCards(ctx context.Context, userID uint, req *pb.GetMyGiftCardsRequest) (*pb.GetMyGiftCardsResponse, error)
	GetGiftCardBrandById(ctx context.Context, req *pb.GetGiftCardBrandByIdRequest) (*pb.GetGiftCardBrandByIdResponse, error)
	GetPopularBrands(ctx context.Context, req *pb.GetPopularBrandsRequest) (*pb.GetPopularBrandsResponse, error)
	PurchaseGiftCard(ctx context.Context, userID uint, req *pb.PurchaseGiftCardRequest) (*pb.PurchaseGiftCardResponse, error)
	GetGiftCardById(ctx context.Context, userID uint, req *pb.GetGiftCardByIdRequest) (*pb.GetGiftCardByIdResponse, error)
	RedeemGiftCard(ctx context.Context, userID uint, req *pb.RedeemGiftCardRequest) (*pb.RedeemGiftCardResponse, error)
	GetGiftCardTransactions(ctx context.Context, userID uint, req *pb.GetGiftCardTransactionsRequest) (*pb.GetGiftCardTransactionsResponse, error)
	GetUserTransactions(ctx context.Context, userID uint, req *pb.GetUserTransactionsRequest) (*pb.GetUserTransactionsResponse, error)
}

// GiftCardService implements IGiftCardService
type GiftCardService struct {
	// In production, this would have Reloadly API client
	mockBrands       []*pb.GiftCardBrandMessage
	mockGiftCards    map[uint][]*pb.GiftCardMessage // userID -> gift cards
	mockTransactions map[string][]*pb.GiftCardTransactionMessage
}

// NewGiftCardService creates a new gift card service
func NewGiftCardService() IGiftCardService {
	service := &GiftCardService{
		mockGiftCards:    make(map[uint][]*pb.GiftCardMessage),
		mockTransactions: make(map[string][]*pb.GiftCardTransactionMessage),
	}
	service.initMockData()
	return service
}

func (s *GiftCardService) initMockData() {
	s.mockBrands = []*pb.GiftCardBrandMessage{
		{
			Id:                     "amazon",
			Name:                   "Amazon",
			LogoUrl:                "https://example.com/amazon-logo.png",
			Description:            "Shop millions of products on Amazon",
			Category:               pb.GiftCardCategory_GIFT_CARD_CATEGORY_SHOPPING,
			DiscountPercentage:     2.5,
			IsPopular:              true,
			AvailableDenominations: []string{"25", "50", "100", "200"},
			MinAmount:              "10",
			MaxAmount:              "500",
			Country:                "US",
		},
		{
			Id:                     "netflix",
			Name:                   "Netflix",
			LogoUrl:                "https://example.com/netflix-logo.png",
			Description:            "Stream unlimited movies and TV shows",
			Category:               pb.GiftCardCategory_GIFT_CARD_CATEGORY_ENTERTAINMENT,
			DiscountPercentage:     1.5,
			IsPopular:              true,
			AvailableDenominations: []string{"30", "60", "100"},
			MinAmount:              "15",
			MaxAmount:              "200",
			Country:                "US",
		},
		{
			Id:                     "starbucks",
			Name:                   "Starbucks",
			LogoUrl:                "https://example.com/starbucks-logo.png",
			Description:            "Your favorite coffee and beverages",
			Category:               pb.GiftCardCategory_GIFT_CARD_CATEGORY_DINING,
			DiscountPercentage:     3.0,
			IsPopular:              true,
			AvailableDenominations: []string{"10", "25", "50"},
			MinAmount:              "5",
			MaxAmount:              "100",
			Country:                "US",
		},
		{
			Id:                     "steam",
			Name:                   "Steam",
			LogoUrl:                "https://example.com/steam-logo.png",
			Description:            "The ultimate gaming platform",
			Category:               pb.GiftCardCategory_GIFT_CARD_CATEGORY_GAMING,
			DiscountPercentage:     2.0,
			IsPopular:              true,
			AvailableDenominations: []string{"20", "50", "100"},
			MinAmount:              "5",
			MaxAmount:              "200",
			Country:                "US",
		},
		{
			Id:                     "airbnb",
			Name:                   "Airbnb",
			LogoUrl:                "https://example.com/airbnb-logo.png",
			Description:            "Book unique homes and experiences",
			Category:               pb.GiftCardCategory_GIFT_CARD_CATEGORY_TRAVEL,
			DiscountPercentage:     1.0,
			IsPopular:              false,
			AvailableDenominations: []string{"50", "100", "200"},
			MinAmount:              "25",
			MaxAmount:              "500",
			Country:                "US",
		},
	}
}

func (s *GiftCardService) GetGiftCardBrands(ctx context.Context, req *pb.GetGiftCardBrandsRequest) (*pb.GetGiftCardBrandsResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	// Filter by country if specified
	brands := s.mockBrands
	if req.Country != "" {
		filtered := []*pb.GiftCardBrandMessage{}
		for _, brand := range brands {
			if brand.Country == req.Country {
				filtered = append(filtered, brand)
			}
		}
		brands = filtered
	}

	// Pagination
	start := int((page - 1) * perPage)
	end := int(page * perPage)
	if start >= len(brands) {
		brands = []*pb.GiftCardBrandMessage{}
	} else {
		if end > len(brands) {
			end = len(brands)
		}
		brands = brands[start:end]
	}

	totalItems := int32(len(s.mockBrands))
	totalPages := int32(math.Ceil(float64(totalItems) / float64(perPage)))

	return &pb.GetGiftCardBrandsResponse{
		Brands: brands,
		Pagination: &pb.GiftCardPaginationInfo{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: perPage,
			HasNext:      page < totalPages,
			HasPrev:      page > 1,
		},
	}, nil
}

func (s *GiftCardService) GetGiftCardBrandsByCategory(ctx context.Context, req *pb.GetGiftCardBrandsByCategoryRequest) (*pb.GetGiftCardBrandsByCategoryResponse, error) {
	filtered := []*pb.GiftCardBrandMessage{}
	for _, brand := range s.mockBrands {
		if brand.Category == req.Category {
			if req.Country == "" || brand.Country == req.Country {
				filtered = append(filtered, brand)
			}
		}
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	totalItems := int32(len(filtered))
	totalPages := int32(math.Ceil(float64(totalItems) / float64(perPage)))

	start := int((page - 1) * perPage)
	end := int(page * perPage)
	if start >= len(filtered) {
		filtered = []*pb.GiftCardBrandMessage{}
	} else {
		if end > len(filtered) {
			end = len(filtered)
		}
		filtered = filtered[start:end]
	}

	return &pb.GetGiftCardBrandsByCategoryResponse{
		Brands: filtered,
		Pagination: &pb.GiftCardPaginationInfo{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: perPage,
			HasNext:      page < totalPages,
			HasPrev:      page > 1,
		},
	}, nil
}

func (s *GiftCardService) SearchGiftCardBrands(ctx context.Context, req *pb.SearchGiftCardBrandsRequest) (*pb.SearchGiftCardBrandsResponse, error) {
	if req.Query == "" {
		return &pb.SearchGiftCardBrandsResponse{
			Brands:     []*pb.GiftCardBrandMessage{},
			Pagination: &pb.GiftCardPaginationInfo{},
		}, nil
	}

	filtered := []*pb.GiftCardBrandMessage{}
	query := req.Query
	for _, brand := range s.mockBrands {
		if contains(brand.Name, query) || contains(brand.Description, query) {
			if req.Country == "" || brand.Country == req.Country {
				filtered = append(filtered, brand)
			}
		}
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	totalItems := int32(len(filtered))
	totalPages := int32(math.Ceil(float64(totalItems) / float64(perPage)))

	return &pb.SearchGiftCardBrandsResponse{
		Brands: filtered,
		Pagination: &pb.GiftCardPaginationInfo{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: perPage,
			HasNext:      page < totalPages,
			HasPrev:      page > 1,
		},
	}, nil
}

func (s *GiftCardService) GetMyGiftCards(ctx context.Context, userID uint, req *pb.GetMyGiftCardsRequest) (*pb.GetMyGiftCardsResponse, error) {
	cards, exists := s.mockGiftCards[userID]
	if !exists {
		cards = []*pb.GiftCardMessage{}
	}

	// Filter by status if specified
	if req.Status != pb.GiftCardStatus_GIFT_CARD_STATUS_UNSPECIFIED {
		filtered := []*pb.GiftCardMessage{}
		for _, card := range cards {
			if card.Status == req.Status {
				filtered = append(filtered, card)
			}
		}
		cards = filtered
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	totalItems := int32(len(cards))
	totalPages := int32(math.Ceil(float64(totalItems) / float64(perPage)))

	return &pb.GetMyGiftCardsResponse{
		GiftCards: cards,
		Pagination: &pb.GiftCardPaginationInfo{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: perPage,
			HasNext:      page < totalPages,
			HasPrev:      page > 1,
		},
	}, nil
}

func (s *GiftCardService) GetGiftCardBrandById(ctx context.Context, req *pb.GetGiftCardBrandByIdRequest) (*pb.GetGiftCardBrandByIdResponse, error) {
	for _, brand := range s.mockBrands {
		if brand.Id == req.BrandId {
			return &pb.GetGiftCardBrandByIdResponse{Brand: brand}, nil
		}
	}
	return nil, fmt.Errorf("brand not found")
}

func (s *GiftCardService) GetPopularBrands(ctx context.Context, req *pb.GetPopularBrandsRequest) (*pb.GetPopularBrandsResponse, error) {
	popular := []*pb.GiftCardBrandMessage{}
	for _, brand := range s.mockBrands {
		if brand.IsPopular {
			if req.Country == "" || brand.Country == req.Country {
				popular = append(popular, brand)
			}
		}
	}

	limit := int(req.Limit)
	if limit < 1 {
		limit = 10
	}
	if len(popular) > limit {
		popular = popular[:limit]
	}

	return &pb.GetPopularBrandsResponse{Brands: popular}, nil
}

func (s *GiftCardService) PurchaseGiftCard(ctx context.Context, userID uint, req *pb.PurchaseGiftCardRequest) (*pb.PurchaseGiftCardResponse, error) {
	// Find brand
	var brand *pb.GiftCardBrandMessage
	for _, b := range s.mockBrands {
		if b.Id == req.BrandId {
			brand = b
			break
		}
	}
	if brand == nil {
		return nil, fmt.Errorf("brand not found")
	}

	// Calculate final price with discount
	finalPrice := req.Amount * (1 - brand.DiscountPercentage/100)

	// Create gift card
	giftCardID := fmt.Sprintf("gc-%d-%d", userID, time.Now().Unix())
	transactionID := fmt.Sprintf("tx-%d-%d", userID, time.Now().Unix())

	giftCard := &pb.GiftCardMessage{
		Id:                     giftCardID,
		BrandId:                brand.Id,
		BrandName:              brand.Name,
		LogoUrl:                brand.LogoUrl,
		Amount:                 req.Amount,
		DiscountPercentage:     brand.DiscountPercentage,
		FinalPrice:             finalPrice,
		Currency:               req.Currency,
		Status:                 pb.GiftCardStatus_GIFT_CARD_STATUS_ACTIVE,
		Type:                   pb.GiftCardType_GIFT_CARD_TYPE_DIGITAL,
		Category:               brand.Category,
		Description:            brand.Description,
		TermsAndConditions:     "Standard terms and conditions apply",
		ExpiryDate:             timestamppb.New(time.Now().AddDate(1, 0, 0)),
		PurchaseDate:           timestamppb.New(time.Now()),
		RecipientEmail:         req.RecipientEmail,
		RecipientName:          req.RecipientName,
		Message:                req.Message,
		Code:                   generateMockCode(),
		Pin:                    generateMockPin(),
		IsRedeemed:             false,
		TransactionId:          transactionID,
		AvailableDenominations: brand.AvailableDenominations,
	}

	// Store gift card
	if _, exists := s.mockGiftCards[userID]; !exists {
		s.mockGiftCards[userID] = []*pb.GiftCardMessage{}
	}
	s.mockGiftCards[userID] = append(s.mockGiftCards[userID], giftCard)

	// Create transaction
	transaction := &pb.GiftCardTransactionMessage{
		Id:              transactionID,
		GiftCardId:      giftCardID,
		UserId:          fmt.Sprintf("%d", userID),
		Amount:          finalPrice,
		Currency:        req.Currency,
		TransactionDate: timestamppb.New(time.Now()),
		TransactionType: "purchase",
		Status:          pb.GiftCardStatus_GIFT_CARD_STATUS_ACTIVE,
	}

	// Store transaction
	s.mockTransactions[giftCardID] = []*pb.GiftCardTransactionMessage{transaction}

	return &pb.PurchaseGiftCardResponse{
		GiftCard:    giftCard,
		Transaction: transaction,
	}, nil
}

func (s *GiftCardService) GetGiftCardById(ctx context.Context, userID uint, req *pb.GetGiftCardByIdRequest) (*pb.GetGiftCardByIdResponse, error) {
	cards, exists := s.mockGiftCards[userID]
	if !exists {
		return nil, fmt.Errorf("gift card not found")
	}

	for _, card := range cards {
		if card.Id == req.GiftCardId {
			return &pb.GetGiftCardByIdResponse{GiftCard: card}, nil
		}
	}

	return nil, fmt.Errorf("gift card not found")
}

func (s *GiftCardService) RedeemGiftCard(ctx context.Context, userID uint, req *pb.RedeemGiftCardRequest) (*pb.RedeemGiftCardResponse, error) {
	cards, exists := s.mockGiftCards[userID]
	if !exists {
		return nil, fmt.Errorf("gift card not found")
	}

	for i, card := range cards {
		if card.Id == req.GiftCardId {
			if card.Code != req.Code || card.Pin != req.Pin {
				return &pb.RedeemGiftCardResponse{
					GiftCard: card,
					Success:  false,
					Message:  "Invalid code or PIN",
				}, nil
			}

			if card.IsRedeemed {
				return &pb.RedeemGiftCardResponse{
					GiftCard: card,
					Success:  false,
					Message:  "Gift card already redeemed",
				}, nil
			}

			// Mark as redeemed
			card.IsRedeemed = true
			card.Status = pb.GiftCardStatus_GIFT_CARD_STATUS_USED
			s.mockGiftCards[userID][i] = card

			// Add redeem transaction
			transactionID := fmt.Sprintf("tx-redeem-%d-%d", userID, time.Now().Unix())
			transaction := &pb.GiftCardTransactionMessage{
				Id:              transactionID,
				GiftCardId:      card.Id,
				UserId:          fmt.Sprintf("%d", userID),
				Amount:          card.Amount,
				Currency:        card.Currency,
				TransactionDate: timestamppb.New(time.Now()),
				TransactionType: "redeem",
				Status:          pb.GiftCardStatus_GIFT_CARD_STATUS_USED,
			}
			s.mockTransactions[card.Id] = append(s.mockTransactions[card.Id], transaction)

			return &pb.RedeemGiftCardResponse{
				GiftCard: card,
				Success:  true,
				Message:  "Gift card redeemed successfully",
			}, nil
		}
	}

	return nil, fmt.Errorf("gift card not found")
}

func (s *GiftCardService) GetGiftCardTransactions(ctx context.Context, userID uint, req *pb.GetGiftCardTransactionsRequest) (*pb.GetGiftCardTransactionsResponse, error) {
	transactions, exists := s.mockTransactions[req.GiftCardId]
	if !exists {
		transactions = []*pb.GiftCardTransactionMessage{}
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	totalItems := int32(len(transactions))
	totalPages := int32(math.Ceil(float64(totalItems) / float64(perPage)))

	return &pb.GetGiftCardTransactionsResponse{
		Transactions: transactions,
		Pagination: &pb.GiftCardPaginationInfo{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: perPage,
			HasNext:      page < totalPages,
			HasPrev:      page > 1,
		},
	}, nil
}

func (s *GiftCardService) GetUserTransactions(ctx context.Context, userID uint, req *pb.GetUserTransactionsRequest) (*pb.GetUserTransactionsResponse, error) {
	allTransactions := []*pb.GiftCardTransactionMessage{}
	for _, transactions := range s.mockTransactions {
		for _, tx := range transactions {
			if tx.UserId == fmt.Sprintf("%d", userID) {
				if req.TransactionType == "" || tx.TransactionType == req.TransactionType {
					allTransactions = append(allTransactions, tx)
				}
			}
		}
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	perPage := req.PerPage
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	totalItems := int32(len(allTransactions))
	totalPages := int32(math.Ceil(float64(totalItems) / float64(perPage)))

	return &pb.GetUserTransactionsResponse{
		Transactions: allTransactions,
		Pagination: &pb.GiftCardPaginationInfo{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: perPage,
			HasNext:      page < totalPages,
			HasPrev:      page > 1,
		},
	}, nil
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0)
}

func generateMockCode() string {
	return fmt.Sprintf("GC%d", time.Now().Unix()%10000000)
}

func generateMockPin() string {
	return fmt.Sprintf("%04d", time.Now().Unix()%10000)
}

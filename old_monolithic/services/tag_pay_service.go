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
	"log"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type TagPayService struct {
	pb.UnimplementedTagPayServiceServer
	db *gorm.DB
}

func NewTagPayService(db *gorm.DB) *TagPayService {
	return &TagPayService{db: db}
}

// getUserIDFromContext extracts the user ID from the JWT token via email database lookup
func (s *TagPayService) getUserIDFromContext(ctx context.Context) (string, error) {
	authPayload := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if authPayload == nil {
		log.Printf("[TagPayService] ERROR: No auth payload in context")
		return "", status.Errorf(codes.Unauthenticated, "authentication required")
	}

	log.Printf("[TagPayService] Auth payload email: %s", authPayload.Email)

	// Get user by email to get the actual user ID
	user, err := database.FindUserByEmail(s.db, authPayload.Email)
	if err != nil {
		// Check if error is because user doesn't exist
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, database.ErrUserNotFound) {
			log.Printf("[TagPayService] ERROR: User record not found for authenticated email %s", authPayload.Email)
			log.Printf("[TagPayService] This usually means the user authenticated but their profile wasn't created in the database")
			log.Printf("[TagPayService] Attempting to create user record from JWT data...")

			// Auto-create user record from JWT data
			newUser := &models.User{
				Email:     authPayload.Email,
				FirstName: "User",    // Default value
				LastName:  "Account", // Default value
				Role:      "user",
				Verified:  true, // Since they authenticated via JWT
				IsPartial: true, // Mark as partial until they complete profile
			}

			// Create the user
			if createErr := s.db.Create(newUser).Error; createErr != nil {
				log.Printf("[TagPayService] ERROR: Failed to auto-create user: %v", createErr)
				return "", status.Errorf(codes.Internal, "user account not found and failed to create. Please contact support or complete registration. Email: %s", authPayload.Email)
			}

			log.Printf("[TagPayService] SUCCESS: Auto-created user ID %d for email %s", newUser.ID, authPayload.Email)
			userID := newUser.UUID
			return userID, nil
		}

		log.Printf("[TagPayService] ERROR: Database error finding user %s: %v", authPayload.Email, err)
		return "", status.Errorf(codes.Internal, "database error: %v", err)
	}

	userID := user.UUID
	log.Printf("[TagPayService] Resolved user ID: %s (email: %s)", userID, authPayload.Email)

	return userID, nil
}

// CreateTagPay - NOT USED, username is already set in user profile
func (s *TagPayService) CreateTagPay(ctx context.Context, req *pb.CreateTagPayRequest) (*pb.CreateTagPayResponse, error) {
	return &pb.CreateTagPayResponse{
		Success: false,
		Message: "Username is already set in your profile. Use your username as your tag.",
	}, nil
}

// GetTagPay retrieves a user by username
func (s *TagPayService) GetTagPay(ctx context.Context, req *pb.GetTagPayRequest) (*pb.GetTagPayResponse, error) {
	var user models.User
	if err := s.db.Where("LOWER(username) = ?", strings.ToLower(req.TagPay)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.GetTagPayResponse{
				Success: false,
				Message: "User not found",
			}, nil
		}
		return nil, status.Error(codes.Internal, "database error")
	}

	return &pb.GetTagPayResponse{
		Success: true,
		Message: "User found",
		TagPay:  userToTagPayProto(&user),
	}, nil
}

// CheckTagPayAvailability checks if a username is available
func (s *TagPayService) CheckTagPayAvailability(ctx context.Context, req *pb.CheckTagPayAvailabilityRequest) (*pb.CheckTagPayAvailabilityResponse, error) {
	username := strings.ToLower(req.TagPay)

	// Validate format first
	if !isValidTagPay(username) {
		return &pb.CheckTagPayAvailabilityResponse{
			Available: false,
			Message:   "Invalid username format",
		}, nil
	}

	// Check if username exists
	var existing models.User
	err := s.db.Where("LOWER(username) = ?", username).First(&existing).Error

	if err == nil {
		// Username exists, generate suggestions
		suggestions := generateTagPaySuggestions(username)
		return &pb.CheckTagPayAvailabilityResponse{
			Available:   false,
			Message:     "Username already taken",
			Suggestions: suggestions,
		}, nil
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		return &pb.CheckTagPayAvailabilityResponse{
			Available: true,
			Message:   "Username is available",
		}, nil
	}

	return nil, status.Error(codes.Internal, "database error")
}

// SearchTagPay searches for users by username
func (s *TagPayService) SearchTagPay(ctx context.Context, req *pb.SearchTagPayRequest) (*pb.SearchTagPayResponse, error) {
	query := strings.ToLower(req.Query)
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	var users []models.User
	if err := s.db.Where("LOWER(username) LIKE ? OR LOWER(name) LIKE ?",
		"%"+query+"%", "%"+query+"%").
		Where("username IS NOT NULL").
		Limit(int(limit)).
		Find(&users).Error; err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}

	results := make([]*pb.TagPay, len(users))
	for i, user := range users {
		results[i] = userToTagPayProto(&user)
	}

	return &pb.SearchTagPayResponse{
		Results: results,
		Total:   int32(len(results)),
	}, nil
}

// SendMoneyTagPay sends money using username
func (s *TagPayService) SendMoneyTagPay(ctx context.Context, req *pb.SendMoneyTagPayRequest) (*pb.SendMoneyTagPayResponse, error) {
	// Get sender ID from context
	senderID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get sender user
	var sender models.User
	if err := s.db.Where("user_id = ?", senderID).First(&sender).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to get sender")
	}

	// Check if sender has username
	if sender.Username == nil || *sender.Username == "" {
		return &pb.SendMoneyTagPayResponse{
			Success: false,
			Message: "You need to set a username first",
		}, nil
	}

	// Get receiver by username
	var receiver models.User
	if err := s.db.Where("LOWER(username) = ?", strings.ToLower(req.ReceiverTagPay)).First(&receiver).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.SendMoneyTagPayResponse{
				Success: false,
				Message: "Receiver username not found",
			}, nil
		}
		return nil, status.Error(codes.Internal, "database error")
	}

	// Validate amount
	if req.Amount <= 0 {
		return &pb.SendMoneyTagPayResponse{
			Success: false,
			Message: "Invalid amount",
		}, nil
	}

	// Check if sending to self
	if senderID == receiver.UUID {
		return &pb.SendMoneyTagPayResponse{
			Success: false,
			Message: "Cannot send money to yourself",
		}, nil
	}

	// TODO: Verify transaction PIN
	// TODO: Check account balance
	// TODO: Process actual money transfer

	// Create transaction record
	transaction := models.TagPayTransaction{
		SenderID:       senderID,
		SenderTagPay:   *sender.Username,
		SenderName:     sender.FirstName + " " + sender.LastName,
		ReceiverID:     receiver.UUID,
		ReceiverTagPay: *receiver.Username,
		ReceiverName:   receiver.FirstName + " " + receiver.LastName,
		Amount:         req.Amount,
		Currency:       req.Currency,
		Description:    req.Description,
		Status:         models.TagPayTransactionStatusCompleted,
		Type:           models.TagPayTransactionTypeSend,
		AccountID:      &req.SourceAccountId,
	}

	now := time.Now()
	transaction.CompletedAt = &now

	if err := s.db.Create(&transaction).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to create transaction")
	}

	return &pb.SendMoneyTagPayResponse{
		Success:     true,
		Message:     "Money sent successfully",
		Transaction: tagPayTransactionToProto(&transaction),
	}, nil
}

// RequestMoneyTagPay creates a money request
func (s *TagPayService) RequestMoneyTagPay(ctx context.Context, req *pb.RequestMoneyTagPayRequest) (*pb.RequestMoneyTagPayResponse, error) {
	// Get requester ID from context
	requesterID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get requester user
	var requester models.User
	if err := s.db.Where("user_id = ?", requesterID).First(&requester).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to get requester")
	}

	// Check if requester has username
	if requester.Username == nil || *requester.Username == "" {
		return &pb.RequestMoneyTagPayResponse{
			Success: false,
			Message: "You need to set a username first",
		}, nil
	}

	// Get requestee by username
	var requestee models.User
	if err := s.db.Where("LOWER(username) = ?", strings.ToLower(req.RequesteeTagPay)).First(&requestee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.RequestMoneyTagPayResponse{
				Success: false,
				Message: "User username not found",
			}, nil
		}
		return nil, status.Error(codes.Internal, "database error")
	}

	// Validate amount
	if req.Amount <= 0 {
		return &pb.RequestMoneyTagPayResponse{
			Success: false,
			Message: "Invalid amount",
		}, nil
	}

	// Check if requesting from self
	if requesterID == requestee.UUID {
		return &pb.RequestMoneyTagPayResponse{
			Success: false,
			Message: "Cannot request money from yourself",
		}, nil
	}

	// Create money request
	moneyRequest := models.MoneyRequest{
		RequesterID:     requesterID,
		RequesterTagPay: *requester.Username,
		RequesterName:   requester.FirstName + " " + requester.LastName,
		RequesteeID:     requestee.UUID,
		RequesteeTagPay: *requestee.Username,
		RequesteeName:   requestee.FirstName + " " + requestee.LastName,
		Amount:          req.Amount,
		Currency:        req.Currency,
		Description:     req.Description,
		Status:          models.MoneyRequestStatusPending,
	}

	if err := s.db.Create(&moneyRequest).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to create money request")
	}

	return &pb.RequestMoneyTagPayResponse{
		Success:      true,
		Message:      "Money request sent successfully",
		MoneyRequest: moneyRequestToProto(&moneyRequest),
	}, nil
}

// GetTagPayTransactions retrieves tag pay transactions
func (s *TagPayService) GetTagPayTransactions(ctx context.Context, req *pb.GetTagPayTransactionsRequest) (*pb.GetTagPayTransactionsResponse, error) {
	// Get user ID from context
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}
	offset := (page - 1) * limit

	query := s.db.Where("sender_id = ? OR receiver_id = ?", userID, userID)

	var transactions []models.TagPayTransaction
	if err := query.Order("created_at DESC").
		Offset(int(offset)).
		Limit(int(limit)).
		Find(&transactions).Error; err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}

	var total int64
	if err := query.Model(&models.TagPayTransaction{}).Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}

	results := make([]*pb.TagPayTransaction, len(transactions))
	for i, txn := range transactions {
		results[i] = tagPayTransactionToProto(&txn)
	}

	totalPages := int32((total + int64(limit) - 1) / int64(limit))

	return &pb.GetTagPayTransactionsResponse{
		Transactions: results,
		Total:        int32(total),
		Page:         page,
		TotalPages:   totalPages,
	}, nil
}

// AcceptMoneyRequest accepts a money request
func (s *TagPayService) AcceptMoneyRequest(ctx context.Context, req *pb.AcceptMoneyRequestRequest) (*pb.AcceptMoneyRequestResponse, error) {
	// Get user ID from context
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	// Get money request
	var moneyRequest models.MoneyRequest
	if err := s.db.Where("id = ? AND requestee_id = ? AND status = ?",
		req.RequestId, userID, models.MoneyRequestStatusPending).
		First(&moneyRequest).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.AcceptMoneyRequestResponse{
				Success: false,
				Message: "Money request not found or already processed",
			}, nil
		}
		return nil, status.Error(codes.Internal, "database error")
	}

	// Check if expired
	if time.Now().After(moneyRequest.ExpiresAt) {
		moneyRequest.Status = models.MoneyRequestStatusExpired
		s.db.Save(&moneyRequest)
		return &pb.AcceptMoneyRequestResponse{
			Success: false,
			Message: "Money request has expired",
		}, nil
	}

	// TODO: Verify transaction PIN
	// TODO: Check account balance
	// TODO: Process actual money transfer

	// Create transaction
	transaction := models.TagPayTransaction{
		SenderID:       userID,
		SenderTagPay:   moneyRequest.RequesteeTagPay,
		SenderName:     moneyRequest.RequesteeName,
		ReceiverID:     moneyRequest.RequesterID,
		ReceiverTagPay: moneyRequest.RequesterTagPay,
		ReceiverName:   moneyRequest.RequesterName,
		Amount:         moneyRequest.Amount,
		Currency:       moneyRequest.Currency,
		Description:    fmt.Sprintf("Request fulfilled: %s", moneyRequest.Description),
		Status:         models.TagPayTransactionStatusCompleted,
		Type:           models.TagPayTransactionTypeRequestFulfilled,
		AccountID:      &req.SourceAccountId,
	}

	now := time.Now()
	transaction.CompletedAt = &now

	if err := s.db.Create(&transaction).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to create transaction")
	}

	// Update money request status
	moneyRequest.Status = models.MoneyRequestStatusAccepted
	moneyRequest.RespondedAt = &now
	if err := s.db.Save(&moneyRequest).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to update money request")
	}

	return &pb.AcceptMoneyRequestResponse{
		Success:     true,
		Message:     "Money request accepted",
		Transaction: tagPayTransactionToProto(&transaction),
	}, nil
}

// DeclineMoneyRequest declines a money request
func (s *TagPayService) DeclineMoneyRequest(ctx context.Context, req *pb.DeclineMoneyRequestRequest) (*pb.DeclineMoneyRequestResponse, error) {
	// Get user ID from context
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	// Get money request
	var moneyRequest models.MoneyRequest
	if err := s.db.Where("id = ? AND requestee_id = ? AND status = ?",
		req.RequestId, userID, models.MoneyRequestStatusPending).
		First(&moneyRequest).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.DeclineMoneyRequestResponse{
				Success: false,
				Message: "Money request not found or already processed",
			}, nil
		}
		return nil, status.Error(codes.Internal, "database error")
	}

	// Update status
	now := time.Now()
	moneyRequest.Status = models.MoneyRequestStatusDeclined
	moneyRequest.RespondedAt = &now

	if err := s.db.Save(&moneyRequest).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to update money request")
	}

	return &pb.DeclineMoneyRequestResponse{
		Success: true,
		Message: "Money request declined",
	}, nil
}

// GetPendingMoneyRequests gets pending money requests
func (s *TagPayService) GetPendingMoneyRequests(ctx context.Context, req *pb.GetPendingMoneyRequestsRequest) (*pb.GetPendingMoneyRequestsResponse, error) {
	// Get user ID from context
	userID, ok := ctx.Value("user_id").(string)
	if !ok || userID == "" {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}
	offset := (page - 1) * limit

	var query *gorm.DB
	if req.Incoming {
		// Requests TO the user
		query = s.db.Where("requestee_id = ? AND status = ?", userID, models.MoneyRequestStatusPending)
	} else {
		// Requests FROM the user
		query = s.db.Where("requester_id = ? AND status = ?", userID, models.MoneyRequestStatusPending)
	}

	var requests []models.MoneyRequest
	if err := query.Order("created_at DESC").
		Offset(int(offset)).
		Limit(int(limit)).
		Find(&requests).Error; err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}

	var total int64
	if err := query.Model(&models.MoneyRequest{}).Count(&total).Error; err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}

	results := make([]*pb.MoneyRequest, len(requests))
	for i, req := range requests {
		results[i] = moneyRequestToProto(&req)
	}

	totalPages := int32((total + int64(limit) - 1) / int64(limit))

	return &pb.GetPendingMoneyRequestsResponse{
		Requests:   results,
		Total:      int32(total),
		Page:       page,
		TotalPages: totalPages,
	}, nil
}

// Helper functions

func isValidTagPay(tagPay string) bool {
	// Tag pay must be 3-20 characters, alphanumeric and underscores only
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]{3,20}$`, tagPay)
	return matched
}

func generateTagPaySuggestions(baseTagPay string) []string {
	suggestions := []string{
		baseTagPay + "1",
		baseTagPay + "2",
		baseTagPay + "_",
		baseTagPay + "x",
		"the" + baseTagPay,
	}
	return suggestions
}

func userToTagPayProto(user *models.User) *pb.TagPay {
	username := ""
	if user.Username != nil {
		username = *user.Username
	}

	return &pb.TagPay{
		Id:          user.UUID,
		UserId:      user.UUID,
		TagPay:      username,
		DisplayName: user.FirstName + " " + user.LastName,
		AvatarUrl:   "", // TODO: Add avatar URL from user profile if available
		IsActive:    true,
		CreatedAt:   timestamppb.New(user.CreatedAt),
		UpdatedAt:   timestamppb.New(user.UpdatedAt),
	}
}

func tagPayToProto(tagPay *models.TagPay) *pb.TagPay {
	return &pb.TagPay{
		Id:          tagPay.ID,
		UserId:      tagPay.UserID,
		TagPay:      tagPay.TagPay,
		DisplayName: tagPay.DisplayName,
		AvatarUrl:   tagPay.AvatarURL,
		IsActive:    tagPay.IsActive,
		CreatedAt:   timestamppb.New(tagPay.CreatedAt),
		UpdatedAt:   timestamppb.New(tagPay.UpdatedAt),
	}
}

func tagPayTransactionToProto(txn *models.TagPayTransaction) *pb.TagPayTransaction {
	pbTxn := &pb.TagPayTransaction{
		Id:              txn.ID,
		SenderId:        txn.SenderID,
		SenderTagPay:    txn.SenderTagPay,
		SenderName:      txn.SenderName,
		ReceiverId:      txn.ReceiverID,
		ReceiverTagPay:  txn.ReceiverTagPay,
		ReceiverName:    txn.ReceiverName,
		Amount:          txn.Amount,
		Currency:        txn.Currency,
		Description:     txn.Description,
		Status:          tagPayTransactionStatusToProto(txn.Status),
		Type:            tagPayTransactionTypeToProto(txn.Type),
		ReferenceNumber: txn.ReferenceNumber,
		CreatedAt:       timestamppb.New(txn.CreatedAt),
	}

	if txn.CompletedAt != nil {
		pbTxn.CompletedAt = timestamppb.New(*txn.CompletedAt)
	}

	return pbTxn
}

func moneyRequestToProto(req *models.MoneyRequest) *pb.MoneyRequest {
	pbReq := &pb.MoneyRequest{
		Id:              req.ID,
		RequesterId:     req.RequesterID,
		RequesterTagPay: req.RequesterTagPay,
		RequesterName:   req.RequesterName,
		RequesteeId:     req.RequesteeID,
		RequesteeTagPay: req.RequesteeTagPay,
		RequesteeName:   req.RequesteeName,
		Amount:          req.Amount,
		Currency:        req.Currency,
		Description:     req.Description,
		Status:          moneyRequestStatusToProto(req.Status),
		CreatedAt:       timestamppb.New(req.CreatedAt),
		ExpiresAt:       timestamppb.New(req.ExpiresAt),
	}

	if req.RespondedAt != nil {
		pbReq.RespondedAt = timestamppb.New(*req.RespondedAt)
	}

	return pbReq
}

func tagPayTransactionStatusToProto(status models.TagPayTransactionStatus) pb.TagPayTransactionStatus {
	switch status {
	case models.TagPayTransactionStatusPending:
		return pb.TagPayTransactionStatus_TAG_PAY_TRANSACTION_STATUS_PENDING
	case models.TagPayTransactionStatusProcessing:
		return pb.TagPayTransactionStatus_TAG_PAY_TRANSACTION_STATUS_PROCESSING
	case models.TagPayTransactionStatusCompleted:
		return pb.TagPayTransactionStatus_TAG_PAY_TRANSACTION_STATUS_COMPLETED
	case models.TagPayTransactionStatusFailed:
		return pb.TagPayTransactionStatus_TAG_PAY_TRANSACTION_STATUS_FAILED
	case models.TagPayTransactionStatusCancelled:
		return pb.TagPayTransactionStatus_TAG_PAY_TRANSACTION_STATUS_CANCELLED
	case models.TagPayTransactionStatusRefunded:
		return pb.TagPayTransactionStatus_TAG_PAY_TRANSACTION_STATUS_REFUNDED
	default:
		return pb.TagPayTransactionStatus_TAG_PAY_TRANSACTION_STATUS_PENDING
	}
}

func tagPayTransactionTypeToProto(txnType models.TagPayTransactionType) pb.TagPayTransactionType {
	switch txnType {
	case models.TagPayTransactionTypeSend:
		return pb.TagPayTransactionType_TAG_PAY_TRANSACTION_TYPE_SEND
	case models.TagPayTransactionTypeReceive:
		return pb.TagPayTransactionType_TAG_PAY_TRANSACTION_TYPE_RECEIVE
	case models.TagPayTransactionTypeRequest:
		return pb.TagPayTransactionType_TAG_PAY_TRANSACTION_TYPE_REQUEST
	case models.TagPayTransactionTypeRequestFulfilled:
		return pb.TagPayTransactionType_TAG_PAY_TRANSACTION_TYPE_REQUEST_FULFILLED
	default:
		return pb.TagPayTransactionType_TAG_PAY_TRANSACTION_TYPE_SEND
	}
}

func moneyRequestStatusToProto(status models.MoneyRequestStatus) pb.MoneyRequestStatus {
	switch status {
	case models.MoneyRequestStatusPending:
		return pb.MoneyRequestStatus_MONEY_REQUEST_STATUS_PENDING
	case models.MoneyRequestStatusAccepted:
		return pb.MoneyRequestStatus_MONEY_REQUEST_STATUS_ACCEPTED
	case models.MoneyRequestStatusDeclined:
		return pb.MoneyRequestStatus_MONEY_REQUEST_STATUS_DECLINED
	case models.MoneyRequestStatusExpired:
		return pb.MoneyRequestStatus_MONEY_REQUEST_STATUS_EXPIRED
	case models.MoneyRequestStatusCancelled:
		return pb.MoneyRequestStatus_MONEY_REQUEST_STATUS_CANCELLED
	default:
		return pb.MoneyRequestStatus_MONEY_REQUEST_STATUS_PENDING
	}
}

func tagStatusToProto(status models.TagStatus) pb.TagStatus {
	switch status {
	case models.TagStatusPending:
		return pb.TagStatus_TAG_STATUS_PENDING
	case models.TagStatusPaid:
		return pb.TagStatus_TAG_STATUS_PAID
	case models.TagStatusCancelled:
		return pb.TagStatus_TAG_STATUS_CANCELLED
	default:
		return pb.TagStatus_TAG_STATUS_PENDING
	}
}

// CreateTag creates a tag for another user
func (s *TagPayService) CreateTag(ctx context.Context, req *pb.CreateTagRequest) (*pb.CreateTagResponse, error) {
	// Get authenticated user ID from context
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get tagger user
	var taggerUser models.User
	if err := s.db.Where("user_id = ?", userID).First(&taggerUser).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "tagger user not found")
	}

	if taggerUser.Username == nil || *taggerUser.Username == "" {
		return nil, status.Errorf(codes.FailedPrecondition, "you must have a username to create tags")
	}

	// Get tagged user by username
	var taggedUser models.User
	if err := s.db.Where("LOWER(username) = ?", strings.ToLower(req.TaggedUserTagPay)).First(&taggedUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "user with username '%s' not found", req.TaggedUserTagPay)
		}
		return nil, status.Errorf(codes.Internal, "error finding user: %v", err)
	}

	if taggedUser.Username == nil || *taggedUser.Username == "" {
		return nil, status.Errorf(codes.FailedPrecondition, "tagged user does not have a username")
	}

	// Validate amount
	if req.Amount <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be greater than 0")
	}

	// Prevent tagging yourself
	if taggerUser.UUID == taggedUser.UUID {
		return nil, status.Errorf(codes.InvalidArgument, "you cannot tag yourself")
	}

	// Create the tag
	tag := &models.UserTag{
		TaggerID:         taggerUser.UUID,
		TaggerTagPay:     *taggerUser.Username,
		TaggerName:       taggerUser.FirstName + " " + taggerUser.LastName,
		TaggedUserID:     taggedUser.UUID,
		TaggedUserTagPay: *taggedUser.Username,
		TaggedUserName:   taggedUser.FirstName + " " + taggedUser.LastName,
		Amount:           req.Amount,
		Currency:         req.Currency,
		Description:      req.Description,
		Status:           models.TagStatusPending,
	}

	if err := s.db.Create(tag).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create tag: %v", err)
	}

	// Send notification to tagged user
	notificationService := NewNotificationService(s.db)
	if err := notificationService.SendTagNotification(ctx, tag); err != nil {
		log.Printf("[CreateTag] Failed to send tag notification: %v", err)
		// Don't fail the request if notification fails
	}

	return &pb.CreateTagResponse{
		Success: true,
		Message: fmt.Sprintf("Tagged %s with %s %.2f", taggedUser.FirstName, req.Currency, req.Amount),
		Tag:     userTagToProto(tag),
	}, nil
}

// GetMyOutgoingTags retrieves tags I created (I'm the tagger, I owe them money)
func (s *TagPayService) GetMyOutgoingTags(ctx context.Context, req *pb.GetMyTagsRequest) (*pb.GetMyTagsResponse, error) {
	// Get authenticated user ID from context
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Default pagination
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Build query - Get tags where current user is the TAGGER (creator)
	query := s.db.Where("tagger_id = ?", userID)

	// Filter by status ONLY if explicitly requested
	// Proto enum defaults to 0 (TAG_STATUS_PENDING), but we want to return ALL tags by default
	// Only filter if page > 1 (pagination) OR status is explicitly non-default
	// This allows first page to show all tags, while still supporting status filtering
	if page > 1 || req.Status != pb.TagStatus_TAG_STATUS_PENDING {
		var statusStr string
		switch req.Status {
		case pb.TagStatus_TAG_STATUS_PAID:
			statusStr = string(models.TagStatusPaid)
		case pb.TagStatus_TAG_STATUS_CANCELLED:
			statusStr = string(models.TagStatusCancelled)
		case pb.TagStatus_TAG_STATUS_PENDING:
			statusStr = string(models.TagStatusPending)
		}

		// Only apply status filter for subsequent pages or explicit status requests
		if page > 1 {
			query = query.Where("status = ?", statusStr)
		} else if req.Status != pb.TagStatus_TAG_STATUS_PENDING {
			query = query.Where("status = ?", statusStr)
		}
	}
	// No else block - returns ALL tags for first page with default status

	// Get total count
	var total int64
	if err := query.Model(&models.UserTag{}).Count(&total).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count tags: %v", err)
	}

	// Get tags
	var tags []models.UserTag
	if err := query.Order("created_at DESC").Limit(int(limit)).Offset(int(offset)).Find(&tags).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve tags: %v", err)
	}

	// Convert to proto
	protoTags := make([]*pb.UserTag, len(tags))
	for i, tag := range tags {
		protoTags[i] = userTagToProto(&tag)
	}

	totalPages := int32((total + int64(limit) - 1) / int64(limit))

	return &pb.GetMyTagsResponse{
		Tags:       protoTags,
		Total:      int32(total),
		Page:       page,
		TotalPages: totalPages,
	}, nil
}

// GetMyIncomingTags retrieves tags others created for me (they owe me money)
func (s *TagPayService) GetMyIncomingTags(ctx context.Context, req *pb.GetMyTagsRequest) (*pb.GetMyTagsResponse, error) {
	// Get authenticated user ID from context
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Default pagination
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Build query - Get tags where current user is the TAGGED USER (they owe me)
	query := s.db.Where("tagged_user_id = ?", userID)

	// Filter by status if requested
	if req.Status != pb.TagStatus_TAG_STATUS_PENDING || page > 1 {
		var statusStr string
		switch req.Status {
		case pb.TagStatus_TAG_STATUS_PAID:
			statusStr = string(models.TagStatusPaid)
		case pb.TagStatus_TAG_STATUS_CANCELLED:
			statusStr = string(models.TagStatusCancelled)
		case pb.TagStatus_TAG_STATUS_PENDING:
			statusStr = string(models.TagStatusPending)
		}

		if page > 1 || req.Status != pb.TagStatus_TAG_STATUS_PENDING {
			query = query.Where("status = ?", statusStr)
		}
	}

	// Get total count
	var total int64
	if err := query.Model(&models.UserTag{}).Count(&total).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count tags: %v", err)
	}

	// Get tags
	var tags []models.UserTag
	if err := query.Order("created_at DESC").Limit(int(limit)).Offset(int(offset)).Find(&tags).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve tags: %v", err)
	}

	// Convert to proto
	protoTags := make([]*pb.UserTag, len(tags))
	for i, tag := range tags {
		protoTags[i] = userTagToProto(&tag)
	}

	totalPages := int32((total + int64(limit) - 1) / int64(limit))

	return &pb.GetMyTagsResponse{
		Tags:       protoTags,
		Total:      int32(total),
		Page:       page,
		TotalPages: totalPages,
	}, nil
}

// GetMyTags retrieves all tags for the authenticated user (backward compatibility)
func (s *TagPayService) GetMyTags(ctx context.Context, req *pb.GetMyTagsRequest) (*pb.GetMyTagsResponse, error) {
	// Default to outgoing tags for backward compatibility
	return s.GetMyOutgoingTags(ctx, req)
}

// PayTag pays a specific tag
func (s *TagPayService) PayTag(ctx context.Context, req *pb.PayTagRequest) (*pb.PayTagResponse, error) {
	// Get authenticated user ID from context
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Get the tag
	var tag models.UserTag
	if err := s.db.Where("id = ?", req.TagId).First(&tag).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "tag not found")
		}
		return nil, status.Errorf(codes.Internal, "error finding tag: %v", err)
	}

	// Verify the authenticated user is the tagged user (person who needs to pay)
	if tag.TaggedUserID != userID {
		return nil, status.Errorf(codes.PermissionDenied, "you can only pay tags that were created for you")
	}

	// Check if tag is already paid
	if tag.Status != models.TagStatusPending {
		return nil, status.Errorf(codes.FailedPrecondition, "tag is not pending (status: %s)", tag.Status)
	}

	// No transaction PIN required for quick pay
	// TODO: Verify account balance (skipping for now as per original implementation pattern)
	// TODO: Perform actual transfer (skipping for now as per original implementation pattern)

	// Create transaction record
	now := time.Now()
	transaction := &models.TagPayTransaction{
		SenderID:       tag.TaggedUserID, // The tagged user is paying
		SenderTagPay:   tag.TaggedUserTagPay,
		SenderName:     tag.TaggedUserName,
		ReceiverID:     tag.TaggerID, // The tagger (creator) receives the payment
		ReceiverTagPay: tag.TaggerTagPay,
		ReceiverName:   tag.TaggerName,
		Amount:         tag.Amount,
		Currency:       tag.Currency,
		Description:    fmt.Sprintf("Payment for tag: %s", tag.Description),
		Status:         models.TagPayTransactionStatusCompleted,
		Type:           models.TagPayTransactionTypeSend,
		AccountID:      nil, // NULL in database (not needed for tag pay)
		CompletedAt:    &now,
	}

	// Start transaction
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create transaction
	if err := tx.Create(transaction).Error; err != nil {
		tx.Rollback()
		return nil, status.Errorf(codes.Internal, "failed to create transaction: %v", err)
	}

	// Update tag status
	tag.Status = models.TagStatusPaid
	tag.PaidAt = &now
	if err := tx.Save(&tag).Error; err != nil {
		tx.Rollback()
		return nil, status.Errorf(codes.Internal, "failed to update tag status: %v", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to commit transaction: %v", err)
	}

	return &pb.PayTagResponse{
		Success:     true,
		Message:     fmt.Sprintf("Successfully paid %s %.2f to %s", tag.Currency, tag.Amount, tag.TaggerName),
		Transaction: tagPayTransactionToProto(transaction),
	}, nil
}

func userTagToProto(tag *models.UserTag) *pb.UserTag {
	protoTag := &pb.UserTag{
		Id:               tag.ID,
		TaggerId:         tag.TaggerID,
		TaggerTagPay:     tag.TaggerTagPay,
		TaggerName:       tag.TaggerName,
		TaggedUserId:     tag.TaggedUserID,
		TaggedUserTagPay: tag.TaggedUserTagPay,
		TaggedUserName:   tag.TaggedUserName,
		Amount:           tag.Amount,
		Currency:         tag.Currency,
		Description:      tag.Description,
		Status:           tagStatusToProto(tag.Status),
		CreatedAt:        timestamppb.New(tag.CreatedAt),
	}

	if tag.PaidAt != nil {
		protoTag.PaidAt = timestamppb.New(*tag.PaidAt)
	}

	return protoTag
}

// SearchUsers searches for users by username or name for tagging
func (s *TagPayService) SearchUsers(ctx context.Context, req *pb.SearchUsersForTagRequest) (*pb.SearchUsersForTagResponse, error) {
	// Get authenticated user ID from context (to exclude self from results)
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		return &pb.SearchUsersForTagResponse{Users: []*pb.UserSearchResult{}}, nil
	}

	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	var users []models.User
	// Search by username, first name, or last name
	// Only return non-partial users (fully registered) and exclude self
	if err := s.db.Where("is_partial = ? AND user_id != ? AND (username ILIKE ? OR first_name ILIKE ? OR last_name ILIKE ?)",
		false, userID, "%"+query+"%", "%"+query+"%", "%"+query+"%").
		Limit(int(limit)).
		Find(&users).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search users: %v", err)
	}

	// Convert to proto
	results := make([]*pb.UserSearchResult, len(users))
	for i, user := range users {
		username := ""
		if user.Username != nil {
			username = *user.Username
		}

		// For now, profile picture is empty - will be added when user profile system is enhanced
		profilePicture := ""

		results[i] = &pb.UserSearchResult{
			UserId:         user.UUID,
			Username:       username,
			FirstName:      user.FirstName,
			LastName:       user.LastName,
			Email:          user.Email,
			ProfilePicture: profilePicture,
		}
	}

	return &pb.SearchUsersForTagResponse{Users: results}, nil
}

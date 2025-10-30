package services

import (
	"context"
	"errors"
	"lazervaultGo/database"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/pb"
	"lazervaultGo/token"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

var (
	ErrInvalidTaggedInvoiceData = errors.New("invalid tagged invoice data")
	ErrTaggedInvoiceNotFound    = errors.New("tagged invoice not found")
)

type ITaggedInvoiceService interface {
	// Core Tagged Invoice Operations
	GetTaggedInvoices(ctx context.Context, req *pb.GetTaggedInvoicesRequest) (*pb.GetTaggedInvoicesResponse, error)
	GetTaggedInvoicesByStatus(ctx context.Context, req *pb.GetTaggedInvoicesByStatusRequest) (*pb.GetTaggedInvoicesByStatusResponse, error)
	GetTaggedInvoiceById(ctx context.Context, req *pb.GetTaggedInvoiceByIdRequest) (*pb.GetTaggedInvoiceByIdResponse, error)
	GetOverdueTaggedInvoices(ctx context.Context, req *pb.GetOverdueTaggedInvoicesRequest) (*pb.GetOverdueTaggedInvoicesResponse, error)
	GetUpcomingTaggedInvoices(ctx context.Context, req *pb.GetUpcomingTaggedInvoicesRequest) (*pb.GetUpcomingTaggedInvoicesResponse, error)

	// Search and Filtering
	SearchTaggedInvoices(ctx context.Context, req *pb.SearchTaggedInvoicesRequest) (*pb.SearchTaggedInvoicesResponse, error)
	FilterTaggedInvoicesByPriority(ctx context.Context, req *pb.FilterTaggedInvoicesByPriorityRequest) (*pb.FilterTaggedInvoicesByPriorityResponse, error)
	FilterTaggedInvoicesByDateRange(ctx context.Context, req *pb.FilterTaggedInvoicesByDateRangeRequest) (*pb.FilterTaggedInvoicesByDateRangeResponse, error)
	FilterTaggedInvoicesByAmount(ctx context.Context, req *pb.FilterTaggedInvoicesByAmountRequest) (*pb.FilterTaggedInvoicesByAmountResponse, error)

	// Tagged Invoice Actions
	MarkTaggedInvoiceAsViewed(ctx context.Context, req *pb.MarkTaggedInvoiceAsViewedRequest) (*pb.MarkTaggedInvoiceAsViewedResponse, error)
	SetInvoicePaymentReminder(ctx context.Context, req *pb.SetInvoicePaymentReminderRequest) (*pb.SetInvoicePaymentReminderResponse, error)
	RequestTaggedInvoiceDetails(ctx context.Context, req *pb.RequestTaggedInvoiceDetailsRequest) (*pb.RequestTaggedInvoiceDetailsResponse, error)
	GetInvoicePaymentNotifications(ctx context.Context, req *pb.GetInvoicePaymentNotificationsRequest) (*pb.GetInvoicePaymentNotificationsResponse, error)
	UpdateTaggedInvoiceStatus(ctx context.Context, req *pb.UpdateTaggedInvoiceStatusRequest) (*pb.UpdateTaggedInvoiceStatusResponse, error)
	DeleteTaggedInvoice(ctx context.Context, req *pb.DeleteTaggedInvoiceRequest) (*pb.DeleteTaggedInvoiceResponse, error)

	// Bulk Operations
	MarkMultipleInvoicesAsViewed(ctx context.Context, req *pb.MarkMultipleInvoicesAsViewedRequest) (*pb.MarkMultipleInvoicesAsViewedResponse, error)
	BulkSetPaymentReminders(ctx context.Context, req *pb.BulkSetPaymentRemindersRequest) (*pb.BulkSetPaymentRemindersResponse, error)

	// Statistics
	GetTaggedInvoiceStatistics(ctx context.Context, req *pb.GetTaggedInvoiceStatisticsRequest) (*pb.GetTaggedInvoiceStatisticsResponse, error)
}

type TaggedInvoiceService struct {
	db *gorm.DB
}

func NewTaggedInvoiceService(db *gorm.DB) ITaggedInvoiceService {
	return &TaggedInvoiceService{db: db}
}

// getUserIDFromContext extracts the user ID from the JWT token via email database lookup
func (s *TaggedInvoiceService) getUserIDFromContext(ctx context.Context) (string, error) {
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

// GetTaggedInvoices retrieves tagged invoices for a user
func (s *TaggedInvoiceService) GetTaggedInvoices(ctx context.Context, req *pb.GetTaggedInvoicesRequest) (*pb.GetTaggedInvoicesResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual tagged invoice retrieval from database
	return &pb.GetTaggedInvoicesResponse{
		Invoices:   []*pb.TaggedInvoice{},
		TotalCount: 0,
		Summary: &pb.TaggedInvoicesSummary{
			TotalInvoices:   0,
			PendingInvoices: 0,
			OverdueInvoices: 0,
			PaidInvoices:    0,
		},
	}, nil
}

// GetTaggedInvoicesByStatus retrieves tagged invoices by status
func (s *TaggedInvoiceService) GetTaggedInvoicesByStatus(ctx context.Context, req *pb.GetTaggedInvoicesByStatusRequest) (*pb.GetTaggedInvoicesByStatusResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual tagged invoice retrieval by status
	return &pb.GetTaggedInvoicesByStatusResponse{
		Invoices:   []*pb.TaggedInvoice{},
		TotalCount: 0,
	}, nil
}

// GetTaggedInvoiceById retrieves a specific tagged invoice
func (s *TaggedInvoiceService) GetTaggedInvoiceById(ctx context.Context, req *pb.GetTaggedInvoiceByIdRequest) (*pb.GetTaggedInvoiceByIdResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual tagged invoice retrieval by ID
	return &pb.GetTaggedInvoiceByIdResponse{
		Invoice: &pb.TaggedInvoice{},
	}, nil
}

// GetOverdueTaggedInvoices retrieves overdue tagged invoices
func (s *TaggedInvoiceService) GetOverdueTaggedInvoices(ctx context.Context, req *pb.GetOverdueTaggedInvoicesRequest) (*pb.GetOverdueTaggedInvoicesResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual overdue tagged invoice retrieval
	return &pb.GetOverdueTaggedInvoicesResponse{
		Invoices:           []*pb.TaggedInvoice{},
		TotalCount:         0,
		TotalOverdueAmount: 0.0,
	}, nil
}

// GetUpcomingTaggedInvoices retrieves upcoming tagged invoices
func (s *TaggedInvoiceService) GetUpcomingTaggedInvoices(ctx context.Context, req *pb.GetUpcomingTaggedInvoicesRequest) (*pb.GetUpcomingTaggedInvoicesResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual upcoming tagged invoice retrieval
	return &pb.GetUpcomingTaggedInvoicesResponse{
		Invoices:   []*pb.TaggedInvoice{},
		TotalCount: 0,
	}, nil
}

// SearchTaggedInvoices searches for tagged invoices
func (s *TaggedInvoiceService) SearchTaggedInvoices(ctx context.Context, req *pb.SearchTaggedInvoicesRequest) (*pb.SearchTaggedInvoicesResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement actual search functionality
	return &pb.SearchTaggedInvoicesResponse{
		Invoices:   []*pb.TaggedInvoice{},
		TotalCount: 0,
	}, nil
}

// FilterTaggedInvoicesByPriority filters tagged invoices by priority
func (s *TaggedInvoiceService) FilterTaggedInvoicesByPriority(ctx context.Context, req *pb.FilterTaggedInvoicesByPriorityRequest) (*pb.FilterTaggedInvoicesByPriorityResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement priority filtering
	return &pb.FilterTaggedInvoicesByPriorityResponse{
		Invoices:   []*pb.TaggedInvoice{},
		TotalCount: 0,
	}, nil
}

// FilterTaggedInvoicesByDateRange filters tagged invoices by date range
func (s *TaggedInvoiceService) FilterTaggedInvoicesByDateRange(ctx context.Context, req *pb.FilterTaggedInvoicesByDateRangeRequest) (*pb.FilterTaggedInvoicesByDateRangeResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement date range filtering
	return &pb.FilterTaggedInvoicesByDateRangeResponse{
		Invoices:   []*pb.TaggedInvoice{},
		TotalCount: 0,
	}, nil
}

// FilterTaggedInvoicesByAmount filters tagged invoices by amount range
func (s *TaggedInvoiceService) FilterTaggedInvoicesByAmount(ctx context.Context, req *pb.FilterTaggedInvoicesByAmountRequest) (*pb.FilterTaggedInvoicesByAmountResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement amount filtering
	return &pb.FilterTaggedInvoicesByAmountResponse{
		Invoices:   []*pb.TaggedInvoice{},
		TotalCount: 0,
	}, nil
}

// MarkTaggedInvoiceAsViewed marks a tagged invoice as viewed
func (s *TaggedInvoiceService) MarkTaggedInvoiceAsViewed(ctx context.Context, req *pb.MarkTaggedInvoiceAsViewedRequest) (*pb.MarkTaggedInvoiceAsViewedResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement mark as viewed functionality
	return &pb.MarkTaggedInvoiceAsViewedResponse{
		Invoice: &pb.TaggedInvoice{},
		Success: true,
		Message: "Invoice marked as viewed successfully",
	}, nil
}

// SetInvoicePaymentReminder sets a payment reminder for a tagged invoice
func (s *TaggedInvoiceService) SetInvoicePaymentReminder(ctx context.Context, req *pb.SetInvoicePaymentReminderRequest) (*pb.SetInvoicePaymentReminderResponse, error) {
	// Extract user ID from JWT token via email database lookup
	userID, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement payment reminder functionality
	return &pb.SetInvoicePaymentReminderResponse{
		Reminder: &pb.PaymentReminder{
			InvoiceId:    req.InvoiceId,
			UserId:       userID,
			ReminderDate: req.ReminderDate,
			Status:       "pending",
		},
		Success: true,
		Message: "Payment reminder set successfully",
	}, nil
}

// RequestTaggedInvoiceDetails requests additional details for a tagged invoice
func (s *TaggedInvoiceService) RequestTaggedInvoiceDetails(ctx context.Context, req *pb.RequestTaggedInvoiceDetailsRequest) (*pb.RequestTaggedInvoiceDetailsResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement invoice details request functionality
	return &pb.RequestTaggedInvoiceDetailsResponse{
		Success:   true,
		Message:   "Invoice details requested successfully",
		RequestId: uuid.New().String(),
	}, nil
}

// GetInvoicePaymentNotifications retrieves payment notifications for a user
func (s *TaggedInvoiceService) GetInvoicePaymentNotifications(ctx context.Context, req *pb.GetInvoicePaymentNotificationsRequest) (*pb.GetInvoicePaymentNotificationsResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement notification retrieval
	return &pb.GetInvoicePaymentNotificationsResponse{
		Notifications: []*pb.InvoicePaymentNotification{},
		TotalCount:    0,
		UnreadCount:   0,
	}, nil
}

// UpdateTaggedInvoiceStatus updates the status of a tagged invoice
func (s *TaggedInvoiceService) UpdateTaggedInvoiceStatus(ctx context.Context, req *pb.UpdateTaggedInvoiceStatusRequest) (*pb.UpdateTaggedInvoiceStatusResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement status update functionality
	return &pb.UpdateTaggedInvoiceStatusResponse{
		Invoice: &pb.TaggedInvoice{},
		Success: true,
		Message: "Invoice status updated successfully",
	}, nil
}

// DeleteTaggedInvoice deletes a tagged invoice
func (s *TaggedInvoiceService) DeleteTaggedInvoice(ctx context.Context, req *pb.DeleteTaggedInvoiceRequest) (*pb.DeleteTaggedInvoiceResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement delete functionality
	return &pb.DeleteTaggedInvoiceResponse{
		Success: true,
		Message: "Tagged invoice deleted successfully",
	}, nil
}

// MarkMultipleInvoicesAsViewed marks multiple invoices as viewed
func (s *TaggedInvoiceService) MarkMultipleInvoicesAsViewed(ctx context.Context, req *pb.MarkMultipleInvoicesAsViewedRequest) (*pb.MarkMultipleInvoicesAsViewedResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement bulk mark as viewed functionality
	return &pb.MarkMultipleInvoicesAsViewedResponse{
		Success:      true,
		Message:      "Multiple invoices marked as viewed successfully",
		UpdatedCount: uint64(len(req.InvoiceIds)),
	}, nil
}

// BulkSetPaymentReminders sets payment reminders for multiple invoices
func (s *TaggedInvoiceService) BulkSetPaymentReminders(ctx context.Context, req *pb.BulkSetPaymentRemindersRequest) (*pb.BulkSetPaymentRemindersResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement bulk reminder functionality
	return &pb.BulkSetPaymentRemindersResponse{
		Success:      true,
		Message:      "Payment reminders set successfully",
		UpdatedCount: uint64(len(req.InvoiceIds)),
	}, nil
}

// GetTaggedInvoiceStatistics retrieves statistics for tagged invoices
func (s *TaggedInvoiceService) GetTaggedInvoiceStatistics(ctx context.Context, req *pb.GetTaggedInvoiceStatisticsRequest) (*pb.GetTaggedInvoiceStatisticsResponse, error) {
	// Extract user ID from JWT token via email database lookup
	_, err := s.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement statistics calculation
	return &pb.GetTaggedInvoiceStatisticsResponse{
		Statistics: &pb.TaggedInvoiceStatistics{
			TotalInvoices:     0,
			PendingInvoices:   0,
			OverdueInvoices:   0,
			CompletedInvoices: 0,
			TotalAmount:       0.0,
			PendingAmount:     0.0,
			OverdueAmount:     0.0,
			CompletedAmount:   0.0,
			AverageAmount:     0.0,
		},
	}, nil
}

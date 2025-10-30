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

// TaggedInvoiceController handles gRPC requests for tagged invoice operations
type TaggedInvoiceController struct {
	pb.UnimplementedTaggedInvoiceServiceServer
	taggedInvoiceService services.ITaggedInvoiceService
	userService          services.IUserService
	db                   *gorm.DB
}

// NewTaggedInvoiceController creates a new TaggedInvoiceController
func NewTaggedInvoiceController(taggedInvoiceService services.ITaggedInvoiceService, userService services.IUserService, db *gorm.DB) *TaggedInvoiceController {
	return &TaggedInvoiceController{
		taggedInvoiceService: taggedInvoiceService,
		userService:          userService,
		db:                   db,
	}
}

// getUserFromAuthPayload extracts user information from the authentication payload
func (c *TaggedInvoiceController) getUserFromAuthPayload(ctx context.Context) (string, error) {
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

// Core Tagged Invoice Operations

// GetTaggedInvoices retrieves tagged invoices for a user
func (c *TaggedInvoiceController) GetTaggedInvoices(ctx context.Context, req *pb.GetTaggedInvoicesRequest) (*pb.GetTaggedInvoicesResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.GetTaggedInvoices(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get tagged invoices: %v", err)
	}

	return response, nil
}

// GetTaggedInvoicesByStatus retrieves tagged invoices by status
func (c *TaggedInvoiceController) GetTaggedInvoicesByStatus(ctx context.Context, req *pb.GetTaggedInvoicesByStatusRequest) (*pb.GetTaggedInvoicesByStatusResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.GetTaggedInvoicesByStatus(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get tagged invoices by status: %v", err)
	}

	return response, nil
}

// GetTaggedInvoiceById retrieves a specific tagged invoice
func (c *TaggedInvoiceController) GetTaggedInvoiceById(ctx context.Context, req *pb.GetTaggedInvoiceByIdRequest) (*pb.GetTaggedInvoiceByIdResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.GetTaggedInvoiceById(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get tagged invoice by id: %v", err)
	}

	return response, nil
}

// GetOverdueTaggedInvoices retrieves overdue tagged invoices
func (c *TaggedInvoiceController) GetOverdueTaggedInvoices(ctx context.Context, req *pb.GetOverdueTaggedInvoicesRequest) (*pb.GetOverdueTaggedInvoicesResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.GetOverdueTaggedInvoices(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get overdue tagged invoices: %v", err)
	}

	return response, nil
}

// GetUpcomingTaggedInvoices retrieves upcoming tagged invoices
func (c *TaggedInvoiceController) GetUpcomingTaggedInvoices(ctx context.Context, req *pb.GetUpcomingTaggedInvoicesRequest) (*pb.GetUpcomingTaggedInvoicesResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.GetUpcomingTaggedInvoices(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get upcoming tagged invoices: %v", err)
	}

	return response, nil
}

// Search and Filtering Operations

// SearchTaggedInvoices searches for tagged invoices
func (c *TaggedInvoiceController) SearchTaggedInvoices(ctx context.Context, req *pb.SearchTaggedInvoicesRequest) (*pb.SearchTaggedInvoicesResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.SearchTaggedInvoices(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search tagged invoices: %v", err)
	}

	return response, nil
}

// FilterTaggedInvoicesByPriority filters tagged invoices by priority
func (c *TaggedInvoiceController) FilterTaggedInvoicesByPriority(ctx context.Context, req *pb.FilterTaggedInvoicesByPriorityRequest) (*pb.FilterTaggedInvoicesByPriorityResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.FilterTaggedInvoicesByPriority(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to filter tagged invoices by priority: %v", err)
	}

	return response, nil
}

// FilterTaggedInvoicesByDateRange filters tagged invoices by date range
func (c *TaggedInvoiceController) FilterTaggedInvoicesByDateRange(ctx context.Context, req *pb.FilterTaggedInvoicesByDateRangeRequest) (*pb.FilterTaggedInvoicesByDateRangeResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.FilterTaggedInvoicesByDateRange(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to filter tagged invoices by date range: %v", err)
	}

	return response, nil
}

// FilterTaggedInvoicesByAmount filters tagged invoices by amount
func (c *TaggedInvoiceController) FilterTaggedInvoicesByAmount(ctx context.Context, req *pb.FilterTaggedInvoicesByAmountRequest) (*pb.FilterTaggedInvoicesByAmountResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.FilterTaggedInvoicesByAmount(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to filter tagged invoices by amount: %v", err)
	}

	return response, nil
}

// Tagged Invoice Actions

// MarkTaggedInvoiceAsViewed marks a tagged invoice as viewed
func (c *TaggedInvoiceController) MarkTaggedInvoiceAsViewed(ctx context.Context, req *pb.MarkTaggedInvoiceAsViewedRequest) (*pb.MarkTaggedInvoiceAsViewedResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.MarkTaggedInvoiceAsViewed(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mark tagged invoice as viewed: %v", err)
	}

	return response, nil
}

// SetInvoicePaymentReminder sets a payment reminder for a tagged invoice
func (c *TaggedInvoiceController) SetInvoicePaymentReminder(ctx context.Context, req *pb.SetInvoicePaymentReminderRequest) (*pb.SetInvoicePaymentReminderResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.SetInvoicePaymentReminder(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to set payment reminder: %v", err)
	}

	return response, nil
}

// RequestTaggedInvoiceDetails requests additional details for a tagged invoice
func (c *TaggedInvoiceController) RequestTaggedInvoiceDetails(ctx context.Context, req *pb.RequestTaggedInvoiceDetailsRequest) (*pb.RequestTaggedInvoiceDetailsResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.RequestTaggedInvoiceDetails(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to request tagged invoice details: %v", err)
	}

	return response, nil
}

// GetInvoicePaymentNotifications retrieves payment notifications for a user
func (c *TaggedInvoiceController) GetInvoicePaymentNotifications(ctx context.Context, req *pb.GetInvoicePaymentNotificationsRequest) (*pb.GetInvoicePaymentNotificationsResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.GetInvoicePaymentNotifications(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get payment notifications: %v", err)
	}

	return response, nil
}

// UpdateTaggedInvoiceStatus updates the status of a tagged invoice
func (c *TaggedInvoiceController) UpdateTaggedInvoiceStatus(ctx context.Context, req *pb.UpdateTaggedInvoiceStatusRequest) (*pb.UpdateTaggedInvoiceStatusResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.UpdateTaggedInvoiceStatus(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update tagged invoice status: %v", err)
	}

	return response, nil
}

// DeleteTaggedInvoice deletes a tagged invoice
func (c *TaggedInvoiceController) DeleteTaggedInvoice(ctx context.Context, req *pb.DeleteTaggedInvoiceRequest) (*pb.DeleteTaggedInvoiceResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.DeleteTaggedInvoice(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete tagged invoice: %v", err)
	}

	return response, nil
}

// Bulk Operations

// MarkMultipleInvoicesAsViewed marks multiple invoices as viewed
func (c *TaggedInvoiceController) MarkMultipleInvoicesAsViewed(ctx context.Context, req *pb.MarkMultipleInvoicesAsViewedRequest) (*pb.MarkMultipleInvoicesAsViewedResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.MarkMultipleInvoicesAsViewed(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mark multiple invoices as viewed: %v", err)
	}

	return response, nil
}

// BulkSetPaymentReminders sets payment reminders for multiple invoices
func (c *TaggedInvoiceController) BulkSetPaymentReminders(ctx context.Context, req *pb.BulkSetPaymentRemindersRequest) (*pb.BulkSetPaymentRemindersResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.BulkSetPaymentReminders(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to bulk set payment reminders: %v", err)
	}

	return response, nil
}

// Statistics

// GetTaggedInvoiceStatistics retrieves statistics for tagged invoices
func (c *TaggedInvoiceController) GetTaggedInvoiceStatistics(ctx context.Context, req *pb.GetTaggedInvoiceStatisticsRequest) (*pb.GetTaggedInvoiceStatisticsResponse, error) {
	// Get user ID from auth payload (validates authentication)
	_, err := c.getUserFromAuthPayload(ctx)
	if err != nil {
		return nil, err
	}

	// Call service
	response, err := c.taggedInvoiceService.GetTaggedInvoiceStatistics(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get tagged invoice statistics: %v", err)
	}

	return response, nil
}

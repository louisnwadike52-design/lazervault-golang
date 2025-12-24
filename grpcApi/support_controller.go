package grpcApi

import (
	"context"
	"fmt"
	"time"

	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// SupportController handles support ticket operations
type SupportController struct {
	pb.UnimplementedSupportServiceServer
	db          *gorm.DB
	userService services.IUserService
}

// NewSupportController creates a new support controller
func NewSupportController(db *gorm.DB, userService services.IUserService) *SupportController {
	return &SupportController{
		db:          db,
		userService: userService,
	}
}

// CreateSupportTicket creates a new support ticket
func (sc *SupportController) CreateSupportTicket(ctx context.Context, req *pb.CreateSupportTicketRequest) (*pb.CreateSupportTicketResponse, error) {
	// Get user from context
	user, err := getUserFromContext(ctx, sc.userService)
	if err != nil {
		return &pb.CreateSupportTicketResponse{
			Success: false,
			Message: "Unauthorized",
		}, err
	}
	userID := int(user.ID)

	// Generate unique ticket number
	ticketNumber := fmt.Sprintf("TKT-%d-%d", time.Now().Unix(), userID)

	// Map proto category to model category
	category := mapProtoToModelCategory(req.Category)

	// Create ticket
	ticket := &models.SupportTicket{
		UserID:       userID,
		TicketNumber: ticketNumber,
		Category:     category,
		Subject:      req.Subject,
		Description:  req.Description,
		Status:       models.TicketStatusOpen,
		Priority:     models.TicketPriorityMedium,
	}

	if err := sc.db.Create(ticket).Error; err != nil {
		return &pb.CreateSupportTicketResponse{
			Success: false,
			Message: "Failed to create support ticket",
		}, status.Error(codes.Internal, "Failed to create support ticket")
	}

	// Load user relationship
	sc.db.Preload("User").First(ticket, ticket.ID)

	return &pb.CreateSupportTicketResponse{
		Success: true,
		Message: "Support ticket created successfully",
		Ticket:  mapModelToProtoTicket(ticket),
	}, nil
}

// GetSupportTickets retrieves user's support tickets
func (sc *SupportController) GetSupportTickets(ctx context.Context, req *pb.GetSupportTicketsRequest) (*pb.GetSupportTicketsResponse, error) {
	// Get user ID from context
	user, err := getUserFromContext(ctx, sc.userService)
	if err != nil {
		return &pb.GetSupportTicketsResponse{
			Success: false,
			Message: "Unauthorized",
		}, err
	}
	userID := int(user.ID)

	// Set default pagination
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Build query
	query := sc.db.Where("user_id = ?", userID)

	// Apply status filter if specified
	if req.StatusFilter != pb.TicketStatus_TICKET_STATUS_UNSPECIFIED {
		query = query.Where("status = ?", mapProtoToModelStatus(req.StatusFilter))
	}

	// Get total count
	var totalCount int64
	if err := query.Model(&models.SupportTicket{}).Count(&totalCount).Error; err != nil {
		return &pb.GetSupportTicketsResponse{
			Success: false,
			Message: "Failed to get tickets count",
		}, status.Error(codes.Internal, "Failed to get tickets count")
	}

	// Get tickets with pagination
	var tickets []models.SupportTicket
	offset := (page - 1) * pageSize
	if err := query.Preload("Replies").Order("created_at DESC").Limit(int(pageSize)).Offset(int(offset)).Find(&tickets).Error; err != nil {
		return &pb.GetSupportTicketsResponse{
			Success: false,
			Message: "Failed to retrieve support tickets",
		}, status.Error(codes.Internal, "Failed to retrieve support tickets")
	}

	// Map to proto tickets
	protoTickets := make([]*pb.SupportTicket, len(tickets))
	for i, ticket := range tickets {
		protoTickets[i] = mapModelToProtoTicket(&ticket)
	}

	return &pb.GetSupportTicketsResponse{
		Success:    true,
		Message:    "Support tickets retrieved successfully",
		Tickets:    protoTickets,
		TotalCount: int32(totalCount),
		Page:       page,
		PageSize:   pageSize,
	}, nil
}

// GetSupportTicket retrieves a specific support ticket
func (sc *SupportController) GetSupportTicket(ctx context.Context, req *pb.GetSupportTicketRequest) (*pb.GetSupportTicketResponse, error) {
	// Get user ID from context
	user, err := getUserFromContext(ctx, sc.userService)
	if err != nil {
		return &pb.GetSupportTicketResponse{
			Success: false,
			Message: "Unauthorized",
		}, err
	}
	userID := int(user.ID)

	// Find ticket
	var ticket models.SupportTicket
	if err := sc.db.Preload("Replies.User").Where("id = ? AND user_id = ?", req.TicketId, userID).First(&ticket).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.GetSupportTicketResponse{
				Success: false,
				Message: "Ticket not found",
			}, status.Error(codes.NotFound, "Ticket not found")
		}
		return &pb.GetSupportTicketResponse{
			Success: false,
			Message: "Failed to retrieve ticket",
		}, status.Error(codes.Internal, "Failed to retrieve ticket")
	}

	return &pb.GetSupportTicketResponse{
		Success: true,
		Message: "Ticket retrieved successfully",
		Ticket:  mapModelToProtoTicket(&ticket),
	}, nil
}

// UpdateTicketStatus updates the status of a support ticket
func (sc *SupportController) UpdateTicketStatus(ctx context.Context, req *pb.UpdateTicketStatusRequest) (*pb.UpdateTicketStatusResponse, error) {
	// Get user ID from context
	user, err := getUserFromContext(ctx, sc.userService)
	if err != nil {
		return &pb.UpdateTicketStatusResponse{
			Success: false,
			Message: "Unauthorized",
		}, err
	}
	userID := int(user.ID)

	// Find ticket
	var ticket models.SupportTicket
	if err := sc.db.Where("id = ? AND user_id = ?", req.TicketId, userID).First(&ticket).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.UpdateTicketStatusResponse{
				Success: false,
				Message: "Ticket not found",
			}, status.Error(codes.NotFound, "Ticket not found")
		}
		return &pb.UpdateTicketStatusResponse{
			Success: false,
			Message: "Failed to find ticket",
		}, status.Error(codes.Internal, "Failed to find ticket")
	}

	// Update status
	ticket.Status = mapProtoToModelStatus(req.Status)

	// Set resolved time if status is resolved or closed
	if ticket.Status == models.TicketStatusResolved || ticket.Status == models.TicketStatusClosed {
		now := time.Now()
		ticket.ResolvedAt = &now
	}

	if err := sc.db.Save(&ticket).Error; err != nil {
		return &pb.UpdateTicketStatusResponse{
			Success: false,
			Message: "Failed to update ticket status",
		}, status.Error(codes.Internal, "Failed to update ticket status")
	}

	// Reload with relationships
	sc.db.Preload("Replies").First(&ticket, ticket.ID)

	return &pb.UpdateTicketStatusResponse{
		Success: true,
		Message: "Ticket status updated successfully",
		Ticket:  mapModelToProtoTicket(&ticket),
	}, nil
}

// AddTicketReply adds a reply to a support ticket
func (sc *SupportController) AddTicketReply(ctx context.Context, req *pb.AddTicketReplyRequest) (*pb.AddTicketReplyResponse, error) {
	// Get user ID from context
	user, err := getUserFromContext(ctx, sc.userService)
	if err != nil {
		return &pb.AddTicketReplyResponse{
			Success: false,
			Message: "Unauthorized",
		}, err
	}
	userID := int(user.ID)

	// Find ticket
	var ticket models.SupportTicket
	if err := sc.db.Where("id = ? AND user_id = ?", req.TicketId, userID).First(&ticket).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.AddTicketReplyResponse{
				Success: false,
				Message: "Ticket not found",
			}, status.Error(codes.NotFound, "Ticket not found")
		}
		return &pb.AddTicketReplyResponse{
			Success: false,
			Message: "Failed to find ticket",
		}, status.Error(codes.Internal, "Failed to find ticket")
	}

	// Create reply
	reply := &models.TicketReply{
		TicketID: ticket.ID,
		UserID:   userID,
		Message:  req.Message,
		IsStaff:  false,
	}

	if err := sc.db.Create(reply).Error; err != nil {
		return &pb.AddTicketReplyResponse{
			Success: false,
			Message: "Failed to add reply",
		}, status.Error(codes.Internal, "Failed to add reply")
	}

	// Update ticket status to waiting for support if it was waiting for customer
	if ticket.Status == models.TicketStatusWaitingForCustomer {
		ticket.Status = models.TicketStatusInProgress
		sc.db.Save(&ticket)
	}

	// Load user relationship
	sc.db.Preload("User").First(reply, reply.ID)

	return &pb.AddTicketReplyResponse{
		Success: true,
		Message: "Reply added successfully",
		Reply:   mapModelToProtoReply(reply),
	}, nil
}

// SubmitContactForm submits a contact form message
func (sc *SupportController) SubmitContactForm(ctx context.Context, req *pb.SubmitContactFormRequest) (*pb.SubmitContactFormResponse, error) {
	// Get user ID from context (optional for contact form)
	user, _ := getUserFromContext(ctx, sc.userService)

	// Create contact message
	contactMessage := &models.ContactMessage{
		Name:    req.Name,
		Email:   req.Email,
		Topic:   req.Topic,
		Subject: req.Subject,
		Message: req.Message,
	}

	if user != nil {
		userID := int(user.ID)
		contactMessage.UserID = &userID
	}

	if err := sc.db.Create(contactMessage).Error; err != nil {
		return &pb.SubmitContactFormResponse{
			Success: false,
			Message: "Failed to submit contact form",
		}, status.Error(codes.Internal, "Failed to submit contact form")
	}

	return &pb.SubmitContactFormResponse{
		Success:        true,
		Message:        "Contact form submitted successfully. We'll get back to you soon!",
		ContactMessage: mapModelToProtoContactMessage(contactMessage),
	}, nil
}

// Helper functions to map between proto and model types
func mapProtoToModelCategory(category pb.TicketCategory) models.TicketCategory {
	switch category {
	case pb.TicketCategory_GENERAL_INQUIRY:
		return models.TicketCategoryGeneralInquiry
	case pb.TicketCategory_TRANSACTION_ISSUE:
		return models.TicketCategoryTransactionIssue
	case pb.TicketCategory_ACCOUNT_PROBLEM:
		return models.TicketCategoryAccountProblem
	case pb.TicketCategory_TECHNICAL_SUPPORT:
		return models.TicketCategoryTechnicalSupport
	case pb.TicketCategory_SECURITY_CONCERN:
		return models.TicketCategorySecurityConcern
	case pb.TicketCategory_OTHER:
		return models.TicketCategoryOther
	default:
		return models.TicketCategoryOther
	}
}

func mapModelToProtoCategory(category models.TicketCategory) pb.TicketCategory {
	switch category {
	case models.TicketCategoryGeneralInquiry:
		return pb.TicketCategory_GENERAL_INQUIRY
	case models.TicketCategoryTransactionIssue:
		return pb.TicketCategory_TRANSACTION_ISSUE
	case models.TicketCategoryAccountProblem:
		return pb.TicketCategory_ACCOUNT_PROBLEM
	case models.TicketCategoryTechnicalSupport:
		return pb.TicketCategory_TECHNICAL_SUPPORT
	case models.TicketCategorySecurityConcern:
		return pb.TicketCategory_SECURITY_CONCERN
	case models.TicketCategoryOther:
		return pb.TicketCategory_OTHER
	default:
		return pb.TicketCategory_OTHER
	}
}

func mapProtoToModelStatus(status pb.TicketStatus) models.TicketStatus {
	switch status {
	case pb.TicketStatus_OPEN:
		return models.TicketStatusOpen
	case pb.TicketStatus_IN_PROGRESS:
		return models.TicketStatusInProgress
	case pb.TicketStatus_WAITING_FOR_CUSTOMER:
		return models.TicketStatusWaitingForCustomer
	case pb.TicketStatus_RESOLVED:
		return models.TicketStatusResolved
	case pb.TicketStatus_CLOSED:
		return models.TicketStatusClosed
	default:
		return models.TicketStatusOpen
	}
}

func mapModelToProtoStatus(status models.TicketStatus) pb.TicketStatus {
	switch status {
	case models.TicketStatusOpen:
		return pb.TicketStatus_OPEN
	case models.TicketStatusInProgress:
		return pb.TicketStatus_IN_PROGRESS
	case models.TicketStatusWaitingForCustomer:
		return pb.TicketStatus_WAITING_FOR_CUSTOMER
	case models.TicketStatusResolved:
		return pb.TicketStatus_RESOLVED
	case models.TicketStatusClosed:
		return pb.TicketStatus_CLOSED
	default:
		return pb.TicketStatus_OPEN
	}
}

func mapProtoToModelPriority(priority pb.TicketPriority) models.TicketPriority {
	switch priority {
	case pb.TicketPriority_LOW:
		return models.TicketPriorityLow
	case pb.TicketPriority_MEDIUM:
		return models.TicketPriorityMedium
	case pb.TicketPriority_HIGH:
		return models.TicketPriorityHigh
	case pb.TicketPriority_URGENT:
		return models.TicketPriorityUrgent
	default:
		return models.TicketPriorityMedium
	}
}

func mapModelToProtoPriority(priority models.TicketPriority) pb.TicketPriority {
	switch priority {
	case models.TicketPriorityLow:
		return pb.TicketPriority_LOW
	case models.TicketPriorityMedium:
		return pb.TicketPriority_MEDIUM
	case models.TicketPriorityHigh:
		return pb.TicketPriority_HIGH
	case models.TicketPriorityUrgent:
		return pb.TicketPriority_URGENT
	default:
		return pb.TicketPriority_MEDIUM
	}
}

func mapModelToProtoTicket(ticket *models.SupportTicket) *pb.SupportTicket {
	protoTicket := &pb.SupportTicket{
		Id:           fmt.Sprintf("%d", ticket.ID),
		UserId:       int32(ticket.UserID),
		TicketNumber: ticket.TicketNumber,
		Category:     mapModelToProtoCategory(ticket.Category),
		Subject:      ticket.Subject,
		Description:  ticket.Description,
		Status:       mapModelToProtoStatus(ticket.Status),
		Priority:     mapModelToProtoPriority(ticket.Priority),
		CreatedAt:    timestamppb.New(ticket.CreatedAt),
		UpdatedAt:    timestamppb.New(ticket.UpdatedAt),
	}

	if ticket.ResolvedAt != nil {
		protoTicket.ResolvedAt = timestamppb.New(*ticket.ResolvedAt)
	}

	// Map replies
	if len(ticket.Replies) > 0 {
		protoTicket.Replies = make([]*pb.TicketReply, len(ticket.Replies))
		for i, reply := range ticket.Replies {
			protoTicket.Replies[i] = mapModelToProtoReply(&reply)
		}
	}

	return protoTicket
}

func mapModelToProtoReply(reply *models.TicketReply) *pb.TicketReply {
	return &pb.TicketReply{
		Id:        fmt.Sprintf("%d", reply.ID),
		TicketId:  fmt.Sprintf("%d", reply.TicketID),
		UserId:    int32(reply.UserID),
		Message:   reply.Message,
		IsStaff:   reply.IsStaff,
		CreatedAt: timestamppb.New(reply.CreatedAt),
	}
}

func mapModelToProtoContactMessage(msg *models.ContactMessage) *pb.ContactMessage {
	protoMsg := &pb.ContactMessage{
		Id:        fmt.Sprintf("%d", msg.ID),
		Name:      msg.Name,
		Email:     msg.Email,
		Topic:     msg.Topic,
		Subject:   msg.Subject,
		Message:   msg.Message,
		CreatedAt: timestamppb.New(msg.CreatedAt),
		IsRead:    msg.IsRead,
	}

	if msg.UserID != nil {
		protoMsg.UserId = int32(*msg.UserID)
	}

	return protoMsg
}

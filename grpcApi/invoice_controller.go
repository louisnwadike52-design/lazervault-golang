package grpcApi

import (
	"context"
	"encoding/json"
	"errors"
	"lazervaultGo/grpcApi/middleware"
	"lazervaultGo/models"
	"lazervaultGo/pb"
	"lazervaultGo/services"
	"lazervaultGo/token"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InvoiceController handles gRPC requests for invoice operations
type InvoiceController struct {
	pb.UnimplementedInvoiceServiceServer // Embed for forward compatibility
	invoiceService                       services.IInvoiceService
	userService                          services.IUserService // Needed to get UserID
}

// NewInvoiceController creates a new InvoiceController
func NewInvoiceController(invoiceService services.IInvoiceService, userService services.IUserService) *InvoiceController {
	return &InvoiceController{
		invoiceService: invoiceService,
		userService:    userService,
	}
}

// --- Helper: Convert model.Invoice to pb.Invoice ---
func convertInvoiceToProto(inv *models.Invoice) (*pb.Invoice, error) {
	if inv == nil {
		return nil, nil
	}

	var paymentMethodId string
	if inv.PaymentMethodID != nil {
		paymentMethodId = *inv.PaymentMethodID
	}

	pbInvoice := &pb.Invoice{
		Id:               inv.ID,
		UserId:           inv.UserID,
		RecipientId:      inv.RecipientID,
		Title:            inv.Title,
		Description:      inv.Description,
		Amount:           inv.Amount,
		Currency:         inv.Currency,
		DueDate:          inv.DueDate.Format(time.RFC3339),
		IsPaid:           inv.IsPaid,
		PaymentMethodId:  paymentMethodId,
		PaymentReference: inv.PaymentReference,
		CreatedAt:        timestamppb.New(inv.CreatedAt),
		UpdatedAt:        timestamppb.New(inv.UpdatedAt),
		Notes:            inv.Notes,
		TaxAmount:        inv.TaxAmount,
		DiscountAmount:   inv.DiscountAmount,
		TotalAmount:      inv.TotalAmount,
		ToEmail:          inv.ToEmail,
		ToName:           inv.ToName,
	}

	// Deserialize items from JSON
	if len(inv.Items) > 0 {
		var items []models.InvoiceItem
		if err := json.Unmarshal(inv.Items, &items); err == nil {
			for _, item := range items {
				pbInvoice.Items = append(pbInvoice.Items, &pb.InvoiceItem{
					Id:          item.ItemID,
					Name:        item.Name,
					Description: item.Description,
					Quantity:    item.Quantity,
					UnitPrice:   item.UnitPrice,
					TotalPrice:  item.TotalPrice,
					Category:    item.Category,
				})
			}
		}
	}

	// Deserialize recipient details from JSON
	if len(inv.RecipientDetails) > 0 {
		var details models.AddressDetails
		if err := json.Unmarshal(inv.RecipientDetails, &details); err == nil {
			pbInvoice.RecipientDetails = &pb.AddressDetails{
				CompanyName:  details.CompanyName,
				ContactName:  details.ContactName,
				Email:        details.Email,
				Phone:        details.Phone,
				AddressLine1: details.AddressLine1,
				AddressLine2: details.AddressLine2,
				City:         details.City,
				State:        details.State,
				Postcode:     details.Postcode,
				Country:      details.Country,
			}
		}
	}

	// Deserialize payer details from JSON
	if len(inv.PayerDetails) > 0 {
		var details models.AddressDetails
		if err := json.Unmarshal(inv.PayerDetails, &details); err == nil {
			pbInvoice.PayerDetails = &pb.AddressDetails{
				CompanyName:  details.CompanyName,
				ContactName:  details.ContactName,
				Email:        details.Email,
				Phone:        details.Phone,
				AddressLine1: details.AddressLine1,
				AddressLine2: details.AddressLine2,
				City:         details.City,
				State:        details.State,
				Postcode:     details.Postcode,
				Country:      details.Country,
			}
		}
	}

	return pbInvoice, nil
}

// CreateInvoice handles the gRPC request
func (controller *InvoiceController) CreateInvoice(ctx context.Context, req *pb.CreateInvoiceRequest) (*pb.CreateInvoiceResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// --- Validation ---
	if req.Title == "" {
		return nil, status.Errorf(codes.InvalidArgument, "title is required")
	}
	if req.Amount <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "amount must be positive")
	}
	if req.Currency == "" {
		return nil, status.Errorf(codes.InvalidArgument, "currency is required")
	}

	// Parse due date
	dueDate, err := time.Parse(time.RFC3339, req.DueDate)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid due_date format: %v", err)
	}
	if dueDate.Before(time.Now()) {
		return nil, status.Errorf(codes.InvalidArgument, "due_date must be in the future")
	}

	// --- Get User ID ---
	user, err := controller.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// --- Create Invoice ---
	// If no recipient specified, use user's own ID (self-invoice/draft)
	recipientID := req.RecipientId
	if recipientID == "" {
		recipientID = user.UUID
	}

	// Convert proto items to model items
	var items []models.InvoiceItem
	for _, item := range req.Items {
		items = append(items, models.InvoiceItem{
			ItemID:      item.Id,
			Name:        item.Name,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			TotalPrice:  item.TotalPrice,
			Category:    item.Category,
		})
	}

	// Convert proto address details to model
	var recipientDetails, payerDetails *models.AddressDetails
	if req.RecipientDetails != nil {
		recipientDetails = &models.AddressDetails{
			CompanyName:  req.RecipientDetails.CompanyName,
			ContactName:  req.RecipientDetails.ContactName,
			Email:        req.RecipientDetails.Email,
			Phone:        req.RecipientDetails.Phone,
			AddressLine1: req.RecipientDetails.AddressLine1,
			AddressLine2: req.RecipientDetails.AddressLine2,
			City:         req.RecipientDetails.City,
			State:        req.RecipientDetails.State,
			Postcode:     req.RecipientDetails.Postcode,
			Country:      req.RecipientDetails.Country,
		}
	}

	if req.PayerDetails != nil {
		payerDetails = &models.AddressDetails{
			CompanyName:  req.PayerDetails.CompanyName,
			ContactName:  req.PayerDetails.ContactName,
			Email:        req.PayerDetails.Email,
			Phone:        req.PayerDetails.Phone,
			AddressLine1: req.PayerDetails.AddressLine1,
			AddressLine2: req.PayerDetails.AddressLine2,
			City:         req.PayerDetails.City,
			State:        req.PayerDetails.State,
			Postcode:     req.PayerDetails.Postcode,
			Country:      req.PayerDetails.Country,
		}
	}

	invoice := &models.Invoice{
		UserID:           user.UUID,
		RecipientID:      recipientID,
		Title:            req.Title,
		Description:      req.Description,
		Amount:           req.Amount,
		Currency:         req.Currency,
		DueDate:          dueDate,
		Notes:            req.Notes,
		TaxAmount:        req.TaxAmount,
		DiscountAmount:   req.DiscountAmount,
		TotalAmount:      req.TotalAmount,
		ToEmail:          req.ToEmail,
		ToName:           req.ToName,
	}

	// Serialize items to JSON for storage
	if len(items) > 0 {
		itemsJSON, err := json.Marshal(items)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to serialize items: %v", err)
		}
		invoice.Items = itemsJSON
	}

	// Serialize address details to JSON for storage
	if recipientDetails != nil {
		recipientJSON, err := json.Marshal(recipientDetails)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to serialize recipient details: %v", err)
		}
		invoice.RecipientDetails = recipientJSON
	}

	if payerDetails != nil {
		payerJSON, err := json.Marshal(payerDetails)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to serialize payer details: %v", err)
		}
		invoice.PayerDetails = payerJSON
	}

	// --- Call Service ---
	createdInvoice, err := controller.invoiceService.CreateInvoice(ctx, invoice)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidInvoiceData):
			return nil, status.Errorf(codes.InvalidArgument, "invalid invoice data: %v", err)
		case errors.Is(err, services.ErrInvoiceCreationFailed):
			return nil, status.Errorf(codes.Internal, "failed to create invoice: %v", err)
		default:
			return nil, status.Errorf(codes.Internal, "failed to create invoice: %v", err)
		}
	}

	// --- Convert Response ---
	pbInvoice, err := convertInvoiceToProto(createdInvoice)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to format invoice response: %v", err)
	}

	return &pb.CreateInvoiceResponse{Invoice: pbInvoice}, nil
}

// GetInvoices retrieves a paginated list of invoices
func (controller *InvoiceController) GetInvoices(ctx context.Context, req *pb.GetInvoicesRequest) (*pb.GetInvoicesResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// Get user ID from auth
	user, err := controller.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// Call Service
	invoices, total, err := controller.invoiceService.GetInvoices(ctx, user.UUID, int(req.Page), int(req.Limit))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get invoices: %v", err)
	}

	// Convert Response
	pbInvoices := make([]*pb.Invoice, len(invoices))
	for i, invoice := range invoices {
		pbInvoice, err := convertInvoiceToProto(invoice)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to format invoice response: %v", err)
		}
		pbInvoices[i] = pbInvoice
	}

	return &pb.GetInvoicesResponse{
		Invoices: pbInvoices,
		Total:    total,
	}, nil
}

// GetInvoiceById retrieves a single invoice by ID
func (controller *InvoiceController) GetInvoiceById(ctx context.Context, req *pb.GetInvoiceByIdRequest) (*pb.GetInvoiceByIdResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// Get user ID from auth
	user, err := controller.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// Call Service
	invoice, err := controller.invoiceService.GetInvoiceById(ctx, user.UUID, req.InvoiceId)
	if err != nil {
		if errors.Is(err, services.ErrInvoiceNotFound) {
			return nil, status.Errorf(codes.NotFound, "invoice not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get invoice: %v", err)
	}

	// Convert Response
	pbInvoice, err := convertInvoiceToProto(invoice)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to format invoice response: %v", err)
	}

	return &pb.GetInvoiceByIdResponse{Invoice: pbInvoice}, nil
}

// UpdateInvoice handles the request to update an existing invoice
func (c *InvoiceController) UpdateInvoice(ctx context.Context, req *pb.UpdateInvoiceRequest) (*pb.UpdateInvoiceResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	dueDate, err := time.Parse(time.RFC3339, req.DueDate)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid due date format")
	}

	// Convert proto items to model items
	var items []models.InvoiceItem
	for _, item := range req.Items {
		items = append(items, models.InvoiceItem{
			ItemID:      item.Id,
			Name:        item.Name,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			TotalPrice:  item.TotalPrice,
			Category:    item.Category,
		})
	}

	// Convert proto address details to model
	var recipientDetails, payerDetails *models.AddressDetails
	if req.RecipientDetails != nil {
		recipientDetails = &models.AddressDetails{
			CompanyName:  req.RecipientDetails.CompanyName,
			ContactName:  req.RecipientDetails.ContactName,
			Email:        req.RecipientDetails.Email,
			Phone:        req.RecipientDetails.Phone,
			AddressLine1: req.RecipientDetails.AddressLine1,
			AddressLine2: req.RecipientDetails.AddressLine2,
			City:         req.RecipientDetails.City,
			State:        req.RecipientDetails.State,
			Postcode:     req.RecipientDetails.Postcode,
			Country:      req.RecipientDetails.Country,
		}
	}

	if req.PayerDetails != nil {
		payerDetails = &models.AddressDetails{
			CompanyName:  req.PayerDetails.CompanyName,
			ContactName:  req.PayerDetails.ContactName,
			Email:        req.PayerDetails.Email,
			Phone:        req.PayerDetails.Phone,
			AddressLine1: req.PayerDetails.AddressLine1,
			AddressLine2: req.PayerDetails.AddressLine2,
			City:         req.PayerDetails.City,
			State:        req.PayerDetails.State,
			Postcode:     req.PayerDetails.Postcode,
			Country:      req.PayerDetails.Country,
		}
	}

	invoice := &models.Invoice{
		ID:             req.InvoiceId,
		UserID:         user.UUID,
		RecipientID:    req.RecipientId,
		Title:          req.Title,
		Description:    req.Description,
		Amount:         req.Amount,
		Currency:       req.Currency,
		DueDate:        dueDate,
		Notes:          req.Notes,
		TaxAmount:      req.TaxAmount,
		DiscountAmount: req.DiscountAmount,
		TotalAmount:    req.TotalAmount,
		ToEmail:        req.ToEmail,
		ToName:         req.ToName,
	}

	// Serialize items to JSON for storage
	if len(items) > 0 {
		itemsJSON, err := json.Marshal(items)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to serialize items: %v", err)
		}
		invoice.Items = itemsJSON
	}

	// Serialize address details to JSON for storage
	if recipientDetails != nil {
		recipientJSON, err := json.Marshal(recipientDetails)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to serialize recipient details: %v", err)
		}
		invoice.RecipientDetails = recipientJSON
	}

	if payerDetails != nil {
		payerJSON, err := json.Marshal(payerDetails)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to serialize payer details: %v", err)
		}
		invoice.PayerDetails = payerJSON
	}

	updatedInvoice, err := c.invoiceService.UpdateInvoice(ctx, invoice)
	if err != nil {
		if errors.Is(err, services.ErrInvoiceNotFound) {
			return nil, status.Error(codes.NotFound, "invoice not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to update invoice: %v", err)
	}

	pbInvoice, err := convertInvoiceToProto(updatedInvoice)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to format invoice response: %v", err)
	}

	return &pb.UpdateInvoiceResponse{
		Invoice: pbInvoice,
	}, nil
}

// DeleteInvoice handles the request to delete an invoice
func (c *InvoiceController) DeleteInvoice(ctx context.Context, req *pb.DeleteInvoiceRequest) (*pb.DeleteInvoiceResponse, error) {
	user, err := getUserFromContext(ctx, c.userService)
	if err != nil {
		return nil, err
	}

	err = c.invoiceService.DeleteInvoice(ctx, user.UUID, req.InvoiceId)
	if err != nil {
		if errors.Is(err, services.ErrInvoiceNotFound) {
			return nil, status.Error(codes.NotFound, "invoice not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete invoice: %v", err)
	}

	return &pb.DeleteInvoiceResponse{
		Success: true,
	}, nil
}

// GetInvoicesByStatus handles the gRPC request
func (controller *InvoiceController) GetInvoicesByStatus(ctx context.Context, req *pb.GetInvoicesByStatusRequest) (*pb.GetInvoicesByStatusResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// Get user ID from auth
	user, err := controller.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// Get invoices
	invoices, total, err := controller.invoiceService.GetInvoicesByStatus(ctx, user.UUID, req.IsPaid, int(req.Page), int(req.Limit))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get invoices: %v", err)
	}

	// Convert to proto
	pbInvoices := make([]*pb.Invoice, len(invoices))
	for i, invoice := range invoices {
		pbInvoice, err := convertInvoiceToProto(invoice)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to format invoice response: %v", err)
		}
		pbInvoices[i] = pbInvoice
	}

	return &pb.GetInvoicesByStatusResponse{
		Invoices: pbInvoices,
		Total:    total,
	}, nil
}

// MarkInvoiceAsPaid handles marking an invoice as paid with payment details
func (c *InvoiceController) MarkInvoiceAsPaid(ctx context.Context, req *pb.MarkInvoiceAsPaidRequest) (*pb.MarkInvoiceAsPaidResponse, error) {
	// Get auth payload from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// Validate request
	if req.InvoiceId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invoice_id is required")
	}
	if req.PaymentMethod == nil {
		return nil, status.Errorf(codes.InvalidArgument, "payment_method is required")
	}

	// Get user ID from auth
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// Call service to mark invoice as paid
	invoice, err := c.invoiceService.MarkInvoiceAsPaid(ctx, user.UUID, req.InvoiceId, req.PaymentMethod.MethodId, req.PaymentReference)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvoiceNotFound):
			return nil, status.Errorf(codes.NotFound, "invoice not found")
		case errors.Is(err, services.ErrUnauthorizedAccess):
			return nil, status.Errorf(codes.PermissionDenied, "unauthorized access to invoice")
		default:
			return nil, status.Errorf(codes.Internal, "failed to mark invoice as paid: %v", err)
		}
	}

	// Convert to proto response
	pbInvoice, err := convertInvoiceToProto(invoice)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to format invoice response: %v", err)
	}

	return &pb.MarkInvoiceAsPaidResponse{
		Invoice: pbInvoice,
	}, nil
}

// SendInvoice handles sending an invoice to the recipient
func (c *InvoiceController) SendInvoice(ctx context.Context, req *pb.SendInvoiceRequest) (*pb.SendInvoiceResponse, error) {
	// Get auth payload from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// Validate request
	if req.InvoiceId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invoice_id is required")
	}

	// Get user ID from auth
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// Call service to send invoice
	err = c.invoiceService.SendInvoice(ctx, user.UUID, req.InvoiceId)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvoiceNotFound):
			return nil, status.Errorf(codes.NotFound, "invoice not found")
		case errors.Is(err, services.ErrUnauthorizedAccess):
			return nil, status.Errorf(codes.PermissionDenied, "unauthorized access to invoice")
		default:
			return nil, status.Errorf(codes.Internal, "failed to send invoice: %v", err)
		}
	}

	return &pb.SendInvoiceResponse{
		Success: true,
	}, nil
}

// TagUsersToInvoice handles tagging multiple users to an invoice
func (c *InvoiceController) TagUsersToInvoice(ctx context.Context, req *pb.TagUsersToInvoiceRequest) (*pb.TagUsersToInvoiceResponse, error) {
	// Get auth payload from context
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// Validate request
	if req.InvoiceId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invoice_id is required")
	}

	// Get user ID from auth (to ensure user owns the invoice)
	user, err := c.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}

	// Verify invoice ownership
	invoice, err := c.invoiceService.GetInvoiceById(ctx, user.UUID, req.InvoiceId)
	if err != nil {
		if errors.Is(err, services.ErrInvoiceNotFound) {
			return nil, status.Errorf(codes.NotFound, "invoice not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to get invoice: %v", err)
	}

	if invoice.UserID != user.UUID {
		return nil, status.Errorf(codes.PermissionDenied, "unauthorized access to invoice")
	}

	// Call service to tag users
	serviceReq := &services.TagUsersToInvoiceRequest{
		InvoiceID:    req.InvoiceId,
		UserIDs:      req.UserIds,
		Emails:       req.Emails,
		PhoneNumbers: req.PhoneNumbers,
	}

	response, err := c.invoiceService.TagUsersToInvoice(ctx, serviceReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to tag users to invoice: %v", err)
	}

	return &pb.TagUsersToInvoiceResponse{
		Success:       response.Success,
		TaggedUserIds: response.TaggedUserIDs,
		InvitedEmails: response.InvitedEmails,
		InvitedPhones: response.InvitedPhones,
		Message:       response.Message,
	}, nil
}

// SearchInvoiceUsers handles searching for users to tag in invoice
func (c *InvoiceController) SearchInvoiceUsers(ctx context.Context, req *pb.SearchInvoiceUsersRequest) (*pb.SearchInvoiceUsersResponse, error) {
	// Get auth payload from context
	_, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// Validate request
	if req.Query == "" {
		return &pb.SearchInvoiceUsersResponse{
			Users: []*pb.InvoiceUserResult{},
		}, nil
	}

	// Default limit
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	// Call service to search users
	users, err := c.invoiceService.SearchInvoiceUsers(ctx, req.Query, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to search users: %v", err)
	}

	// Convert to proto response
	pbUsers := make([]*pb.InvoiceUserResult, len(users))
	for i, user := range users {
		username := ""
		if user.Username != nil {
			username = *user.Username
		}

		pbUsers[i] = &pb.InvoiceUserResult{
			Id:       user.UUID,
			Name:     user.FirstName + " " + user.LastName,
			Email:    user.Email,
			Username: username,
			Phone:    user.PhoneNumber,
			IsOnline: false, // TODO: Add online status tracking
		}
	}

	return &pb.SearchInvoiceUsersResponse{
		Users: pbUsers,
	}, nil
}

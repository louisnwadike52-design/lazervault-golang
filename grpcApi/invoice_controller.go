package grpcApi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	// Unmarshal Customer Details
	var customerDetails models.CustomerDetails
	if err := json.Unmarshal(inv.CustomerDetails, &customerDetails); err != nil {
		return nil, fmt.Errorf("failed to unmarshal customer details for invoice %s: %w", inv.ID, err)
	}
	pbCustomer := &pb.CustomerDetails{
		Name:    customerDetails.Name,
		Email:   customerDetails.Email,
		Address: customerDetails.Address,
	}

	// Unmarshal Items
	var items []models.InvoiceItem
	if err := json.Unmarshal(inv.Items, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal items for invoice %s: %w", inv.ID, err)
	}
	pbItems := make([]*pb.InvoiceItem, len(items))
	for i, item := range items {
		pbItems[i] = &pb.InvoiceItem{
			ItemId:      item.ItemID,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			TotalPrice:  item.TotalPrice,
		}
	}

	// Convert status string to enum
	statusEnum, _ := pb.InvoiceStatus_value[inv.Status]

	return &pb.Invoice{
		InvoiceId:       inv.ID,
		UserId:          inv.UserID,
		InvoiceNumber:   inv.InvoiceNumber,
		CustomerDetails: pbCustomer,
		Items:           pbItems,
		Subtotal:        inv.Subtotal,
		Tax:             inv.Tax,
		TotalAmount:     inv.TotalAmount,
		CurrencyCode:    inv.CurrencyCode,
		IssueDate:       timestamppb.New(inv.IssueDate),
		DueDate:         timestamppb.New(inv.DueDate),
		Status:          pb.InvoiceStatus(statusEnum),
		Notes:           inv.Notes,
		CreatedAt:       timestamppb.New(inv.CreatedAt),
		UpdatedAt:       timestamppb.New(inv.UpdatedAt),
	}, nil
}

// CreateInvoice handles the gRPC request
func (controller *InvoiceController) CreateInvoice(ctx context.Context, req *pb.CreateInvoiceRequest) (*pb.CreateInvoiceResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// --- Validation (Basic - Service layer does more thorough checks) ---
	if req.CustomerDetails == nil || req.CustomerDetails.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "customer_details with name is required")
	}
	if len(req.Items) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "at least one item is required")
	}
	for i, item := range req.Items {
		if item.Description == "" || item.Quantity <= 0 || item.UnitPrice < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "item at index %d is invalid (missing description, non-positive quantity, or negative unit price)", i)
		}
	}
	if req.CurrencyCode == "" {
		return nil, status.Errorf(codes.InvalidArgument, "currency_code is required")
	}
	if req.DueDate == nil || !req.DueDate.IsValid() || req.DueDate.AsTime().Before(time.Now()) {
		return nil, status.Errorf(codes.InvalidArgument, "due_date is required and must be in the future")
	}

	// --- Get User ID ---
	user, err := controller.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	userID := user.ID // Assuming user model has ID (uint)

	// --- Prepare Service Request ---
	modelItems := make([]models.InvoiceItem, len(req.Items))
	for i, item := range req.Items {
		modelItems[i] = models.InvoiceItem{
			ItemID:      item.ItemId,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			// TotalPrice will be calculated by the service
		}
	}

	serviceReq := &services.CreateInvoiceServiceRequest{
		UserID: fmt.Sprint(userID), // Convert uint user ID to string
		CustomerDetails: models.CustomerDetails{
			Name:    req.CustomerDetails.GetName(),
			Email:   req.CustomerDetails.GetEmail(),
			Address: req.CustomerDetails.GetAddress(),
		},
		Items:        modelItems,
		Tax:          req.GetTax(),
		CurrencyCode: req.GetCurrencyCode(),
		DueDate:      req.DueDate.AsTime(),
		Notes:        req.GetNotes(),
	}

	// --- Call Service ---
	createdInvoice, err := controller.invoiceService.CreateInvoice(ctx, serviceReq)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidInvoiceData), errors.Is(err, services.ErrInvoiceItemInvalid):
			return nil, status.Errorf(codes.InvalidArgument, "invalid invoice data: %v", err)
		case errors.Is(err, services.ErrInvoiceCreationFailed):
			return nil, status.Errorf(codes.Internal, "failed to create invoice: %v", err)
		// Handle other specific errors (like DB constraint errors if not wrapped)
		default:
			return nil, status.Errorf(codes.Internal, "failed to create invoice: %v", err)
		}
	}

	// --- Convert Response ---
	pbInvoice, err := convertInvoiceToProto(createdInvoice)
	if err != nil {
		// Log error but return success as invoice was created
		fmt.Printf("Error converting created invoice %s to proto: %v\n", createdInvoice.ID, err)
		// return nil, status.Errorf(codes.Internal, "failed to format response: %v", err)
	}

	return &pb.CreateInvoiceResponse{Invoice: pbInvoice}, nil
}

// GetInvoice handles the gRPC request
func (controller *InvoiceController) GetInvoice(ctx context.Context, req *pb.GetInvoiceRequest) (*pb.Invoice, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	if req.GetInvoiceId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invoice_id is required")
	}

	// --- Get User ID ---
	user, err := controller.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	userID := user.ID // Assuming user model has ID (uint)

	// --- Call Service ---
	invoice, err := controller.invoiceService.GetInvoice(ctx, fmt.Sprint(userID), req.GetInvoiceId())
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvoiceNotFound):
			return nil, status.Errorf(codes.NotFound, "invoice not found")
		default:
			return nil, status.Errorf(codes.Internal, "failed to get invoice: %v", err)
		}
	}

	// --- Convert Response ---
	pbInvoice, err := convertInvoiceToProto(invoice)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to format invoice response: %v", err)
	}

	return pbInvoice, nil
}

// ListInvoices handles the gRPC request
func (controller *InvoiceController) ListInvoices(ctx context.Context, req *pb.ListInvoicesRequest) (*pb.ListInvoicesResponse, error) {
	authPayload, ok := ctx.Value(middleware.AuthorizationPayloadKey).(*token.Payload)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "unable to retrieve payload from context")
	}

	// --- Get User ID ---
	user, err := controller.userService.GetUserByEmail(ctx, authPayload.Email)
	if err != nil {
		if errors.Is(err, services.ErrUserNotFound) {
			return nil, status.Errorf(codes.Unauthenticated, "user associated with token not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve user details: %v", err)
	}
	userID := user.ID // Assuming user model has ID (uint)

	// --- Prepare Service Request ---
	statusFilterString := ""                                                                     // Default to no filter
	if req.StatusFilter != pb.InvoiceStatus_DRAFT && req.StatusFilter != pb.InvoiceStatus_PAID { // Check if filter is explicitly set (using hypothetical field)
		// TODO: Need a reliable way to check if the enum default (DRAFT) was explicitly sent
		// For now, assume any non-zero value means filter is intended.
		// A better approach might be to use wrappers.Int32Value or a separate bool field.
		if req.StatusFilter != pb.InvoiceStatus_DRAFT {
			statusFilterString = req.StatusFilter.String()
		}
	}

	serviceReq := &services.ListInvoicesServiceRequest{
		UserID:       fmt.Sprint(userID),
		PageSize:     int(req.GetPageSize()),
		PageToken:    req.GetPageToken(),
		StatusFilter: statusFilterString,
	}

	// --- Call Service ---
	invoices, nextToken, err := controller.invoiceService.ListInvoices(ctx, serviceReq)
	if err != nil {
		// Handle potential pagination token errors specifically if service returns them
		return nil, status.Errorf(codes.Internal, "failed to list invoices: %v", err)
	}

	// --- Convert Response ---
	pbInvoices := make([]*pb.Invoice, 0, len(invoices))
	for _, inv := range invoices {
		pbInv, err := convertInvoiceToProto(&inv)
		if err != nil {
			fmt.Printf("Error converting invoice %s to proto: %v\n", inv.ID, err)
			continue // Skip problematic invoice
		}
		pbInvoices = append(pbInvoices, pbInv)
	}

	return &pb.ListInvoicesResponse{
		Invoices:      pbInvoices,
		NextPageToken: nextToken,
	}, nil
}

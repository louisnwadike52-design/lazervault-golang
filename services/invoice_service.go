package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"lazervaultGo/models" // Correct path
	"lazervaultGo/pb"     // Correct path
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrInvoiceNotFound       = errors.New("invoice not found")
	ErrInvalidInvoiceData    = errors.New("invalid invoice data")
	ErrInvoiceItemInvalid    = errors.New("invoice contains invalid item data")
	ErrInvoiceCreationFailed = errors.New("failed to create invoice")
)

const DefaultInvoicePageSize = 20

// IInvoiceService defines the interface for invoice operations
type IInvoiceService interface {
	CreateInvoice(ctx context.Context, req *CreateInvoiceServiceRequest) (*models.Invoice, error)
	GetInvoice(ctx context.Context, userID, invoiceID string) (*models.Invoice, error)
	ListInvoices(ctx context.Context, req *ListInvoicesServiceRequest) ([]models.Invoice, string, error)
}

// InvoiceService implements the IInvoiceService
type InvoiceService struct {
	db *gorm.DB
	// Add dependencies later if needed
}

// NewInvoiceService creates a new InvoiceService
func NewInvoiceService(db *gorm.DB) IInvoiceService {
	return &InvoiceService{db: db}
}

// generateInvoiceNumber generates a simple sequential invoice number (replace with robust logic)
// Warning: This is NOT concurrency-safe. Needs proper sequence generation.
func (s *InvoiceService) generateInvoiceNumber(tx *gorm.DB, userID string) (string, error) {
	var count int64
	// Use the transaction tx for the count within the transaction
	if err := tx.Model(&models.Invoice{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return "", fmt.Errorf("failed to count existing invoices: %w", err)
	}
	return fmt.Sprintf("INV-%04d", count+1), nil
}

// CreateInvoiceServiceRequest contains parameters for creating an invoice
type CreateInvoiceServiceRequest struct {
	UserID          string // ID of the user creating
	CustomerDetails models.CustomerDetails
	Items           []models.InvoiceItem
	Tax             float64
	CurrencyCode    string
	DueDate         time.Time
	Notes           string
}

// CreateInvoice handles validating input and creating a new invoice record
func (s *InvoiceService) CreateInvoice(ctx context.Context, req *CreateInvoiceServiceRequest) (*models.Invoice, error) {
	// --- Validation ---
	if req.UserID == "" {
		return nil, ErrInvalidUserID // Reuse error
	}
	if req.CustomerDetails.Name == "" {
		return nil, fmt.Errorf("%w: customer name is required", ErrInvalidInvoiceData)
	}
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("%w: invoice must contain at least one item", ErrInvalidInvoiceData)
	}
	if req.CurrencyCode == "" {
		return nil, fmt.Errorf("%w: currency code is required", ErrInvalidInvoiceData)
	}
	if req.DueDate.IsZero() || req.DueDate.Before(time.Now()) {
		return nil, fmt.Errorf("%w: due date must be in the future", ErrInvalidInvoiceData)
	}

	var subtotal float64
	calculatedItems := make([]models.InvoiceItem, len(req.Items))
	for i, item := range req.Items {
		if item.Description == "" || item.Quantity <= 0 || item.UnitPrice < 0 {
			return nil, fmt.Errorf("%w at index %d: description, positive quantity, and non-negative unit price required", ErrInvoiceItemInvalid, i)
		}
		total := float64(item.Quantity) * item.UnitPrice
		subtotal += total
		calculatedItems[i] = item             // Use original item
		calculatedItems[i].TotalPrice = total // Set calculated total
	}

	totalAmount := subtotal + req.Tax

	// Marshal embedded JSON fields
	customerJSON, err := json.Marshal(req.CustomerDetails)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal customer details: %w", err)
	}
	itemsJSON, err := json.Marshal(calculatedItems)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal invoice items: %w", err)
	}

	// --- Database Interaction (Transactional) ---
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if tx.Error != nil {
			tx.Rollback()
		}
	}()

	// Generate Invoice Number using the service method
	invoiceNumber, err := s.generateInvoiceNumber(tx, req.UserID) // Call service method
	if err != nil {
		// tx.Rollback() handled by defer
		return nil, fmt.Errorf("failed to generate invoice number: %w", err)
	}

	// Create Invoice model
	invoice := &models.Invoice{
		UserID:          req.UserID,
		InvoiceNumber:   invoiceNumber,
		CustomerDetails: datatypes.JSON(customerJSON),
		Items:           datatypes.JSON(itemsJSON),
		Subtotal:        subtotal,
		Tax:             req.Tax,
		TotalAmount:     totalAmount,
		CurrencyCode:    req.CurrencyCode,
		IssueDate:       time.Now().UTC(),
		DueDate:         req.DueDate,
		Status:          pb.InvoiceStatus_DRAFT.String(), // Start as DRAFT (Will error until proto generated)
		Notes:           req.Notes,
		CreatedAt:       time.Now().UTC(), // GORM default might handle this
	}

	if err := tx.Create(invoice).Error; err != nil {
		// tx.Rollback() handled by defer
		return nil, fmt.Errorf("%w: %v", ErrInvoiceCreationFailed, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return invoice, nil
}

// GetInvoice retrieves a single invoice by ID for a specific user
func (s *InvoiceService) GetInvoice(ctx context.Context, userID, invoiceID string) (*models.Invoice, error) {
	if userID == "" || invoiceID == "" {
		return nil, ErrInvalidUserID // Or a more specific error
	}

	var invoice models.Invoice
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", invoiceID, userID).First(&invoice).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("database error retrieving invoice: %w", err)
	}
	return &invoice, nil
}

// ListInvoicesServiceRequest contains parameters for listing invoices
type ListInvoicesServiceRequest struct {
	UserID       string
	PageSize     int
	PageToken    string // Use invoice ID as page token
	StatusFilter string // Status string (e.g., "DRAFT", "PAID") - empty means no filter
}

// ListInvoices retrieves a paginated list of invoices for a user
func (s *InvoiceService) ListInvoices(ctx context.Context, req *ListInvoicesServiceRequest) ([]models.Invoice, string, error) {
	if req.UserID == "" {
		return nil, "", ErrInvalidUserID
	}

	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = DefaultInvoicePageSize
	}

	var invoices []models.Invoice
	query := s.db.WithContext(ctx).Model(&models.Invoice{}).
		Where("user_id = ?", req.UserID)

	// Apply status filter if provided
	if req.StatusFilter != "" {
		// Validate status filter? Optional.
		query = query.Where("status = ?", req.StatusFilter)
	}

	// Keyset Pagination using CreatedAt and ID
	if req.PageToken != "" {
		var lastInv models.Invoice
		err := s.db.WithContext(ctx).Select("created_at").First(&lastInv, "id = ? AND user_id = ?", req.PageToken, req.UserID).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return []models.Invoice{}, "", fmt.Errorf("invalid page token: %w", err)
			}
			return nil, "", fmt.Errorf("failed to query page token invoice: %w", err)
		}
		query = query.Where("(created_at, id) < (?, ?)", lastInv.CreatedAt, req.PageToken)
	}

	// Order by creation time descending, ID secondary
	err := query.Order("created_at desc, id desc").Limit(pageSize + 1).Find(&invoices).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", fmt.Errorf("failed to retrieve invoices: %w", err)
	}

	// Determine next page token
	nextPageToken := ""
	if len(invoices) > pageSize {
		nextPageToken = invoices[pageSize-1].ID
		invoices = invoices[:pageSize] // Trim the extra invoice
	}

	return invoices, nextPageToken, nil
}

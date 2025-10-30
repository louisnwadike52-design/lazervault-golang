package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models" // Correct path

	// Correct path
	"time"

	"gorm.io/gorm"
)

// Using common errors from errors.go

const DefaultInvoicePageSize = 20

// IInvoiceService defines the interface for invoice operations
type IInvoiceService interface {
	GetInvoices(ctx context.Context, userID string, page, limit int) ([]*models.Invoice, int64, error)
	GetInvoiceById(ctx context.Context, userID, invoiceID string) (*models.Invoice, error)
	CreateInvoice(ctx context.Context, invoice *models.Invoice) (*models.Invoice, error)
	UpdateInvoice(ctx context.Context, invoice *models.Invoice) (*models.Invoice, error)
	DeleteInvoice(ctx context.Context, userID, invoiceID string) error
	GetInvoicesByStatus(ctx context.Context, userID string, isPaid bool, page, limit int) ([]*models.Invoice, int64, error)
	MarkInvoiceAsPaid(ctx context.Context, userID, invoiceID, paymentMethodID, paymentReference string) (*models.Invoice, error)
	SendInvoice(ctx context.Context, userID, invoiceID string) error
	ListInvoices(ctx context.Context, req *ListInvoicesServiceRequest) ([]*models.Invoice, string, error)
}

// ListInvoicesServiceRequest contains parameters for listing invoices
type ListInvoicesServiceRequest struct {
	UserID       string
	PageSize     int
	PageToken    string
	StatusFilter string
}

// InvoiceService implements IInvoiceService
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

// CreateInvoice handles creating a new invoice record
func (s *InvoiceService) CreateInvoice(ctx context.Context, invoice *models.Invoice) (*models.Invoice, error) {
	if err := s.validateInvoice(invoice); err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Create(invoice).Error; err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvoiceCreationFailed, err)
	}

	return invoice, nil
}

// GetInvoices retrieves a paginated list of invoices
func (s *InvoiceService) GetInvoices(ctx context.Context, userID string, page, limit int) ([]*models.Invoice, int64, error) {
	var invoices []*models.Invoice
	var total int64

	if limit <= 0 {
		limit = DefaultInvoicePageSize
	}
	offset := (page - 1) * limit

	tx := s.db.WithContext(ctx).Model(&models.Invoice{})
	if userID != "" {
		tx = tx.Where("user_id = ?", userID)
	}

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := tx.Offset(offset).Limit(limit).Find(&invoices).Error; err != nil {
		return nil, 0, err
	}

	return invoices, total, nil
}

// GetInvoiceById retrieves a single invoice by ID
func (s *InvoiceService) GetInvoiceById(ctx context.Context, userID, invoiceID string) (*models.Invoice, error) {
	var invoice models.Invoice
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", invoiceID, userID).First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, err
	}
	return &invoice, nil
}

// UpdateInvoice updates an existing invoice
func (s *InvoiceService) UpdateInvoice(ctx context.Context, invoice *models.Invoice) (*models.Invoice, error) {
	if err := s.validateInvoice(invoice); err != nil {
		return nil, err
	}

	var existing models.Invoice
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", invoice.ID, invoice.UserID).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, err
	}

	if err := s.db.WithContext(ctx).Model(&existing).Updates(invoice).Error; err != nil {
		return nil, err
	}

	return &existing, nil
}

// DeleteInvoice deletes an invoice
func (s *InvoiceService) DeleteInvoice(ctx context.Context, userID, invoiceID string) error {
	result := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", invoiceID, userID).Delete(&models.Invoice{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrInvoiceNotFound
	}
	return nil
}

// GetInvoicesByStatus retrieves invoices filtered by payment status
func (s *InvoiceService) GetInvoicesByStatus(ctx context.Context, userID string, isPaid bool, page, limit int) ([]*models.Invoice, int64, error) {
	var invoices []*models.Invoice
	var total int64

	if limit <= 0 {
		limit = DefaultInvoicePageSize
	}
	offset := (page - 1) * limit

	tx := s.db.WithContext(ctx).Model(&models.Invoice{}).Where("user_id = ? AND is_paid = ?", userID, isPaid)

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := tx.Offset(offset).Limit(limit).Find(&invoices).Error; err != nil {
		return nil, 0, err
	}

	return invoices, total, nil
}

// MarkInvoiceAsPaid marks an invoice as paid
func (s *InvoiceService) MarkInvoiceAsPaid(ctx context.Context, userID, invoiceID, paymentMethodID, paymentReference string) (*models.Invoice, error) {
	var invoice models.Invoice
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", invoiceID, userID).First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, err
	}

	invoice.IsPaid = true
	invoice.PaymentMethodID = &paymentMethodID
	invoice.PaymentReference = paymentReference
	invoice.Status = models.InvoicePaymentStatusCompleted

	if err := s.db.WithContext(ctx).Save(&invoice).Error; err != nil {
		return nil, err
	}

	return &invoice, nil
}

// SendInvoice sends an invoice to the recipient
func (s *InvoiceService) SendInvoice(ctx context.Context, userID, invoiceID string) error {
	var invoice models.Invoice
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", invoiceID, userID).First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvoiceNotFound
		}
		return err
	}

	// TODO: Implement invoice sending logic
	// This could involve:
	// 1. Generating PDF
	// 2. Sending email
	// 3. Updating invoice status
	// 4. Recording notification/email history

	return nil
}

// validateInvoice validates invoice data
func (s *InvoiceService) validateInvoice(invoice *models.Invoice) error {
	if invoice == nil {
		return ErrInvalidInvoiceData
	}

	if invoice.UserID == "" {
		return fmt.Errorf("%w: user ID is required", ErrInvalidInvoiceData)
	}

	if invoice.RecipientID == "" {
		return fmt.Errorf("%w: recipient ID is required", ErrInvalidInvoiceData)
	}

	if invoice.Title == "" {
		return fmt.Errorf("%w: title is required", ErrInvalidInvoiceData)
	}

	if invoice.Amount <= 0 {
		return fmt.Errorf("%w: amount must be positive", ErrInvalidInvoiceData)
	}

	if invoice.Currency == "" {
		return fmt.Errorf("%w: currency is required", ErrInvalidInvoiceData)
	}

	if invoice.DueDate.IsZero() || invoice.DueDate.Before(time.Now()) {
		return fmt.Errorf("%w: due date must be in the future", ErrInvalidInvoiceData)
	}

	return nil
}

// ListInvoices retrieves a paginated list of invoices with optional status filter
func (s *InvoiceService) ListInvoices(ctx context.Context, req *ListInvoicesServiceRequest) ([]*models.Invoice, string, error) {
	var invoices []*models.Invoice

	if req.PageSize <= 0 {
		req.PageSize = DefaultInvoicePageSize
	}

	tx := s.db.WithContext(ctx).Model(&models.Invoice{})
	if req.UserID != "" {
		tx = tx.Where("user_id = ?", req.UserID)
	}
	if req.StatusFilter != "" {
		tx = tx.Where("status = ?", req.StatusFilter)
	}

	// Handle pagination
	if req.PageToken != "" {
		// Assuming PageToken is a base64 encoded cursor
		// In a real implementation, you would decode and use the cursor
		tx = tx.Where("id > ?", req.PageToken)
	}

	if err := tx.Limit(req.PageSize + 1).Find(&invoices).Error; err != nil {
		return nil, "", err
	}

	// Check if there are more results
	var nextPageToken string
	if len(invoices) > req.PageSize {
		nextPageToken = invoices[req.PageSize-1].ID // Use the last ID as the next page token
		invoices = invoices[:req.PageSize]          // Remove the extra item
	}

	return invoices, nextPageToken, nil
}

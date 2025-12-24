package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models" // Correct path
	"lazervaultGo/tasks"

	// Correct path
	"time"

	"github.com/hibiken/asynq"
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
	TagUsersToInvoice(ctx context.Context, req *TagUsersToInvoiceRequest) (*TagUsersToInvoiceResponse, error)
	SearchInvoiceUsers(ctx context.Context, query string, limit int) ([]*models.User, error)
}

// ListInvoicesServiceRequest contains parameters for listing invoices
type ListInvoicesServiceRequest struct {
	UserID       string
	PageSize     int
	PageToken    string
	StatusFilter string
}

// TagUsersToInvoiceRequest contains parameters for tagging users to an invoice
type TagUsersToInvoiceRequest struct {
	InvoiceID    string
	UserIDs      []string
	Emails       []string
	PhoneNumbers []string
}

// TagUsersToInvoiceResponse contains the result of tagging users to an invoice
type TagUsersToInvoiceResponse struct {
	Success       bool
	TaggedUserIDs []string
	InvitedEmails []string
	InvitedPhones []string
	Message       string
}

// InvoiceService implements IInvoiceService
type InvoiceService struct {
	db          *gorm.DB
	distributor tasks.TaskDistributor
}

// NewInvoiceService creates a new InvoiceService
func NewInvoiceService(db *gorm.DB, distributor tasks.TaskDistributor) IInvoiceService {
	return &InvoiceService{
		db:          db,
		distributor: distributor,
	}
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
	// 1. Fetch the invoice
	var invoice models.Invoice
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", invoiceID, userID).First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvoiceNotFound
		}
		return err
	}

	// 2. Fetch sender (user) details
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to fetch user: %w", err)
	}

	// 3. Fetch recipient details (assuming RecipientID is another user)
	var recipient models.User
	if err := s.db.WithContext(ctx).Where("id = ?", invoice.RecipientID).First(&recipient).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("recipient not found")
		}
		return fmt.Errorf("failed to fetch recipient: %w", err)
	}

	// 4. Prepare email payload
	userName := user.Email
	if user.FirstName != "" && user.LastName != "" {
		userName = user.FirstName + " " + user.LastName
	}

	recipientName := recipient.Email
	if recipient.FirstName != "" && recipient.LastName != "" {
		recipientName = recipient.FirstName + " " + recipient.LastName
	}

	// 5. Queue email task
	payloadBytes, err := tasks.NewSendInvoiceEmailTask(
		userID,
		user.Email,
		userName,
		invoiceID,
		invoice.ID, // Using invoice ID as invoice number for now
		recipient.Email,
		recipientName,
		invoice.Amount,
		invoice.Currency,
		invoice.DueDate.Format(time.RFC3339),
		invoice.Description,
	)
	if err != nil {
		return fmt.Errorf("failed to create email task payload: %w", err)
	}

	if err := s.distributor.DistributeTask(
		ctx,
		tasks.TypeEmailSendInvoice,
		payloadBytes,
		asynq.Queue(tasks.QueueDefault),
		asynq.MaxRetry(3),
	); err != nil {
		return fmt.Errorf("failed to queue invoice email: %w", err)
	}

	// 6. Update invoice status to sent
	if err := s.db.WithContext(ctx).
		Model(&invoice).
		Updates(map[string]interface{}{
			"status":     models.InvoicePaymentStatusPending,
			"updated_at": time.Now().UTC(),
		}).Error; err != nil {
		return fmt.Errorf("failed to update invoice status: %w", err)
	}

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

// TagUsersToInvoice tags multiple users to an invoice and creates tagged invoice records
func (s *InvoiceService) TagUsersToInvoice(ctx context.Context, req *TagUsersToInvoiceRequest) (*TagUsersToInvoiceResponse, error) {
	// Verify invoice exists
	var invoice models.Invoice
	if err := s.db.WithContext(ctx).Where("id = ?", req.InvoiceID).First(&invoice).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("failed to fetch invoice: %w", err)
	}

	response := &TagUsersToInvoiceResponse{
		Success:       true,
		TaggedUserIDs: []string{},
		InvitedEmails: []string{},
		InvitedPhones: []string{},
	}

	// Tag existing users by ID
	for _, userID := range req.UserIDs {
		// Verify user exists
		var user models.User
		if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue // Skip non-existent users
			}
			return nil, fmt.Errorf("failed to fetch user %s: %w", userID, err)
		}

		// Create tagged invoice record
		taggedInvoice := &models.TaggedInvoice{
			InvoiceID:     req.InvoiceID,
			UserID:        userID,
			PaymentStatus: models.InvoicePaymentStatusPending,
			Priority:      "medium",
			IsViewed:      false,
		}

		if err := s.db.WithContext(ctx).Create(taggedInvoice).Error; err != nil {
			// Check if already tagged (unique constraint violation)
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				continue
			}
			return nil, fmt.Errorf("failed to create tagged invoice for user %s: %w", userID, err)
		}

		response.TaggedUserIDs = append(response.TaggedUserIDs, userID)
	}

	// Handle non-platform users by email
	for _, email := range req.Emails {
		// Check if user exists by email
		var user models.User
		err := s.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
		if err == nil {
			// User exists, tag them
			userIDStr := fmt.Sprintf("%d", user.ID)
			taggedInvoice := &models.TaggedInvoice{
				InvoiceID:     req.InvoiceID,
				UserID:        userIDStr,
				PaymentStatus: models.InvoicePaymentStatusPending,
				Priority:      "medium",
				IsViewed:      false,
			}

			if err := s.db.WithContext(ctx).Create(taggedInvoice).Error; err != nil {
				if !errors.Is(err, gorm.ErrDuplicatedKey) {
					return nil, fmt.Errorf("failed to create tagged invoice for email %s: %w", email, err)
				}
			} else {
				response.TaggedUserIDs = append(response.TaggedUserIDs, userIDStr)
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// User doesn't exist, add to invite list
			response.InvitedEmails = append(response.InvitedEmails, email)
			// TODO: Send invitation email
		}
	}

	// Handle non-platform users by phone
	for _, phone := range req.PhoneNumbers {
		// Check if user exists by phone
		var user models.User
		err := s.db.WithContext(ctx).Where("phone_number = ?", phone).First(&user).Error
		if err == nil {
			// User exists, tag them
			userIDStr := fmt.Sprintf("%d", user.ID)
			taggedInvoice := &models.TaggedInvoice{
				InvoiceID:     req.InvoiceID,
				UserID:        userIDStr,
				PaymentStatus: models.InvoicePaymentStatusPending,
				Priority:      "medium",
				IsViewed:      false,
			}

			if err := s.db.WithContext(ctx).Create(taggedInvoice).Error; err != nil {
				if !errors.Is(err, gorm.ErrDuplicatedKey) {
					return nil, fmt.Errorf("failed to create tagged invoice for phone %s: %w", phone, err)
				}
			} else {
				response.TaggedUserIDs = append(response.TaggedUserIDs, userIDStr)
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// User doesn't exist, add to invite list
			response.InvitedPhones = append(response.InvitedPhones, phone)
			// TODO: Send invitation SMS
		}
	}

	// Build response message
	totalTagged := len(response.TaggedUserIDs)
	totalInvited := len(response.InvitedEmails) + len(response.InvitedPhones)

	if totalTagged > 0 && totalInvited > 0 {
		response.Message = fmt.Sprintf("Tagged %d users and sent %d invitations", totalTagged, totalInvited)
	} else if totalTagged > 0 {
		response.Message = fmt.Sprintf("Successfully tagged %d users", totalTagged)
	} else if totalInvited > 0 {
		response.Message = fmt.Sprintf("Sent %d invitations to non-platform users", totalInvited)
	} else {
		response.Message = "No users were tagged"
		response.Success = false
	}

	return response, nil
}

// SearchInvoiceUsers searches for users by name, email, or username
func (s *InvoiceService) SearchInvoiceUsers(ctx context.Context, query string, limit int) ([]*models.User, error) {
	if query == "" {
		return []*models.User{}, nil
	}

	if limit <= 0 {
		limit = 20
	}

	var users []*models.User
	searchPattern := "%" + query + "%"

	// Search by first name, last name, email, or username
	if err := s.db.WithContext(ctx).
		Where("first_name ILIKE ? OR last_name ILIKE ? OR email ILIKE ? OR username ILIKE ?",
			searchPattern, searchPattern, searchPattern, searchPattern).
		Limit(limit).
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to search users: %w", err)
	}

	return users, nil
}

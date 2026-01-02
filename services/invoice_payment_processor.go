package services

import (
	"context"
	"errors"
	"fmt"
	"lazervaultGo/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PaymentProcessor handles actual payment processing logic
type PaymentProcessor struct {
	db                  *gorm.DB
	notificationService *NotificationService
	txTracker           *TransactionTracker // Tracks income/expenditure for statistics
}

// NewPaymentProcessor creates a new payment processor
func NewPaymentProcessor(db *gorm.DB, notificationService *NotificationService) *PaymentProcessor {
	return &PaymentProcessor{
		db:                  db,
		notificationService: notificationService,
		txTracker:           NewTransactionTracker(db), // Initialize transaction tracker
	}
}

// ProcessAccountPayment processes payment from user's account balance
func (p *PaymentProcessor) ProcessAccountPayment(
	ctx context.Context,
	userID string,
	invoiceID string,
	amount float64,
	currency string,
	accountID uint,
	description string,
) (*models.InvoicePaymentTransaction, error) {

	// Start database transaction
	tx := p.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// 1. Fetch and validate account
	var account models.Account
	if err := tx.Where("id = ? AND owner_user_id = ?", accountID, userID).First(&account).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("account not found or access denied")
		}
		return nil, fmt.Errorf("failed to fetch account: %w", err)
	}

	// 2. Check account status
	if account.Status != "active" {
		tx.Rollback()
		return nil, fmt.Errorf("account is not active: %s", account.Status)
	}

	// 3. Convert amount to cents for comparison
	amountCents := int64(amount * 100)

	// 4. Calculate payment fee (0.5% processing fee)
	feeAmount := amount * 0.005
	totalAmountCents := amountCents + int64(feeAmount*100)

	// 5. Check sufficient balance
	if account.Balance < totalAmountCents {
		tx.Rollback()
		return nil, fmt.Errorf("insufficient funds: have %d cents, need %d cents (including %d cents fee)",
			account.Balance, totalAmountCents, int64(feeAmount*100))
	}

	// 6. Fetch invoice to validate
	var invoice models.Invoice
	if err := tx.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invoice not found")
		}
		return nil, fmt.Errorf("failed to fetch invoice: %w", err)
	}

	// 7. Validate invoice is payable
	if invoice.IsPaid {
		tx.Rollback()
		return nil, errors.New("invoice already paid")
	}

	if invoice.Status == models.InvoicePaymentStatusCancelled {
		tx.Rollback()
		return nil, errors.New("invoice is cancelled")
	}

	// 8. Validate amount matches invoice amount
	invoiceAmountCents := int64(invoice.Amount * 100)
	if amountCents != invoiceAmountCents {
		tx.Rollback()
		return nil, fmt.Errorf("payment amount %d does not match invoice amount %d",
			amountCents, invoiceAmountCents)
	}

	// 9. Deduct from account balance
	account.Balance -= totalAmountCents
	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to deduct from account: %w", err)
	}

	// 10. Create payment transaction record
	now := time.Now().UTC()
	transactionID := uuid.New().String()

	transaction := &models.InvoicePaymentTransaction{
		TransactionID:      transactionID,
		InvoiceID:          invoiceID,
		UserID:             userID,
		Amount:             amount,
		Currency:           currency,
		PaymentMethod:      models.PaymentMethodTypeAccountBalance,
		Status:             models.InvoicePaymentStatusCompleted,
		Reference:          fmt.Sprintf("Payment for invoice %s", invoiceID),
		Description:        description,
		FeeAmount:          feeAmount,
		PaymentProcessorID: fmt.Sprintf("account_%d", accountID),
		ConfirmationCode:   generateConfirmationCode(),
		ProcessedAt:        &now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := tx.Create(transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create payment transaction: %w", err)
	}

	// 11. Update invoice status
	invoice.IsPaid = true
	invoice.Status = models.InvoicePaymentStatusCompleted
	invoice.PaymentReference = transactionID
	paymentMethodID := fmt.Sprintf("%d", accountID)
	invoice.PaymentMethodID = &paymentMethodID

	if err := tx.Save(&invoice).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update invoice: %w", err)
	}

	// 12. Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 13. Track transaction for statistics (Non-blocking)
	// Get payer user for tracking
	var payer models.User
	var payerUserID uint
	var payerName string
	if err := p.db.WithContext(ctx).Where("uuid = ?", userID).First(&payer).Error; err == nil {
		payerUserID = payer.ID
		payerName = payer.FirstName + " " + payer.LastName
		if payerName == " " {
			payerName = payer.Email
		}
	}

	// Get invoice creator (recipient) for tracking
	var invoiceCreator models.User
	var creatorUserID uint
	var creatorName string
	// Note: invoice.UserID is a string UUID, attempt to find matching user
	// This may need adjustment based on actual user ID format
	if err := p.db.WithContext(ctx).Where("username = ? OR email = ?", invoice.UserID, invoice.UserID).First(&invoiceCreator).Error; err == nil {
		creatorUserID = invoiceCreator.ID
		creatorName = invoiceCreator.FirstName + " " + invoiceCreator.LastName
		if creatorName == " " {
			creatorName = invoiceCreator.Email
		}
	} else {
		// Fallback to invoice recipient details
		creatorName = invoice.ToName
		if creatorName == "" {
			creatorName = invoice.ToEmail
		}
	}

	// Track expenditure for payer
	if payerUserID > 0 {
		p.txTracker.TrackExpenditureAsync(ctx, ExpenditureTrackingParams{
			UserID:          payerUserID,
			Amount:          transaction.Amount + transaction.FeeAmount, // Include fee
			Currency:        transaction.Currency,
			ExpenseType:     "invoice_payment_made",
			ExpenseID:       transaction.TransactionID,
			ExpenseReference: invoiceID,
			Category:        "EXPENSE_CATEGORY_OTHER",
			RecipientID:     &creatorUserID,
			RecipientName:   creatorName,
			Description:     fmt.Sprintf("Invoice payment to %s", creatorName),
			TransactionDate: transaction.ProcessedAt,
			Metadata: map[string]interface{}{
				"transaction_id":   transaction.TransactionID,
				"invoice_id":       invoiceID,
				"payment_method":   "account_balance",
				"fee_amount":       transaction.FeeAmount,
				"confirmation_code": transaction.ConfirmationCode,
			},
		})
	}

	// Track income for invoice creator (recipient)
	if creatorUserID > 0 {
		p.txTracker.TrackIncomeAsync(ctx, IncomeTrackingParams{
			UserID:          creatorUserID,
			Amount:          transaction.Amount, // Recipient gets amount without fee
			Currency:        transaction.Currency,
			SourceType:      "invoice_payment_received",
			SourceID:        transaction.TransactionID,
			SourceReference: invoiceID,
			Category:        "INCOME_CATEGORY_OTHER",
			Description:     fmt.Sprintf("Invoice payment from %s", payerName),
			SenderID:        &payerUserID,
			SenderName:      payerName,
			TransactionDate: transaction.ProcessedAt,
			Metadata: map[string]interface{}{
				"transaction_id":   transaction.TransactionID,
				"invoice_id":       invoiceID,
				"confirmation_code": transaction.ConfirmationCode,
			},
		})
	}

	// 14. Send notification to invoice creator
	if p.notificationService != nil && payerName != "" {
		// Send notification asynchronously
		go func() {
			_ = p.notificationService.SendInvoicePaidNotification(context.Background(), &invoice, payerName)
		}()
	}

	return transaction, nil
}

// ProcessPartialAccountPayment processes a partial payment from user's account
func (p *PaymentProcessor) ProcessPartialAccountPayment(
	ctx context.Context,
	userID string,
	invoiceID string,
	partialAmount float64,
	currency string,
	accountID uint,
	description string,
) (*models.InvoicePaymentTransaction, float64, error) {

	// Start database transaction
	tx := p.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, 0, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// 1. Fetch account
	var account models.Account
	if err := tx.Where("id = ? AND owner_user_id = ?", accountID, userID).First(&account).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, errors.New("account not found or access denied")
		}
		return nil, 0, fmt.Errorf("failed to fetch account: %w", err)
	}

	// 2. Convert amount to cents
	partialAmountCents := int64(partialAmount * 100)
	feeAmount := partialAmount * 0.005
	totalAmountCents := partialAmountCents + int64(feeAmount*100)

	// 3. Check sufficient balance
	if account.Balance < totalAmountCents {
		tx.Rollback()
		return nil, 0, fmt.Errorf("insufficient funds: have %d cents, need %d cents",
			account.Balance, totalAmountCents)
	}

	// 4. Fetch invoice
	var invoice models.Invoice
	if err := tx.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("failed to fetch invoice: %w", err)
	}

	// 5. Calculate total paid so far
	var totalPaid float64
	if err := tx.Model(&models.InvoicePaymentTransaction{}).
		Where("invoice_id = ? AND status IN (?)", invoiceID, []models.InvoicePaymentStatus{
			models.InvoicePaymentStatusCompleted,
			models.InvoicePaymentStatusPartiallyPaid,
		}).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalPaid).Error; err != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("failed to calculate total paid: %w", err)
	}

	// 6. Calculate remaining amount
	remainingAmount := invoice.Amount - totalPaid - partialAmount
	if remainingAmount < 0 {
		tx.Rollback()
		return nil, 0, fmt.Errorf("partial payment exceeds remaining amount")
	}

	// 7. Deduct from account
	account.Balance -= totalAmountCents
	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("failed to deduct from account: %w", err)
	}

	// 8. Create payment transaction
	now := time.Now().UTC()
	transactionID := uuid.New().String()

	paymentStatus := models.InvoicePaymentStatusPartiallyPaid
	if remainingAmount <= 0.01 { // Account for floating point precision
		paymentStatus = models.InvoicePaymentStatusCompleted
	}

	transaction := &models.InvoicePaymentTransaction{
		TransactionID:      transactionID,
		InvoiceID:          invoiceID,
		UserID:             userID,
		Amount:             partialAmount,
		Currency:           currency,
		PaymentMethod:      models.PaymentMethodTypeAccountBalance,
		Status:             paymentStatus,
		Reference:          fmt.Sprintf("Partial payment for invoice %s", invoiceID),
		Description:        description,
		FeeAmount:          feeAmount,
		PaymentProcessorID: fmt.Sprintf("account_%d", accountID),
		ConfirmationCode:   generateConfirmationCode(),
		ProcessedAt:        &now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := tx.Create(transaction).Error; err != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("failed to create payment transaction: %w", err)
	}

	// 9. Update invoice status
	invoice.Status = paymentStatus
	if paymentStatus == models.InvoicePaymentStatusCompleted {
		invoice.IsPaid = true
		invoice.PaymentReference = transactionID
		paymentMethodID := fmt.Sprintf("%d", accountID)
		invoice.PaymentMethodID = &paymentMethodID
	}

	if err := tx.Save(&invoice).Error; err != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("failed to update invoice: %w", err)
	}

	// 10. Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 11. Send notification to invoice creator if payment is completed
	if paymentStatus == models.InvoicePaymentStatusCompleted && p.notificationService != nil {
		var payer models.User
		if err := p.db.WithContext(ctx).Where("uuid = ?", userID).First(&payer).Error; err == nil {
			payerName := payer.FirstName + " " + payer.LastName
			if payerName == " " {
				payerName = payer.Email
			}
			// Send notification asynchronously
			go func() {
				_ = p.notificationService.SendInvoicePaidNotification(context.Background(), &invoice, payerName)
			}()
		}
	}

	return transaction, remainingAmount, nil
}

// ValidatePayment validates payment data before processing
func (p *PaymentProcessor) ValidatePayment(
	ctx context.Context,
	userID string,
	invoiceID string,
	amount float64,
	accountID uint,
) (bool, []string, float64, float64, error) {

	var validationErrors []string

	// 1. Validate invoice exists
	var invoice models.Invoice
	if err := p.db.WithContext(ctx).First(&invoice, "id = ?", invoiceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			validationErrors = append(validationErrors, "Invoice not found")
			return false, validationErrors, 0, 0, nil
		}
		return false, nil, 0, 0, fmt.Errorf("failed to fetch invoice: %w", err)
	}

	// 2. Check if invoice is payable
	if invoice.IsPaid {
		validationErrors = append(validationErrors, "Invoice is already paid")
	}

	if invoice.Status == models.InvoicePaymentStatusCancelled {
		validationErrors = append(validationErrors, "Invoice is cancelled")
	}

	// 3. Validate amount
	invoiceAmountCents := int64(invoice.Amount * 100)
	paymentAmountCents := int64(amount * 100)

	if paymentAmountCents <= 0 {
		validationErrors = append(validationErrors, "Payment amount must be greater than zero")
	}

	if paymentAmountCents > invoiceAmountCents {
		validationErrors = append(validationErrors, "Payment amount exceeds invoice amount")
	}

	// 4. Check account exists and belongs to user
	var account models.Account
	if err := p.db.WithContext(ctx).Where("id = ? AND owner_user_id = ?", accountID, userID).
		First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			validationErrors = append(validationErrors, "Account not found or access denied")
		} else {
			return false, nil, 0, 0, fmt.Errorf("failed to fetch account: %w", err)
		}
	} else {
		// 5. Check account status
		if account.Status != "active" {
			validationErrors = append(validationErrors, fmt.Sprintf("Account is not active: %s", account.Status))
		}

		// 6. Check sufficient balance
		feeAmount := amount * 0.005
		totalRequired := int64((amount + feeAmount) * 100)

		if account.Balance < totalRequired {
			availableBalance := float64(account.Balance) / 100.0
			validationErrors = append(validationErrors,
				fmt.Sprintf("Insufficient funds. Available: %.2f, Required: %.2f (including %.2f fee)",
					availableBalance, amount+feeAmount, feeAmount))
		}

		// Return available balance and fee
		if len(validationErrors) == 0 {
			availableBalance := float64(account.Balance) / 100.0
			return true, nil, availableBalance, feeAmount, nil
		}
	}

	return false, validationErrors, 0, 0, nil
}

// generateConfirmationCode generates a unique confirmation code
func generateConfirmationCode() string {
	// Generate 8-character alphanumeric code
	return uuid.New().String()[:8]
}

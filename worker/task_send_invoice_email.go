package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"lazervaultGo/mail"
	"lazervaultGo/tasks"

	"github.com/hibiken/asynq"
)

// HandleEmailSendInvoiceTask sends an invoice email to the recipient.
func HandleEmailSendInvoiceTask(ctx context.Context, t *asynq.Task, mailer mail.EmailSender) error {
	var payload tasks.EmailSendInvoicePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal invoice email payload: %w", asynq.SkipRetry)
	}

	// Compose invoice email
	subject := fmt.Sprintf("Invoice from %s - %s %.2f Due", payload.UserName, payload.Currency, payload.Amount)

	content := fmt.Sprintf(`
		<html>
		<body>
			<h2>Invoice from %s</h2>
			<p>Hello %s,</p>
			<p>You have received an invoice:</p>
			<table border="1" cellpadding="10">
				<tr><td><strong>Invoice Number:</strong></td><td>%s</td></tr>
				<tr><td><strong>Amount:</strong></td><td>%s %.2f</td></tr>
				<tr><td><strong>Due Date:</strong></td><td>%s</td></tr>
				<tr><td><strong>Description:</strong></td><td>%s</td></tr>
			</table>
			<p>Please make payment before the due date.</p>
			<p>Thank you!</p>
		</body>
		</html>
	`, payload.UserName, payload.RecipientName, payload.InvoiceNumber,
	   payload.Currency, payload.Amount, payload.DueDate, payload.Description)

	to := []string{payload.RecipientEmail}

	if err := mailer.SendEmail(subject, content, to, nil, nil, nil); err != nil {
		return fmt.Errorf("failed to send invoice email: %w", err)
	}

	fmt.Printf("Sent invoice email to %s (Invoice ID: %s)\n", payload.RecipientEmail, payload.InvoiceID)
	return nil
}

// HandleEmailSendPaymentConfirmationTask sends a payment confirmation email.
func HandleEmailSendPaymentConfirmationTask(ctx context.Context, t *asynq.Task, mailer mail.EmailSender) error {
	var payload tasks.EmailSendPaymentConfirmationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payment confirmation email payload: %w", asynq.SkipRetry)
	}

	subject := fmt.Sprintf("Payment Confirmation - Invoice %s", payload.InvoiceNumber)

	content := fmt.Sprintf(`
		<html>
		<body>
			<h2>Payment Received</h2>
			<p>Hello %s,</p>
			<p>Your payment has been successfully processed:</p>
			<table border="1" cellpadding="10">
				<tr><td><strong>Invoice Number:</strong></td><td>%s</td></tr>
				<tr><td><strong>Amount Paid:</strong></td><td>%s %.2f</td></tr>
				<tr><td><strong>Fee:</strong></td><td>%s %.2f</td></tr>
				<tr><td><strong>Transaction ID:</strong></td><td>%s</td></tr>
				<tr><td><strong>Confirmation Code:</strong></td><td>%s</td></tr>
				<tr><td><strong>Payment Method:</strong></td><td>%s</td></tr>
				<tr><td><strong>Processed At:</strong></td><td>%s</td></tr>
			</table>
			<p>Thank you for your payment!</p>
		</body>
		</html>
	`, payload.UserName, payload.InvoiceNumber, payload.Currency, payload.Amount,
	   payload.Currency, payload.FeeAmount, payload.TransactionID, payload.ConfirmationCode,
	   payload.PaymentMethod, payload.ProcessedAt)

	to := []string{payload.UserEmail}

	if err := mailer.SendEmail(subject, content, to, nil, nil, nil); err != nil {
		return fmt.Errorf("failed to send payment confirmation email: %w", err)
	}

	fmt.Printf("Sent payment confirmation email to %s (Transaction ID: %s)\n", payload.UserEmail, payload.TransactionID)
	return nil
}

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"lazervaultGo/mail"
	"lazervaultGo/tasks"

	"github.com/hibiken/asynq"
)

// HandleEmailSendVerifyUserTask sends a verification email.
func HandleEmailSendVerifyUserTask(ctx context.Context, t *asynq.Task, mailer mail.EmailSender) error {
	var payload tasks.PayloadSendVerifyEmail
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal verify user email payload: %w", asynq.SkipRetry)
	}

	// TODO: Create a more structured verification email template
	subject := "Welcome to LazerVault - Verify Your Email"
	verifyURL := fmt.Sprintf("http://localhost:8080/v1/verify_email?user_id=%d&secret_code=%s", payload.UserID, payload.SecretCode)
	content := fmt.Sprintf("Hello %s,<br/>Thank you for registering! Please verify your email by clicking this link: <a href='%s'>Verify Email</a><br/>Your verification code is: <strong>%s</strong>", payload.Username, verifyURL, payload.SecretCode)
	to := []string{payload.Email}

	if err := mailer.SendEmail(subject, content, to, nil, nil, nil); err != nil {
		return fmt.Errorf("failed to send verify user email: %w", err) // Let Asynq handle retry
	}

	fmt.Printf("Sent verification email to %s (User ID: %d)\n", payload.Email, payload.UserID)
	return nil
}

// HandleEmailSendDepositReversalTask sends a deposit failure notification email.
func HandleEmailSendDepositReversalTask(ctx context.Context, t *asynq.Task, mailer mail.EmailSender) error {
	var payload tasks.EmailSendDepositReversalPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal deposit reversal email payload: %w", asynq.SkipRetry)
	}

	// Convert int64 minor unit amount back to float for display
	displayAmount := float64(payload.Amount) / 100.0

	subject := fmt.Sprintf("Deposit Failed - %s %.2f", payload.Currency, displayAmount)
	// TODO: Create a proper email template
	content := fmt.Sprintf("Hello,<br/>We regret to inform you that your recent deposit attempt of %.2f %s failed.<br/>Reason: %s<br/>Please contact support if you believe this is an error.", displayAmount, payload.Currency, payload.FailureReason)
	to := []string{payload.UserEmail}

	if err := mailer.SendEmail(subject, content, to, nil, nil, nil); err != nil {
		return fmt.Errorf("failed to send deposit reversal email: %w", err)
	}

	fmt.Printf("Sent deposit reversal email to %s\n", payload.UserEmail)
	return nil
}

// HandleEmailSendWithdrawalConfirmationTask sends a withdrawal success notification.
func HandleEmailSendWithdrawalConfirmationTask(ctx context.Context, t *asynq.Task, mailer mail.EmailSender) error {
	var payload tasks.EmailSendWithdrawalConfirmationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal withdrawal confirmation email payload: %w", asynq.SkipRetry)
	}

	// Convert int64 minor unit amount back to float for display
	displayAmount := float64(payload.Amount) / 100.0

	subject := fmt.Sprintf("Withdrawal Completed - %s %.2f", payload.Currency, displayAmount)
	// TODO: Create a proper email template
	content := fmt.Sprintf("Hello,<br/>Your withdrawal of %.2f %s to %s account ending in %s has been processed successfully.<br/>Thank you for using LazerVault.",
		displayAmount, payload.Currency, payload.TargetBankName, payload.TargetAccountNumber)
	to := []string{payload.UserEmail}

	if err := mailer.SendEmail(subject, content, to, nil, nil, nil); err != nil {
		// Log error but allow retry
		fmt.Printf("Error sending withdrawal confirmation email to %s: %v\n", payload.UserEmail, err)
		return fmt.Errorf("failed to send withdrawal confirmation email: %w", err)
	}

	fmt.Printf("Sent withdrawal confirmation email to %s\n", payload.UserEmail)
	return nil
}

// HandleEmailSendWithdrawalFailureTask sends a withdrawal failure notification.
func HandleEmailSendWithdrawalFailureTask(ctx context.Context, t *asynq.Task, mailer mail.EmailSender) error {
	var payload tasks.EmailSendWithdrawalFailurePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal withdrawal failure email payload: %w", asynq.SkipRetry)
	}

	// Convert int64 minor unit amount back to float for display
	displayAmount := float64(payload.Amount) / 100.0

	subject := fmt.Sprintf("Withdrawal Failed - %s %.2f", payload.Currency, displayAmount)
	// TODO: Create a proper email template
	content := fmt.Sprintf("Hello,<br/>We regret to inform you that your recent withdrawal attempt of %.2f %s to %s account ending in %s failed.<br/>Reason: %s<br/>The funds have been returned to your account.<br/>Please contact support if you have questions.",
		displayAmount, payload.Currency, payload.TargetBankName, payload.TargetAccountNumber, payload.FailureReason)
	to := []string{payload.UserEmail}

	if err := mailer.SendEmail(subject, content, to, nil, nil, nil); err != nil {
		// Log error but allow retry
		fmt.Printf("Error sending withdrawal failure email to %s: %v\n", payload.UserEmail, err)
		return fmt.Errorf("failed to send withdrawal failure email: %w", err)
	}

	fmt.Printf("Sent withdrawal failure email to %s\n", payload.UserEmail)
	return nil
}

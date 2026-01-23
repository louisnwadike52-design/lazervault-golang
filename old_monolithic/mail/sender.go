package mail

import (
	"fmt"
	"net/smtp"

	"github.com/jordan-wright/email"
)

type EmailSender interface {
	SendEmail(
		subject string,
		content string,
		to []string,
		cc []string,
		bcc []string,
		attachFiles []string,
	) error
}

// SMTPSender is a generic SMTP email sender supporting any SMTP server
type SMTPSender struct {
	name              string
	fromEmailAddress  string
	fromEmailPassword string
	smtpHost          string
	smtpPort          string
	smtpAuthAddress   string
}

// NewSMTPSender creates a new SMTP sender with configurable SMTP settings
func NewSMTPSender(
	name string,
	fromEmailAddress string,
	fromEmailPassword string,
	smtpHost string,
	smtpPort string,
	smtpAuthAddress string,
) EmailSender {
	return &SMTPSender{
		name:              name,
		fromEmailAddress:  fromEmailAddress,
		fromEmailPassword: fromEmailPassword,
		smtpHost:          smtpHost,
		smtpPort:          smtpPort,
		smtpAuthAddress:   smtpAuthAddress,
	}
}

// Deprecated: Use NewSMTPSender instead
// NewGmailSender creates a Gmail-specific sender (for backward compatibility)
func NewGmailSender(name string, fromEmailAddress string, fromEmailPassword string) EmailSender {
	return NewSMTPSender(
		name,
		fromEmailAddress,
		fromEmailPassword,
		"smtp.gmail.com",
		"587",
		"smtp.gmail.com",
	)
}

func (sender *SMTPSender) SendEmail(
	subject string,
	content string,
	to []string,
	cc []string,
	bcc []string,
	attachFiles []string,
) error {
	e := email.NewEmail()
	e.From = fmt.Sprintf("%s <%s>", sender.name, sender.fromEmailAddress)
	e.Subject = subject
	e.HTML = []byte(content)
	e.To = to
	e.Cc = cc
	e.Bcc = bcc

	for _, f := range attachFiles {
		_, err := e.AttachFile(f)
		if err != nil {
			return fmt.Errorf("failed to attach file %s: %w", f, err)
		}
	}

	smtpServerAddress := fmt.Sprintf("%s:%s", sender.smtpHost, sender.smtpPort)
	smtpAuth := smtp.PlainAuth("", sender.fromEmailAddress, sender.fromEmailPassword, sender.smtpAuthAddress)
	return e.Send(smtpServerAddress, smtpAuth)
}

// SendPasswordResetEmail sends a password reset email.
// resetLink is the URL the user will click (e.g., "https://yourapp.com/reset-password?token=...")
func (sender *SMTPSender) SendPasswordResetEmail(toEmail string, resetToken string) error {
	// TODO: Construct the actual reset link based on your frontend URL structure
	// It's crucial this link points to your frontend reset password page
	// and includes the token.
	// Example: resetLink := fmt.Sprintf("http://localhost:3000/reset-password?token=%s", resetToken)
	resetLink := fmt.Sprintf("YOUR_FRONTEND_RESET_URL?token=%s", resetToken) // IMPORTANT: Replace with actual URL

	subject := "Reset Your LazerVault Password"
	// TODO: Create a nicer HTML email template
	content := fmt.Sprintf(`
		<h1>Password Reset Request</h1>
		<p>You requested a password reset for your LazerVault account.</p>
		<p>Click the link below to set a new password:</p>
		<p><a href="%s">Reset Password</a></p>
		<p>This link will expire in 1 hour.</p>
		<p>If you did not request a password reset, please ignore this email.</p>
		<br>
		<p>Thanks,</p>
		<p>The LazerVault Team</p>
	`, resetLink)

	return sender.SendEmail(subject, content, []string{toEmail}, nil, nil, nil)
}

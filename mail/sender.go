package mail

import (
	"fmt"
	"net/smtp"

	"github.com/jordan-wright/email"
)

const (
	smtpAuthAddress   = "smtp.gmail.com"
	smtpServerAddress = "smtp.gmail.com:587"
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

type GmailSender struct {
	name              string
	fromEmailAddress  string
	fromEmailPassword string
}

func NewGmailSender(name string, fromEmailAddress string, fromEmailPassword string) EmailSender {
	return &GmailSender{
		name:              name,
		fromEmailAddress:  fromEmailAddress,
		fromEmailPassword: fromEmailPassword,
	}
}

func (sender *GmailSender) SendEmail(
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

	smtpAuth := smtp.PlainAuth("", sender.fromEmailAddress, sender.fromEmailPassword, smtpAuthAddress)
	return e.Send(smtpServerAddress, smtpAuth)
}

// SendPasswordResetEmail sends a password reset email.
// resetLink is the URL the user will click (e.g., "https://yourapp.com/reset-password?token=...")
func (sender *GmailSender) SendPasswordResetEmail(toEmail string, resetToken string) error {
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

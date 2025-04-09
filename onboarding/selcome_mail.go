package onboarding

import "lazervaultGo/mail"

func SendWelcomeEmail(email, subject, body string) error {
	sender := mail.NewGmailSender("Lazervault", "lazervault@gmail.com", "lazervault123")
	sender.SendEmail(subject, body, []string{email}, []string{}, []string{}, []string{})
	return nil
}

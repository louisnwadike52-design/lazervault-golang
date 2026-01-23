package mail

import "fmt"

// SendEmailVerificationEmail sends an email verification code to the user
func (sender *SMTPSender) SendEmailVerificationEmail(toEmail string, verificationCode string, userName string) error {
	subject := "Verify Your Lazervault Email Address"

	// Create a nice HTML email template
	content := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .container {
            background-color: #ffffff;
            border-radius: 10px;
            padding: 40px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        .header {
            text-align: center;
            margin-bottom: 30px;
        }
        .logo {
            font-size: 32px;
            font-weight: bold;
            color: #2563eb;
            margin-bottom: 10px;
        }
        .verification-code {
            background-color: #f3f4f6;
            border: 2px dashed #2563eb;
            border-radius: 8px;
            padding: 20px;
            text-align: center;
            margin: 30px 0;
        }
        .code {
            font-size: 36px;
            font-weight: bold;
            letter-spacing: 8px;
            color: #2563eb;
            font-family: 'Courier New', monospace;
        }
        .button {
            display: inline-block;
            background-color: #2563eb;
            color: #ffffff !important;
            text-decoration: none;
            padding: 14px 30px;
            border-radius: 6px;
            font-weight: 600;
            margin: 20px 0;
        }
        .footer {
            margin-top: 40px;
            padding-top: 20px;
            border-top: 1px solid #e5e7eb;
            font-size: 14px;
            color: #6b7280;
            text-align: center;
        }
        .warning {
            background-color: #fef3c7;
            border-left: 4px solid #f59e0b;
            padding: 12px;
            margin: 20px 0;
            border-radius: 4px;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="logo">🔐 Lazervault</div>
            <h1 style="color: #111827; margin: 0;">Welcome to Lazervault!</h1>
            <p style="color: #6b7280; margin-top: 10px;">Verify your email to get started</p>
        </div>

        <p>Hi %s,</p>

        <p>Thank you for signing up with Lazervault! To complete your registration and secure your account, please verify your email address by entering the verification code below:</p>

        <div class="verification-code">
            <p style="margin: 0 0 10px 0; color: #6b7280; font-size: 14px;">Your Verification Code</p>
            <div class="code">%s</div>
            <p style="margin: 10px 0 0 0; color: #6b7280; font-size: 12px;">Valid for 15 minutes</p>
        </div>

        <p>Enter this code in the Lazervault app to verify your email address and activate your account.</p>

        <div class="warning">
            <strong>⚠️ Security Note:</strong> If you didn't create a Lazervault account, please ignore this email. Your email address will not be used without verification.
        </div>

        <div class="footer">
            <p><strong>Need help?</strong></p>
            <p>If you have any questions or need assistance, please contact our support team.</p>
            <p style="margin-top: 20px;">
                Best regards,<br>
                <strong>The Lazervault Team</strong>
            </p>
            <p style="font-size: 12px; margin-top: 20px; color: #9ca3af;">
                This is an automated email. Please do not reply to this message.
            </p>
        </div>
    </div>
</body>
</html>
	`, userName, verificationCode)

	return sender.SendEmail(subject, content, []string{toEmail}, nil, nil, nil)
}

// SendWelcomeEmail sends a welcome email after successful verification
func (sender *SMTPSender) SendWelcomeEmail(toEmail string, userName string) error {
	subject := "Welcome to Lazervault - Get Started!"

	content := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .container {
            background-color: #ffffff;
            border-radius: 10px;
            padding: 40px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        .header {
            text-align: center;
            margin-bottom: 30px;
        }
        .logo {
            font-size: 32px;
            font-weight: bold;
            color: #2563eb;
            margin-bottom: 10px;
        }
        .feature {
            background-color: #f9fafb;
            border-radius: 8px;
            padding: 15px;
            margin: 15px 0;
        }
        .feature-title {
            font-weight: 600;
            color: #111827;
            margin-bottom: 5px;
        }
        .button {
            display: inline-block;
            background-color: #2563eb;
            color: #ffffff !important;
            text-decoration: none;
            padding: 14px 30px;
            border-radius: 6px;
            font-weight: 600;
            margin: 20px 0;
            text-align: center;
        }
        .footer {
            margin-top: 40px;
            padding-top: 20px;
            border-top: 1px solid #e5e7eb;
            font-size: 14px;
            color: #6b7280;
            text-align: center;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="logo">🎉 Lazervault</div>
            <h1 style="color: #111827; margin: 0;">Welcome Aboard, %s!</h1>
            <p style="color: #6b7280; margin-top: 10px;">Your account is now active</p>
        </div>

        <p>Congratulations! Your email has been verified and your Lazervault account is now fully activated.</p>

        <p>Here's what you can do with Lazervault:</p>

        <div class="feature">
            <div class="feature-title">💳 Manage Your Cards</div>
            <p style="margin: 5px 0 0 0; font-size: 14px; color: #6b7280;">Securely store and manage all your payment cards in one place.</p>
        </div>

        <div class="feature">
            <div class="feature-title">💸 Send & Receive Money</div>
            <p style="margin: 5px 0 0 0; font-size: 14px; color: #6b7280;">Transfer funds instantly to friends and family worldwide.</p>
        </div>

        <div class="feature">
            <div class="feature-title">📊 Track Your Finances</div>
            <p style="margin: 5px 0 0 0; font-size: 14px; color: #6b7280;">Get insights into your spending with detailed statistics and analytics.</p>
        </div>

        <div class="feature">
            <div class="feature-title">🎁 Gift Cards & More</div>
            <p style="margin: 5px 0 0 0; font-size: 14px; color: #6b7280;">Purchase gift cards, trade crypto, invest in stocks, and much more.</p>
        </div>

        <p style="margin-top: 30px;">Ready to get started? Open the Lazervault app and explore all the amazing features!</p>

        <div class="footer">
            <p><strong>Need Help Getting Started?</strong></p>
            <p>Check out our help center or contact our support team - we're here to help!</p>
            <p style="margin-top: 20px;">
                Best regards,<br>
                <strong>The Lazervault Team</strong>
            </p>
        </div>
    </div>
</body>
</html>
	`, userName)

	return sender.SendEmail(subject, content, []string{toEmail}, nil, nil, nil)
}

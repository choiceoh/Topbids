package notify

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// SMTPConfig holds the configuration for sending emails via SMTP.
type SMTPConfig struct {
	Host     string // e.g. "smtp.gmail.com"
	Port     string // e.g. "587"
	Username string
	Password string
	From     string // e.g. "noreply@example.com"
}

// EmailNotifier sends notifications via SMTP email.
type EmailNotifier struct {
	cfg SMTPConfig
	// resolveEmail maps a Topbid user UUID to an email address.
	resolveEmail func(ctx context.Context, userID string) (string, error)
}

// NewEmailNotifier creates an email notifier with the given config and user resolver.
func NewEmailNotifier(cfg SMTPConfig, resolver func(ctx context.Context, userID string) (string, error)) *EmailNotifier {
	return &EmailNotifier{cfg: cfg, resolveEmail: resolver}
}

func (e *EmailNotifier) Name() string { return "email" }

func (e *EmailNotifier) Send(ctx context.Context, userID string, msg Message) error {
	email, err := e.resolveEmail(ctx, userID)
	if err != nil {
		return fmt.Errorf("email: resolve user %s: %w", userID, err)
	}
	return e.sendMail(email, msg.Title, msg.Body)
}

func (e *EmailNotifier) SendBulk(ctx context.Context, userIDs []string, msg Message) error {
	var firstErr error
	for _, uid := range userIDs {
		if err := e.Send(ctx, uid, msg); err != nil {
			slog.Error("email send failed", "user", uid, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (e *EmailNotifier) sendMail(to, subject, body string) error {
	addr := e.cfg.Host + ":" + e.cfg.Port

	headers := []string{
		"From: " + e.cfg.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}
	message := strings.Join(headers, "\r\n") + "\r\n\r\n" + body

	var auth smtp.Auth
	if e.cfg.Username != "" {
		auth = smtp.PlainAuth("", e.cfg.Username, e.cfg.Password, e.cfg.Host)
	}
	return smtp.SendMail(addr, auth, e.cfg.From, []string{to}, []byte(message))
}

// SendDirect sends an email directly to an address (not a user ID).
func (e *EmailNotifier) SendDirect(to, subject, body string) error {
	return e.sendMail(to, subject, body)
}

// SendWithAttachment sends an email with a file attachment.
func (e *EmailNotifier) SendWithAttachment(to, subject, htmlBody, attachName string, attachData []byte) error {
	boundary := fmt.Sprintf("topbid-%x", len(attachData))
	addr := e.cfg.Host + ":" + e.cfg.Port

	var sb strings.Builder
	sb.WriteString("From: " + e.cfg.From + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: multipart/mixed; boundary=" + boundary + "\r\n\r\n")

	// HTML body part.
	sb.WriteString("--" + boundary + "\r\n")
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	sb.WriteString(htmlBody + "\r\n")

	// Attachment part.
	sb.WriteString("--" + boundary + "\r\n")
	sb.WriteString("Content-Type: application/octet-stream\r\n")
	sb.WriteString("Content-Disposition: attachment; filename=\"" + attachName + "\"\r\n")
	sb.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")

	encoded := base64.StdEncoding.EncodeToString(attachData)
	// Insert line breaks every 76 chars for RFC compliance.
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		sb.WriteString(encoded[i:end] + "\r\n")
	}

	sb.WriteString("--" + boundary + "--\r\n")

	var auth smtp.Auth
	if e.cfg.Username != "" {
		auth = smtp.PlainAuth("", e.cfg.Username, e.cfg.Password, e.cfg.Host)
	}
	return smtp.SendMail(addr, auth, e.cfg.From, []string{to}, []byte(sb.String()))
}

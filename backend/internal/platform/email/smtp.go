package email

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const smtpBoundary = "pehlione-demo-bank-alternative"

func (s *Service) sendSMTP(ctx context.Context, recipient, subject, htmlBody, textBody string) error {
	from, err := mail.ParseAddress(s.config.From)
	if err != nil {
		return fmt.Errorf("parse SMTP sender: %w", err)
	}
	to, err := mail.ParseAddress(recipient)
	if err != nil {
		return fmt.Errorf("parse SMTP recipient: %w", err)
	}

	address := net.JoinHostPort(s.config.SMTPHost, s.config.SMTPPort)
	dialer := net.Dialer{Timeout: 10 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect SMTP server: %w", err)
	}
	client, err := smtp.NewClient(connection, s.config.SMTPHost)
	if err != nil {
		if closeErr := connection.Close(); closeErr != nil {
			return fmt.Errorf("create SMTP client: %w (connection close: %v)", err, closeErr)
		}
		return fmt.Errorf("create SMTP client: %w", err)
	}
	closedCleanly := false
	defer func() {
		if !closedCleanly {
			if closeErr := client.Close(); closeErr != nil {
				log.Warn().Err(closeErr).Msg("Failed to close SMTP connection")
			}
		}
	}()

	if s.config.SMTPUser != "" {
		if s.config.SMTPPassword == "" {
			return errors.New("SMTP_PASSWORD is required when SMTP_USER is configured")
		}
		if err = client.Auth(smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPassword, s.config.SMTPHost)); err != nil {
			return fmt.Errorf("authenticate SMTP client: %w", err)
		}
	}
	if err = client.Mail(from.Address); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err = client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("start SMTP body: %w", err)
	}
	if _, err = w.Write(buildSMTPMessage(s.config.From, to.Address, subject, htmlBody, textBody)); err != nil {
		return fmt.Errorf("write SMTP body: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("finish SMTP body: %w", err)
	}
	if err = client.Quit(); err != nil {
		return fmt.Errorf("close SMTP session: %w", err)
	}
	closedCleanly = true
	return nil
}

func buildSMTPMessage(from, recipient, subject, htmlBody, textBody string) []byte {
	cleanSubject := strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	var message bytes.Buffer
	fmt.Fprintf(&message, "From: %s\r\n", from)
	fmt.Fprintf(&message, "To: %s\r\n", recipient)
	fmt.Fprintf(&message, "Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", cleanSubject))
	message.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&message, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", smtpBoundary)
	fmt.Fprintf(&message, "--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", smtpBoundary, textBody)
	fmt.Fprintf(&message, "--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", smtpBoundary, htmlBody)
	fmt.Fprintf(&message, "--%s--\r\n", smtpBoundary)
	return message.Bytes()
}

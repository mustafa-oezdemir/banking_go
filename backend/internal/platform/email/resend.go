// Package email sends transactional email through SMTP or the Resend HTTPS API.
package email

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/resend/resend-go/v3"
	"github.com/shopspring/decimal"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
)

const (
	defaultResendEndpoint = "https://api.resend.com/"
	defaultFromAddress    = "Pehlione DemoBank <banking@pehlione.com>"
	defaultFrontendURL    = "http://localhost:3000"
)

// Config contains Resend delivery settings.
type Config struct {
	APIKey       string
	From         string
	FrontendURL  string
	Endpoint     string
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
}

// Service delivers fully resolved notification commands through SMTP or Resend.
type Service struct {
	resendClient *resend.Client
	config       Config
}

// NewFromEnvironment constructs a Resend client from runtime configuration.
func NewFromEnvironment() *Service {
	from := firstNonEmpty(os.Getenv("MAIL_FROM"), os.Getenv("RESEND_FROM_EMAIL"), defaultFromAddress)
	frontendURL := firstNonEmpty(os.Getenv("FRONTEND_URL"), defaultFrontendURL)
	return NewService(Config{
		APIKey:       strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
		From:         from,
		FrontendURL:  frontendURL,
		Endpoint:     defaultResendEndpoint,
		SMTPHost:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		SMTPPort:     firstNonEmpty(os.Getenv("SMTP_PORT"), "1025"),
		SMTPUser:     strings.TrimSpace(os.Getenv("SMTP_USER")),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
	}, &http.Client{Timeout: 10 * time.Second})
}

// NewService constructs a configurable service, primarily for tests.
func NewService(config Config, client *http.Client) *Service {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.From = strings.TrimSpace(config.From)
	config.FrontendURL = strings.TrimRight(strings.TrimSpace(config.FrontendURL), "/")
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	config.SMTPHost = strings.TrimSpace(config.SMTPHost)
	config.SMTPPort = strings.TrimSpace(config.SMTPPort)
	config.SMTPUser = strings.TrimSpace(config.SMTPUser)
	if config.SMTPPort == "" {
		config.SMTPPort = "1025"
	}
	if config.Endpoint == "" {
		config.Endpoint = defaultResendEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resendClient := resend.NewCustomClient(client, config.APIKey)
	if baseURL, err := url.Parse(resendBaseURL(config.Endpoint)); err == nil {
		resendClient.BaseURL = baseURL
	}
	return &Service{resendClient: resendClient, config: config}
}

// Enabled reports whether all required delivery settings are present.
func (s *Service) Enabled() bool {
	return s != nil && s.config.From != "" && s.config.FrontendURL != "" &&
		(s.config.SMTPHost != "" || s.config.APIKey != "")
}

// Provider identifies the configured delivery transport for startup diagnostics.
func (s *Service) Provider() string {
	if s == nil || !s.Enabled() {
		return "disabled"
	}
	if s.config.SMTPHost != "" {
		return "smtp"
	}
	return "resend"
}

// SendPasswordReset sends a 15-minute password reset link to one user.
func (s *Service) SendPasswordReset(ctx context.Context, recipient, fullName, token string) error {
	return s.DeliverPasswordReset(ctx, notification.PasswordResetCommand{
		RecipientEmail: recipient, RecipientName: fullName, ResetToken: token,
	})
}

// DeliverPasswordReset renders and sends one validated password-reset command.
func (s *Service) DeliverPasswordReset(ctx context.Context, command notification.PasswordResetCommand) error {
	if !s.Enabled() {
		return errors.New("email delivery is not configured")
	}
	if err := command.Validate(); err != nil {
		return err
	}
	resetURL := s.config.FrontendURL + "/auth/reset-password?token=" + url.QueryEscape(command.ResetToken)
	name := displayName(command.RecipientName)
	subject := "Passwort zurücksetzen – Pehlione DemoBank"
	plain := fmt.Sprintf("Hallo %s,\n\nüber diesen Link können Sie Ihr Passwort innerhalb von 15 Minuten zurücksetzen:\n%s\n\nFalls Sie dies nicht angefordert haben, ignorieren Sie diese E-Mail.\n", name, resetURL)
	body := fmt.Sprintf(`<h2>Passwort zurücksetzen</h2><p>Hallo %s,</p><p>über den folgenden Link können Sie Ihr Passwort zurücksetzen. Der Link ist <strong>15 Minuten</strong> gültig und nur einmal verwendbar.</p><p><a href="%s" style="display:inline-block;padding:12px 20px;background:#004b80;color:#fff;text-decoration:none;border-radius:8px">Neues Passwort festlegen</a></p><p>Falls Sie dies nicht angefordert haben, ignorieren Sie diese E-Mail.</p>`, html.EscapeString(name), html.EscapeString(resetURL))
	return s.send(ctx, command.RecipientEmail, subject, body, plain)
}

// DeliverActivity renders and sends a fully resolved account-activity command.
func (s *Service) DeliverActivity(ctx context.Context, command notification.ActivityCommand) error {
	if !s.Enabled() {
		return errors.New("email delivery is not configured")
	}
	if err := command.Validate(); err != nil {
		return err
	}
	amount := localizedEUR(command.Amount)
	balance := localizedEUR(command.Balance)
	direction := "Belastung"
	sign := "−"
	if strings.EqualFold(command.Direction, "CREDIT") {
		direction = "Gutschrift"
		sign = "+"
	}
	details := ""
	if strings.TrimSpace(command.Counterparty) != "" {
		details += "<p><strong>Gegenpartei:</strong> " + html.EscapeString(command.Counterparty) + "</p>"
	}
	if strings.TrimSpace(command.Reference) != "" {
		details += "<p><strong>Verwendungszweck:</strong> " + html.EscapeString(command.Reference) + "</p>"
	}
	subject := fmt.Sprintf("%s %s%s auf Ihrem Konto", direction, sign, amount)
	body := fmt.Sprintf(`<h2>Neue Kontobewegung</h2><p>Hallo %s,</p><p>auf Ihrem Konto wurde eine neue Buchung ausgeführt.</p><div style="padding:16px;background:#f1f5f9;border-radius:10px"><p><strong>%s:</strong> %s%s</p><p><strong>Konto:</strong> %s (%s)</p><p><strong>Neuer Saldo:</strong> %s</p>%s</div><p>Wenn Sie diese Aktivität nicht erkennen, melden Sie sich bitte umgehend bei Ihrem Administrator.</p>`,
		html.EscapeString(displayName(command.RecipientName)), html.EscapeString(direction), sign, html.EscapeString(amount),
		html.EscapeString(command.AccountName), html.EscapeString(command.MaskedIBAN), html.EscapeString(balance), details)
	plain := fmt.Sprintf("Hallo %s,\n\nNeue Kontobewegung: %s %s%s\nKonto: %s (%s)\nNeuer Saldo: %s\n", displayName(command.RecipientName), direction, sign, amount, command.AccountName, command.MaskedIBAN, balance)
	return s.send(ctx, command.RecipientEmail, subject, body, plain)
}

func (s *Service) send(ctx context.Context, recipient, subject, htmlBody, textBody string) error {
	if s.config.SMTPHost != "" {
		return s.sendSMTP(ctx, recipient, subject, htmlBody, textBody)
	}
	response, err := s.resendClient.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    s.config.From,
		To:      []string{recipient},
		Subject: subject,
		Html:    htmlBody,
		Text:    textBody,
	})
	if err != nil {
		return fmt.Errorf("send email with Resend: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Id) == "" {
		return errors.New("resend returned an empty email ID")
	}
	return nil
}

func resendBaseURL(endpoint string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	baseURL = strings.TrimSuffix(baseURL, "/emails")
	return baseURL + "/"
}

func localizedEUR(value string) string {
	amount, err := decimal.NewFromString(value)
	if err != nil {
		return value + " EUR"
	}
	parts := strings.Split(amount.StringFixed(2), ".")
	return parts[0] + "," + parts[1] + " EUR"
}

func displayName(value string) string {
	if name := strings.TrimSpace(value); name != "" {
		return name
	}
	return "Kundin/Kunde"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

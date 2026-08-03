// Package email sends transactional email through the Resend HTTPS API.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	"github.com/mustafa-oezdemir/banking_go/internal/db"
	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
)

const (
	defaultResendEndpoint = "https://api.resend.com/emails"
	defaultFromAddress    = "Pehlione DemoBank <banking@pehlione.com>"
	defaultFrontendURL    = "https://pehlione-banking-frontend.onrender.com"
	activityQueueSize     = 100
)

// Config contains Resend delivery settings.
type Config struct {
	APIKey      string
	From        string
	FrontendURL string
	Endpoint    string
}

// Service delivers password-reset messages synchronously and account activity
// through a small non-blocking queue.
type Service struct {
	store  *db.Store
	client *http.Client
	queue  chan notification.Activity
	config Config
}

// NewFromEnvironment constructs a Resend client from runtime configuration.
func NewFromEnvironment(store *db.Store) *Service {
	from := firstNonEmpty(os.Getenv("MAIL_FROM"), os.Getenv("RESEND_FROM_EMAIL"), defaultFromAddress)
	frontendURL := firstNonEmpty(os.Getenv("FRONTEND_URL"), defaultFrontendURL)
	return NewService(store, Config{
		APIKey:      strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
		From:        from,
		FrontendURL: frontendURL,
		Endpoint:    defaultResendEndpoint,
	}, &http.Client{Timeout: 10 * time.Second})
}

// NewService constructs a configurable service, primarily for tests.
func NewService(store *db.Store, config Config, client *http.Client) *Service {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.From = strings.TrimSpace(config.From)
	config.FrontendURL = strings.TrimRight(strings.TrimSpace(config.FrontendURL), "/")
	config.Endpoint = strings.TrimSpace(config.Endpoint)
	if config.Endpoint == "" {
		config.Endpoint = defaultResendEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	service := &Service{
		store: store, client: client, config: config,
		queue: make(chan notification.Activity, activityQueueSize),
	}
	if service.Enabled() {
		go service.runActivityWorker()
	}
	return service
}

// Enabled reports whether all required delivery settings are present.
func (s *Service) Enabled() bool {
	return s != nil && s.config.APIKey != "" && s.config.From != "" && s.config.FrontendURL != ""
}

// SendPasswordReset sends a 15-minute password reset link to one user.
func (s *Service) SendPasswordReset(ctx context.Context, recipient, fullName, token string) error {
	if !s.Enabled() {
		return errors.New("email delivery is not configured")
	}
	resetURL := s.config.FrontendURL + "/auth/reset-password?token=" + url.QueryEscape(token)
	name := displayName(fullName)
	subject := "Passwort zurücksetzen – Pehlione DemoBank"
	plain := fmt.Sprintf("Hallo %s,\n\nüber diesen Link können Sie Ihr Passwort innerhalb von 15 Minuten zurücksetzen:\n%s\n\nFalls Sie dies nicht angefordert haben, ignorieren Sie diese E-Mail.\n", name, resetURL)
	body := fmt.Sprintf(`<h2>Passwort zurücksetzen</h2><p>Hallo %s,</p><p>über den folgenden Link können Sie Ihr Passwort zurücksetzen. Der Link ist <strong>15 Minuten</strong> gültig und nur einmal verwendbar.</p><p><a href="%s" style="display:inline-block;padding:12px 20px;background:#004b80;color:#fff;text-decoration:none;border-radius:8px">Neues Passwort festlegen</a></p><p>Falls Sie dies nicht angefordert haben, ignorieren Sie diese E-Mail.</p>`, html.EscapeString(name), html.EscapeString(resetURL))
	return s.send(ctx, recipient, subject, body, plain)
}

// NotifyActivity queues a committed account movement for email delivery.
func (s *Service) NotifyActivity(activity notification.Activity) {
	if !s.Enabled() || activity.UserID == uuid.Nil || activity.AccountID == uuid.Nil {
		return
	}
	select {
	case s.queue <- activity:
	default:
		log.Warn().Str("account_id", activity.AccountID.String()).Msg("Account email notification queue is full")
	}
}

func (s *Service) runActivityWorker() {
	for activity := range s.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		err := s.sendActivity(ctx, activity)
		cancel()
		if err != nil {
			log.Error().Err(err).Str("account_id", activity.AccountID.String()).Msg("Account activity email failed")
		}
	}
}

func (s *Service) sendActivity(ctx context.Context, activity notification.Activity) error {
	if s.store == nil {
		return errors.New("email account lookup is not configured")
	}
	user, err := s.store.GetUserByID(ctx, activity.UserID)
	if err != nil {
		return fmt.Errorf("load notification user: %w", err)
	}
	account, err := s.store.GetAccount(ctx, activity.AccountID)
	if err != nil {
		return fmt.Errorf("load notification account: %w", err)
	}
	amount := localizedEUR(activity.Amount)
	balance := localizedEUR(account.Balance)
	direction := "Belastung"
	sign := "−"
	if strings.EqualFold(activity.Direction, "CREDIT") {
		direction = "Gutschrift"
		sign = "+"
	}
	details := ""
	if strings.TrimSpace(activity.Counterparty) != "" {
		details += "<p><strong>Gegenpartei:</strong> " + html.EscapeString(activity.Counterparty) + "</p>"
	}
	if strings.TrimSpace(activity.Reference) != "" {
		details += "<p><strong>Verwendungszweck:</strong> " + html.EscapeString(activity.Reference) + "</p>"
	}
	subject := fmt.Sprintf("%s %s%s auf Ihrem Konto", direction, sign, amount)
	body := fmt.Sprintf(`<h2>Neue Kontobewegung</h2><p>Hallo %s,</p><p>auf Ihrem Konto wurde eine neue Buchung ausgeführt.</p><div style="padding:16px;background:#f1f5f9;border-radius:10px"><p><strong>%s:</strong> %s%s</p><p><strong>Konto:</strong> %s (%s)</p><p><strong>Neuer Saldo:</strong> %s</p>%s</div><p>Wenn Sie diese Aktivität nicht erkennen, melden Sie sich bitte umgehend bei Ihrem Administrator.</p>`,
		html.EscapeString(displayName(user.FullName)), html.EscapeString(direction), sign, html.EscapeString(amount),
		html.EscapeString(account.Name), html.EscapeString(sepa.MaskIBAN(account.Iban)), html.EscapeString(balance), details)
	plain := fmt.Sprintf("Hallo %s,\n\nNeue Kontobewegung: %s %s%s\nKonto: %s (%s)\nNeuer Saldo: %s\n", displayName(user.FullName), direction, sign, amount, account.Name, sepa.MaskIBAN(account.Iban), balance)
	return s.send(ctx, user.Email, subject, body, plain)
}

func (s *Service) send(ctx context.Context, recipient, subject, htmlBody, textBody string) error {
	payload, err := json.Marshal(map[string]any{
		"from": s.config.From, "to": []string{recipient}, "subject": subject,
		"html": htmlBody, "text": textBody,
	})
	if err != nil {
		return fmt.Errorf("encode Resend request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create Resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send Resend request: %w", err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close Resend response")
		}
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if _, drainErr := io.Copy(io.Discard, io.LimitReader(response.Body, 2048)); drainErr != nil {
			return fmt.Errorf("resend returned status %d", response.StatusCode)
		}
		return fmt.Errorf("resend returned status %d", response.StatusCode)
	}
	return nil
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

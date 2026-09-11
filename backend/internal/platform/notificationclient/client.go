// Package notificationclient sends explicit notification commands from the
// Banking API to the independently deployed Notification service.
package notificationclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/mustafa-oezdemir/banking_go/internal/account"
	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	db "github.com/mustafa-oezdemir/banking_go/internal/platform/database"
)

const defaultTimeout = 4 * time.Second

// Config contains Banking-side Notification service connection settings.
type Config struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// ActivityRecipient is the Banking-owned data required to render an activity notification.
type ActivityRecipient struct {
	Email       string
	FullName    string
	AccountName string
	MaskedIBAN  string
	Balance     string
}

// ActivityDirectory resolves private Banking data before a command crosses the service boundary.
type ActivityDirectory interface {
	ResolveActivityRecipient(context.Context, uuid.UUID, uuid.UUID) (ActivityRecipient, error)
}

// Client implements notification.Sender over a bounded synchronous HTTP call.
type Client struct {
	httpClient *http.Client
	directory  ActivityDirectory
	baseURL    string
	token      string
}

// NewFromEnvironment constructs the Banking-side Notification service client.
func NewFromEnvironment(store *db.Store) (*Client, error) {
	return New(Config{
		BaseURL: os.Getenv("NOTIFICATION_SERVICE_URL"),
		Token:   os.Getenv("NOTIFICATION_SERVICE_TOKEN"),
		Timeout: defaultTimeout,
	}, databaseDirectory{store: store}, nil)
}

// New constructs a client. URL and token must either both be configured or both be empty.
func New(config Config, directory ActivityDirectory, httpClient *http.Client) (*Client, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.Token = strings.TrimSpace(config.Token)
	if config.BaseURL == "" && config.Token == "" {
		return &Client{}, nil
	}
	if config.BaseURL == "" || config.Token == "" {
		return nil, errors.New("notification service URL and token must be configured together")
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("notification service URL must be an absolute HTTP(S) URL")
	}
	if len(config.Token) < 32 {
		return nil, errors.New("notification service token must contain at least 32 characters")
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.Timeout}
	}
	return &Client{
		httpClient: httpClient, directory: directory, baseURL: config.BaseURL, token: config.Token,
	}, nil
}

// Enabled reports whether the remote Notification service is configured.
func (client *Client) Enabled() bool {
	return client != nil && client.baseURL != "" && client.token != "" && client.httpClient != nil
}

// SendPasswordReset sends an explicit reset command without exposing Banking persistence.
func (client *Client) SendPasswordReset(ctx context.Context, email, fullName, token string) error {
	// A blank URL/token pair deliberately preserves the previous local no-op
	// notification mode. This keeps Banking usable when the optional service is
	// not deployed, while a partially configured client fails during startup.
	if !client.Enabled() {
		return nil
	}
	command := notification.PasswordResetCommand{
		RecipientEmail: email, RecipientName: fullName, ResetToken: token,
	}
	if err := command.Validate(); err != nil {
		return err
	}
	return client.post(ctx, "/v1/notifications/password-reset", command)
}

// NotifyActivity resolves private data inside Banking and sends an explicit activity command.
func (client *Client) NotifyActivity(ctx context.Context, activity notification.Activity) error {
	if !client.Enabled() {
		return nil
	}
	if client.directory == nil {
		return errors.New("notification activity directory is required")
	}
	if activity.UserID == uuid.Nil || activity.AccountID == uuid.Nil {
		return notification.ErrInvalidCommand
	}
	recipient, err := client.directory.ResolveActivityRecipient(ctx, activity.UserID, activity.AccountID)
	if err != nil {
		return fmt.Errorf("resolve notification recipient: %w", err)
	}
	command := notification.ActivityCommand{
		RecipientEmail: recipient.Email, RecipientName: recipient.FullName,
		AccountName: recipient.AccountName, MaskedIBAN: recipient.MaskedIBAN, Balance: recipient.Balance,
		Kind: activity.Kind, Direction: activity.Direction, Amount: activity.Amount, Currency: activity.Currency,
		Counterparty: activity.Counterparty, Reference: activity.Reference,
	}
	if err = command.Validate(); err != nil {
		return err
	}
	return client.post(ctx, "/v1/notifications/account-activity", command)
}

func (client *Client) post(ctx context.Context, path string, command any) error {
	if !client.Enabled() {
		return errors.New("notification service is not configured")
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("encode notification command: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create notification request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	requestID := middleware.GetReqID(ctx)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	request.Header.Set("X-Request-ID", requestID)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.Header))

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call notification service: %w", err)
	}
	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	closeErr := response.Body.Close()
	if readErr != nil {
		return fmt.Errorf("read notification response: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close notification response: %w", closeErr)
	}
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("notification service returned status %d", response.StatusCode)
	}
	return nil
}

type databaseDirectory struct {
	store *db.Store
}

func (directory databaseDirectory) ResolveActivityRecipient(
	ctx context.Context,
	userID, accountID uuid.UUID,
) (ActivityRecipient, error) {
	user, err := directory.store.GetUserByID(ctx, userID)
	if err != nil {
		return ActivityRecipient{}, err
	}
	bankAccount, err := directory.store.GetAccount(ctx, accountID)
	if err != nil {
		return ActivityRecipient{}, err
	}
	if bankAccount.IsSystem || !bankAccount.OwnerID.Valid || bankAccount.OwnerID.UUID != userID {
		return ActivityRecipient{}, errors.New("notification account ownership check failed")
	}
	return ActivityRecipient{
		Email: user.Email, FullName: user.FullName, AccountName: bankAccount.Name,
		MaskedIBAN: account.MaskIBAN(bankAccount.Iban), Balance: bankAccount.Balance,
	}, nil
}

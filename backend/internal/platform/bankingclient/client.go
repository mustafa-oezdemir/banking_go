// Package bankingclient implements the Identity-to-Banking provisioning command.
package bankingclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// Client sends the private idempotent customer-provisioning command.
type Client struct {
	baseURL, token string
	http           *http.Client
}

// New constructs a bounded authenticated client.
func New(baseURL, token string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	token = strings.TrimSpace(token)
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || len(token) < 32 {
		return nil, errors.New("valid Banking internal URL and token are required")
	}
	return &Client{baseURL: baseURL, token: token, http: &http.Client{Timeout: 5 * time.Second}}, nil
}

// ProvisionCustomer creates the Banking Customer with the Identity subject UUID.
func (client *Client) ProvisionCustomer(ctx context.Context, id uuid.UUID, email, fullName string) error {
	payload, err := json.Marshal(map[string]string{"identity_id": id.String(), "email": email, "full_name": fullName})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/internal/customers/provision", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Service-Token", client.token)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	res, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("call Banking provisioning: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Banking provisioning returned status %d", res.StatusCode)
	}
	return nil
}

// RevokeCustomerSessions synchronizes Identity's session generation change
// with the Banking authorization boundary after a password reset.
func (client *Client) RevokeCustomerSessions(ctx context.Context, id uuid.UUID) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/internal/customers/"+id.String()+"/sessions/revoke", http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Service-Token", client.token)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	res, err := client.http.Do(req)
	if err != nil {
		return fmt.Errorf("call Banking session revocation: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Banking session revocation returned status %d", res.StatusCode)
	}
	return nil
}

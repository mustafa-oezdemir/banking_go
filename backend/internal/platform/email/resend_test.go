package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
)

func TestSendPasswordResetUsesResendAPI(t *testing.T) {
	t.Parallel()

	type requestPayload struct {
		From    string   `json:"from"`
		Subject string   `json:"subject"`
		HTML    string   `json:"html"`
		Text    string   `json:"text"`
		To      []string `json:"to"`
	}
	received := make(chan requestPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/emails", r.URL.Path)
		assert.Equal(t, "Bearer re_test_secret", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var payload requestPayload
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		received <- payload
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"email_test_123"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	service := NewService(Config{
		APIKey: "re_test_secret", From: "Pehlione <banking@pehlione.com>",
		FrontendURL: "https://bank.example", Endpoint: server.URL,
	}, server.Client())
	require.NoError(t, service.SendPasswordReset(context.Background(), "owner@example.com", "Ada Beispiel", "safe_token"))

	payload := <-received
	assert.Equal(t, "Pehlione <banking@pehlione.com>", payload.From)
	assert.Equal(t, []string{"owner@example.com"}, payload.To)
	assert.Contains(t, payload.Subject, "Passwort")
	assert.Contains(t, payload.HTML, "https://bank.example/auth/reset-password?token=safe_token")
	assert.Contains(t, payload.HTML, "15 Minuten")
	assert.Contains(t, payload.Text, "15 Minuten")
	assert.False(t, strings.Contains(payload.HTML, "re_test_secret"))
}

func TestSendPasswordResetRejectsMissingResendMessageID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":""}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	service := NewService(Config{
		APIKey: "re_test_secret", From: "Pehlione <banking@pehlione.com>",
		FrontendURL: "https://bank.example", Endpoint: server.URL,
	}, server.Client())
	err := service.SendPasswordReset(context.Background(), "owner@example.com", "Ada Beispiel", "safe_token")
	require.EqualError(t, err, "resend returned an empty email ID")
}

func TestDeliverActivityUsesOnlyExplicitCommandData(t *testing.T) {
	t.Parallel()
	var delivered struct {
		HTML string   `json:"html"`
		To   []string `json:"to"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&delivered))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"email_activity_123"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	service := NewService(Config{
		APIKey: "re_test_secret", From: "Pehlione <banking@pehlione.com>",
		FrontendURL: "https://bank.example", Endpoint: server.URL,
	}, server.Client())

	err := service.DeliverActivity(t.Context(), notification.ActivityCommand{
		RecipientEmail: "owner@example.com", RecipientName: "Ada Beispiel", AccountName: "Girokonto",
		MaskedIBAN: "DE89 •••• •••• •••• ••00", Balance: "87.66", Kind: "TRANSFER_SENT",
		Direction: "DEBIT", Amount: "12.34", Currency: "EUR", Reference: "Miete",
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"owner@example.com"}, delivered.To)
	assert.Contains(t, delivered.HTML, "DE89 •••• •••• •••• ••00")
	assert.Contains(t, delivered.HTML, "87,66 EUR")
}

func TestResendBaseURLAcceptsLegacyEmailsEndpoint(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "https://api.resend.com/", resendBaseURL("https://api.resend.com/emails"))
	assert.Equal(t, "https://api.resend.com/", resendBaseURL("https://api.resend.com/"))
}

func TestDisabledServiceRejectsPasswordResetDelivery(t *testing.T) {
	t.Parallel()
	service := NewService(Config{}, nil)
	assert.False(t, service.Enabled())
	assert.Error(t, service.SendPasswordReset(context.Background(), "owner@example.com", "Owner", "token"))
}

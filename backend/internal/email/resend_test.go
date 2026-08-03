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
		assert.Equal(t, "Bearer re_test_secret", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var payload requestPayload
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		received <- payload
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	service := NewService(nil, Config{
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

func TestDisabledServiceRejectsPasswordResetDelivery(t *testing.T) {
	t.Parallel()
	service := NewService(nil, Config{}, nil)
	assert.False(t, service.Enabled())
	assert.Error(t, service.SendPasswordReset(context.Background(), "owner@example.com", "Owner", "token"))
}

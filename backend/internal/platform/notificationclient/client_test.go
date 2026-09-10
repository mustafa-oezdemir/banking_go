package notificationclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
)

const testServiceToken = "test-notification-service-token-32-bytes"

type directoryStub struct {
	recipient *ActivityRecipient
	err       error
}

func (directory directoryStub) ResolveActivityRecipient(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (ActivityRecipient, error) {
	if directory.recipient == nil {
		return ActivityRecipient{}, directory.err
	}
	return *directory.recipient, directory.err
}

func TestClientSendsExplicitActivityContract(t *testing.T) {
	t.Parallel()
	var received notification.ActivityCommand
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/v1/notifications/account-activity", request.URL.Path)
		assert.Equal(t, "Bearer "+testServiceToken, request.Header.Get("Authorization"))
		assert.NotEmpty(t, request.Header.Get("X-Request-ID"))
		require.NoError(t, json.NewDecoder(request.Body).Decode(&received))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL, Token: testServiceToken}, directoryStub{recipient: &ActivityRecipient{
		Email: "owner@example.com", FullName: "Ada Beispiel", AccountName: "Girokonto",
		MaskedIBAN: "DE89 •••• •••• •••• ••00", Balance: "87.6600",
	}}, server.Client())
	require.NoError(t, err)

	err = client.NotifyActivity(t.Context(), notification.Activity{
		UserID: uuid.New(), AccountID: uuid.New(), Kind: "SEPA_PAYMENT_SENT",
		Direction: "DEBIT", Amount: "12.34", Currency: "EUR", Reference: "Miete",
	})

	require.NoError(t, err)
	assert.Equal(t, "owner@example.com", received.RecipientEmail)
	assert.Equal(t, "87.6600", received.Balance)
	assert.Equal(t, "Miete", received.Reference)
}

func TestClientDoesNotRetryFailedNotification(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL, Token: testServiceToken}, directoryStub{}, server.Client())
	require.NoError(t, err)

	err = client.SendPasswordReset(t.Context(), "owner@example.com", "Ada Beispiel", "safe-token")

	require.Error(t, err)
	assert.EqualValues(t, 1, attempts.Load(), "unsafe delivery requests must not be retried")
}

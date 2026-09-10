package notificationapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
)

const testToken = "notification-api-contract-test-token"

type deliveryStub struct {
	activity      *notification.ActivityCommand
	passwordReset *notification.PasswordResetCommand
	err           error
}

func (delivery *deliveryStub) DeliverPasswordReset(_ context.Context, command notification.PasswordResetCommand) error {
	delivery.passwordReset = &command
	return delivery.err
}

func (delivery *deliveryStub) DeliverActivity(_ context.Context, command notification.ActivityCommand) error {
	delivery.activity = &command
	return delivery.err
}

func (*deliveryStub) Enabled() bool { return true }

func TestNotificationAPIContract(t *testing.T) {
	t.Parallel()
	delivery := &deliveryStub{}
	handler, err := New(delivery, Config{Token: testToken})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, "/v1/notifications/account-activity", strings.NewReader(`{
		"recipient_email":"owner@example.com","recipient_name":"Ada Beispiel","account_name":"Girokonto",
		"masked_iban":"DE89 •••• •••• •••• ••00","balance":"87.6600","kind":"SEPA_PAYMENT_SENT",
		"direction":"DEBIT","amount":"12.34","currency":"EUR","reference":"Miete"}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "banking-request-123")
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, "banking-request-123", response.Header().Get("X-Request-ID"))
	assert.Equal(t, "owner@example.com", delivery.activity.RecipientEmail)
	assert.Equal(t, "87.6600", delivery.activity.Balance)
}

func TestNotificationAPIRejectsUnauthorizedAndUnknownFields(t *testing.T) {
	t.Parallel()
	handler, err := New(&deliveryStub{}, Config{Token: testToken})
	require.NoError(t, err)

	unauthorized := httptest.NewRecorder()
	handler.Routes().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/notifications/password-reset", strings.NewReader(`{}`)))
	assert.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	request := httptest.NewRequest(http.MethodPost, "/v1/notifications/password-reset", strings.NewReader(
		`{"recipient_email":"owner@example.com","recipient_name":"Ada","reset_token":"safe","admin":true}`,
	))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	invalid := httptest.NewRecorder()
	handler.Routes().ServeHTTP(invalid, request)
	assert.Equal(t, http.StatusBadRequest, invalid.Code)
}

func TestNotificationAPIReturnsGenericProviderFailure(t *testing.T) {
	t.Parallel()
	handler, err := New(&deliveryStub{err: errors.New("smtp secret detail")}, Config{Token: testToken})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/v1/notifications/password-reset", strings.NewReader(
		`{"recipient_email":"owner@example.com","recipient_name":"Ada","reset_token":"safe"}`,
	))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadGateway, response.Code)
	assert.NotContains(t, response.Body.String(), "smtp secret detail")
}

func TestNotificationHealthIsPublic(t *testing.T) {
	t.Parallel()
	handler, err := New(&deliveryStub{}, Config{Token: testToken})
	require.NoError(t, err)
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, response.Code)
}

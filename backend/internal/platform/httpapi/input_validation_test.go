package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/payment"
)

func TestCreatePaymentRejectsMarkupWhenFrontendIsBypassed(t *testing.T) {
	require.NoError(t, InitTokenAuth("fV7sliKV3qn657I60wEFtw/Auk/0bNU9zdp30wFzfDg="))
	userID := uuid.New()
	token, err := GenerateTokenForVersion(userID, 0)
	require.NoError(t, err)
	handler := &Handler{payments: payment.NewService(nil, nil)}
	router := chi.NewRouter()
	router.Use(jwtauth.Verifier(TokenAuth))
	router.Use(jwtauth.Authenticator(TokenAuth))
	router.Post("/payments", handler.CreatePayment)

	request := httptest.NewRequest(http.MethodPost, "/payments", strings.NewReader(`{
		"source_account_id":"`+uuid.NewString()+`",
		"beneficiary_name":"<script>alert(1)</script>",
		"beneficiary_iban":"DE89370400440532013000",
		"amount":"1.00",
		"transfer_type":"STANDARD",
		"schedule_type":"IMMEDIATE"
	}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", "payment-http-validation-test")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

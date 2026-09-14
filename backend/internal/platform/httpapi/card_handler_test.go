package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sepa "github.com/mustafa-oezdemir/banking_go/internal/account"
	carddomain "github.com/mustafa-oezdemir/banking_go/internal/card"
	db "github.com/mustafa-oezdemir/banking_go/internal/platform/database"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

const cardTestDataKey = "0123456789abcdef0123456789abcdef"

func setupCardRouter(t *testing.T, handler *Handler) http.Handler {
	t.Helper()
	router := chi.NewRouter()
	router.Use(jwtauth.Verifier(TokenAuth))
	router.Use(jwtauth.Authenticator(TokenAuth))
	router.Use(RequireActiveSession(handler.store))
	router.Get("/cards", handler.ListCards)
	router.Get("/cards/{id}/credentials", handler.GetCardCredentials)
	return router
}

func createCardTestCustomer(t *testing.T, handler *Handler) (uuid.UUID, string, sqlc.Account) {
	t.Helper()
	user, err := handler.store.CreateUser(t.Context(), sqlc.CreateUserParams{
		Email:          "cards-" + uuid.NewString() + "@example.com",
		HashedPassword: "test-only",
		FullName:       "Card Test Customer",
	})
	require.NoError(t, err)
	iban, err := sepa.GenerateGermanDemoIBAN()
	require.NoError(t, err)
	account, err := handler.store.CreateAccount(t.Context(), sqlc.CreateAccountParams{
		OwnerID:     uuid.NullUUID{UUID: user.ID, Valid: true},
		Name:        "Card Account",
		Currency:    "EUR",
		Iban:        iban,
		AccountType: "GIROKONTO",
		Status:      "ACTIVE",
	})
	require.NoError(t, err)
	return user.ID, issueBankingTestToken(t, user.ID, 0), account
}

func createAdditionalCardAccount(t *testing.T, handler *Handler, ownerID uuid.UUID) sqlc.Account {
	t.Helper()
	iban, err := sepa.GenerateGermanDemoIBAN()
	require.NoError(t, err)
	account, err := handler.store.CreateAccount(t.Context(), sqlc.CreateAccountParams{
		OwnerID:     uuid.NullUUID{UUID: ownerID, Valid: true},
		Name:        "Legacy Card Account",
		Currency:    "EUR",
		Iban:        iban,
		AccountType: "GIROKONTO",
		Status:      "ACTIVE",
	})
	require.NoError(t, err)
	return account
}

func TestCardCredentialRevealFlow(t *testing.T) {
	t.Setenv("CARD_DATA_KEY", cardTestDataKey)
	handler := setupTestHandler(t)
	require.NoError(t, InitTokenAuth("fV7sliKV3qn657I60wEFtw/Auk/0bNU9zdp30wFzfDg="))
	router := setupCardRouter(t, handler)

	ownerID, ownerToken, account := createCardTestCustomer(t, handler)
	_, otherToken, _ := createCardTestCustomer(t, handler)

	issued, err := handler.cards.Issue(t.Context(), ownerID, account.ID)
	require.NoError(t, err)
	secondIssued, err := handler.cards.Issue(t.Context(), ownerID, account.ID)
	require.NoError(t, err)
	assert.NotEqual(t, issued.ID, secondIssued.ID)
	assert.NotEqual(t, issued.CardNumber, secondIssued.CardNumber)
	revealedByService, err := handler.cards.Reveal(t.Context(), ownerID, issued.ID)
	require.NoError(t, err)
	if issued.CardNumber != revealedByService.CardNumber || issued.CVC != revealedByService.CVC {
		t.Fatal("Issue and Reveal must return the same derived credentials")
	}

	revealPath := "/cards/" + issued.ID.String() + "/credentials"
	ownerResponse := performJSONRequest(t, router, ownerToken, http.MethodGet, revealPath, nil)
	require.Equal(t, http.StatusOK, ownerResponse.Code, ownerResponse.Body.String())
	assert.Equal(t, "no-store", ownerResponse.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", ownerResponse.Header().Get("Pragma"))
	var revealed carddomain.RevealedCard
	require.NoError(t, json.NewDecoder(ownerResponse.Body).Decode(&revealed))
	if revealed.CardNumber != issued.CardNumber || revealed.CVC != issued.CVC {
		t.Fatal("owner endpoint did not return the issued credentials")
	}

	otherOwnerResponse := performJSONRequest(t, router, otherToken, http.MethodGet, revealPath, nil)
	require.Equal(t, http.StatusNotFound, otherOwnerResponse.Code)
	assert.Equal(t, "no-store", otherOwnerResponse.Header().Get("Cache-Control"))
	assert.NotContains(t, otherOwnerResponse.Body.String(), issued.CardNumber)
	assert.NotContains(t, otherOwnerResponse.Body.String(), issued.CVC)

	invalidIDResponse := performJSONRequest(t, router, ownerToken, http.MethodGet, "/cards/not-a-uuid/credentials", nil)
	require.Equal(t, http.StatusNotFound, invalidIDResponse.Code)
	assert.Equal(t, "no-store", invalidIDResponse.Header().Get("Cache-Control"))

	listResponse := performJSONRequest(t, router, ownerToken, http.MethodGet, "/cards", nil)
	require.Equal(t, http.StatusOK, listResponse.Code, listResponse.Body.String())
	var listed []map[string]any
	require.NoError(t, json.NewDecoder(listResponse.Body).Decode(&listed))
	require.NotEmpty(t, listed)
	for _, listedCard := range listed {
		assert.NotContains(t, listedCard, "card_number")
		assert.NotContains(t, listedCard, "cvc")
		assert.NotContains(t, listedCard, "pan_fingerprint")
		assert.NotContains(t, listedCard, "cvc_hash")
		assert.NotContains(t, listedCard, "credential_version")
	}

	legacyAccount := createAdditionalCardAccount(t, handler, ownerID)
	legacyID := uuid.New()
	_, err = handler.store.CreatePaymentCard(t.Context(), db.PaymentCard{
		ID:                legacyID,
		OwnerID:           ownerID,
		AccountID:         legacyAccount.ID,
		PANFingerprint:    []byte(uuid.NewString()),
		LastFour:          "9999",
		Brand:             "visa",
		ExpMonth:          12,
		ExpYear:           time.Now().UTC().Year() + 3,
		CVCHash:           strings.Repeat("x", 60),
		CredentialVersion: 0,
	})
	require.NoError(t, err)
	legacyResponse := performJSONRequest(t, router, ownerToken, http.MethodGet, "/cards/"+legacyID.String()+"/credentials", nil)
	require.Equal(t, http.StatusNotFound, legacyResponse.Code)
	assert.JSONEq(t, `{"error":"card credentials are unavailable"}`, legacyResponse.Body.String())
}

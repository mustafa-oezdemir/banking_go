package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayRoutesIdentityAndBanking(t *testing.T) {
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/login", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer identity.Close()
	banking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/accounts", r.URL.Path)
		assert.Empty(t, r.Header.Get("X-Internal-Service-Token"))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer banking.Close()
	gate, err := New(Config{IdentityURL: identity.URL, BankingURL: banking.URL})
	require.NoError(t, err)
	login := httptest.NewRecorder()
	gate.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/login", nil))
	assert.Equal(t, http.StatusNoContent, login.Code)
	assert.NotEmpty(t, login.Header().Get("X-Request-ID"))
	accounts := httptest.NewRecorder()
	accountsRequest := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	accountsRequest.Header.Set("X-Internal-Service-Token", "untrusted-browser-value")
	gate.Handler().ServeHTTP(accounts, accountsRequest)
	assert.Equal(t, http.StatusAccepted, accounts.Code)
}

func TestGatewayIdentityOutageDoesNotBlockBankingRoute(t *testing.T) {
	banking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/accounts", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer banking.Close()

	gate, err := New(Config{IdentityURL: "http://127.0.0.1:1", BankingURL: banking.URL})
	require.NoError(t, err)

	login := httptest.NewRecorder()
	gate.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/login", nil))
	assert.Equal(t, http.StatusBadGateway, login.Code)

	accounts := httptest.NewRecorder()
	gate.Handler().ServeHTTP(accounts, httptest.NewRequest(http.MethodGet, "/accounts", nil))
	assert.Equal(t, http.StatusNoContent, accounts.Code)
}

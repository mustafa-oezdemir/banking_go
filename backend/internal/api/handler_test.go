package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/db"
	"github.com/mustafa-oezdemir/banking_go/internal/service"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

func setupTestHandler(t *testing.T) *Handler {
	// Keep test database configuration separate from the application database.
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DB_URL")
	}
	if dbURL == "" {
		dbURL = "postgresql://root:secret@localhost:5433/simple_ledger?sslmode=disable"
	}
	sqlDB, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	store := db.NewStore(sqlDB)
	ledger := service.NewLedgerService(store)
	return NewHandler(ledger, store)
}

func TestRegisterHandler_BadRequest(t *testing.T) {
	// Missing request body should trigger 400 validation response.
	h := setupTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/register", nil)
	rw := httptest.NewRecorder()
	h.Register(rw, req)
	assert.Equal(t, http.StatusBadRequest, rw.Code)
}

func TestRegisterHandler_Success(t *testing.T) {
	h := setupTestHandler(t)
	require.NoError(t, InitTokenAuth("fV7sliKV3qn657I60wEFtw/Auk/0bNU9zdp30wFzfDg="))

	// Use a unique email per run to avoid DB uniqueness collisions.
	email := "testuser_" + uuid.New().String() + "@example.com"
	body := map[string]string{"email": email, "password": "testpassword123"}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(b))
	rw := httptest.NewRecorder()

	h.Register(rw, req)
	assert.Equal(t, http.StatusCreated, rw.Code)

	cookies := rw.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, sessionCookieName, cookies[0].Name)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
}

func TestRegisterHandler_RejectsWeakPassword(t *testing.T) {
	h := setupTestHandler(t)
	body := map[string]string{"email": "weak@example.com", "password": "password"}
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(payload))
	rw := httptest.NewRecorder()
	h.Register(rw, req)
	assert.Equal(t, http.StatusBadRequest, rw.Code)
}

func TestValidateAccountName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		wantError bool
	}{
		{name: "trims whitespace", input: "  Savings  ", expected: "Savings"},
		{name: "rejects empty", input: "   ", wantError: true},
		{name: "accepts unicode", input: "Özel Hesap", expected: "Özel Hesap"},
		{name: "rejects long names", input: strings.Repeat("a", maxAccountNameLength+1), wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := validateAccountName(test.input)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func setupAccountRouter(t *testing.T, h *Handler) http.Handler {
	t.Helper()

	router := chi.NewRouter()
	router.Use(jwtauth.Verifier(TokenAuth))
	router.Use(jwtauth.Authenticator(TokenAuth))
	router.Post("/accounts", h.CreateAccount)
	router.Get("/accounts/{id}", h.GetAccount)
	router.Put("/accounts/{id}", h.UpdateAccount)
	router.Delete("/accounts/{id}", h.DeleteAccount)
	return router
}

func createTestUserToken(t *testing.T, h *Handler) string {
	t.Helper()

	user, err := h.store.CreateUser(t.Context(), sqlc.CreateUserParams{
		Email:          "crud_" + uuid.New().String() + "@example.com",
		HashedPassword: "not-used-by-this-test",
	})
	require.NoError(t, err)

	token, err := GenerateToken(user.ID)
	require.NoError(t, err)
	return token
}

func performJSONRequest(
	t *testing.T,
	handler http.Handler,
	token string,
	method string,
	path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	return rw
}

func TestAccountCRUDAndOwnership(t *testing.T) {
	h := setupTestHandler(t)
	require.NoError(t, InitTokenAuth("fV7sliKV3qn657I60wEFtw/Auk/0bNU9zdp30wFzfDg="))
	router := setupAccountRouter(t, h)
	ownerToken := createTestUserToken(t, h)
	otherUserToken := createTestUserToken(t, h)

	createResponse := performJSONRequest(
		t,
		router,
		ownerToken,
		http.MethodPost,
		"/accounts",
		map[string]string{"name": "  Main Account  "},
	)
	require.Equal(t, http.StatusCreated, createResponse.Code)
	var account AccountResponse
	require.NoError(t, json.NewDecoder(createResponse.Body).Decode(&account))
	assert.Equal(t, "Main Account", account.Name)

	accountPath := "/accounts/" + account.ID
	readResponse := performJSONRequest(t, router, ownerToken, http.MethodGet, accountPath, nil)
	require.Equal(t, http.StatusOK, readResponse.Code)

	forbiddenRead := performJSONRequest(t, router, otherUserToken, http.MethodGet, accountPath, nil)
	require.Equal(t, http.StatusForbidden, forbiddenRead.Code)
	forbiddenUpdate := performJSONRequest(
		t,
		router,
		otherUserToken,
		http.MethodPut,
		accountPath,
		map[string]string{"name": "Stolen Account"},
	)
	require.Equal(t, http.StatusForbidden, forbiddenUpdate.Code)
	forbiddenDelete := performJSONRequest(t, router, otherUserToken, http.MethodDelete, accountPath, nil)
	require.Equal(t, http.StatusForbidden, forbiddenDelete.Code)

	updateResponse := performJSONRequest(
		t,
		router,
		ownerToken,
		http.MethodPut,
		accountPath,
		map[string]string{"name": "Emergency Fund"},
	)
	require.Equal(t, http.StatusOK, updateResponse.Code)
	var updatedAccount AccountResponse
	require.NoError(t, json.NewDecoder(updateResponse.Body).Decode(&updatedAccount))
	assert.Equal(t, "Emergency Fund", updatedAccount.Name)

	deleteResponse := performJSONRequest(t, router, ownerToken, http.MethodDelete, accountPath, nil)
	require.Equal(t, http.StatusOK, deleteResponse.Code)

	missingResponse := performJSONRequest(t, router, ownerToken, http.MethodGet, accountPath, nil)
	require.Equal(t, http.StatusNotFound, missingResponse.Code)
}

// Add more handler tests as needed (mock dependencies for full coverage)

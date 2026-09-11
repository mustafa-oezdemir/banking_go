package identityapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/identity"
)

const identityTestSecret = "a-32-character-identity-test-signing-secret"

type memoryStore struct {
	users map[string]identity.LoginAccount
}

func (store *memoryStore) FindLoginAccount(_ context.Context, email string) (identity.LoginAccount, bool, error) {
	account, ok := store.users[email]
	return account, ok, nil
}
func (*memoryStore) SessionVersion(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (store *memoryStore) Register(_ context.Context, email, name, hash string) (uuid.UUID, error) {
	if _, exists := store.users[email]; exists {
		return uuid.Nil, errors.New("duplicate")
	}
	id := uuid.New()
	store.users[email] = identity.LoginAccount{ID: id, Email: email, HashedPassword: hash}
	return id, nil
}
func (store *memoryStore) Delete(_ context.Context, id uuid.UUID) error {
	for email, user := range store.users {
		if user.ID == id {
			delete(store.users, email)
		}
	}
	return nil
}
func (store *memoryStore) GetByEmail(_ context.Context, email string) (uuid.UUID, string, bool, error) {
	user, ok := store.users[email]
	return user.ID, "Ada Example", ok, nil
}
func (*memoryStore) CreatePasswordReset(context.Context, uuid.UUID, []byte, time.Time) error {
	return nil
}
func (*memoryStore) ResetPassword(context.Context, []byte, string, time.Time) error { return nil }

type provisionerStub struct{ calls int }

func (stub *provisionerStub) ProvisionCustomer(context.Context, uuid.UUID, string, string) error {
	stub.calls++
	return nil
}

type notifierStub struct{}

func (notifierStub) SendPasswordReset(context.Context, string, string, string) error { return nil }

func jsonRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestIdentityRegisterLoginAndInvalidCredentials(t *testing.T) {
	store := &memoryStore{users: map[string]identity.LoginAccount{}}
	provisioner := &provisionerStub{}
	handler, err := New(store, identityTestSecret, provisioner, notifierStub{})
	require.NoError(t, err)
	register := httptest.NewRecorder()
	handler.Routes().ServeHTTP(register, jsonRequest(http.MethodPost, "/register", `{"email":"ada@example.test","password":"IdentityPhase5!Pass","full_name":"Ada Example"}`))
	require.Equal(t, http.StatusCreated, register.Code)
	assert.Equal(t, 1, provisioner.calls)
	require.NotEmpty(t, register.Result().Cookies())
	invalid := httptest.NewRecorder()
	handler.Routes().ServeHTTP(invalid, jsonRequest(http.MethodPost, "/login", `{"email":"ada@example.test","password":"wrong"}`))
	assert.Equal(t, http.StatusUnauthorized, invalid.Code)
	login := httptest.NewRecorder()
	handler.Routes().ServeHTTP(login, jsonRequest(http.MethodPost, "/login", `{"email":"ada@example.test","password":"IdentityPhase5!Pass"}`))
	assert.Equal(t, http.StatusOK, login.Code)
	assert.Equal(t, sessionCookie, login.Result().Cookies()[0].Name)
}

func TestDecodeRejectsUnknownAndTrailingJSON(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field":  `{"email":"ada@example.test","password":"IdentityPhase5!Pass","admin":true}`,
		"trailing value": `{"email":"ada@example.test","password":"IdentityPhase5!Pass"}{}`,
	} {
		t.Run(name, func(t *testing.T) {
			var input struct {
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			require.Error(t, decode(httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body)), &input))
		})
	}
}

func TestIdentityAPIRejectsNonJSONAndMarkupRegistration(t *testing.T) {
	handler, err := New(&memoryStore{users: map[string]identity.LoginAccount{}}, identityTestSecret, &provisionerStub{}, notifierStub{})
	require.NoError(t, err)

	nonJSON := httptest.NewRecorder()
	handler.Routes().ServeHTTP(nonJSON, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`email=ada@example.test`)))
	assert.Equal(t, http.StatusUnsupportedMediaType, nonJSON.Code)

	markup := httptest.NewRecorder()
	handler.Routes().ServeHTTP(markup, jsonRequest(http.MethodPost, "/register", `{"email":"ada@example.test","password":"IdentityPhase5!Pass","full_name":"<script>alert(1)</script>"}`))
	assert.Equal(t, http.StatusBadRequest, markup.Code)
}

package identityapi

import (
	"context"
	"encoding/base64"
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
func (store *memoryStore) ResetPassword(context.Context, []byte, string, time.Time) (uuid.UUID, error) {
	for _, user := range store.users {
		return user.ID, nil
	}
	return uuid.Nil, errors.New("reset token not found")
}

type provisionerStub struct {
	calls   int
	revoked []uuid.UUID
}

func (stub *provisionerStub) ProvisionCustomer(context.Context, uuid.UUID, string, string) error {
	stub.calls++
	return nil
}

func (stub *provisionerStub) RevokeCustomerSessions(_ context.Context, id uuid.UUID) error {
	stub.revoked = append(stub.revoked, id)
	return nil
}

type notifierStub struct{}

func (notifierStub) SendPasswordReset(context.Context, string, string, string) error { return nil }

func TestIdentityRegisterLoginAndInvalidCredentials(t *testing.T) {
	store := &memoryStore{users: map[string]identity.LoginAccount{}}
	provisioner := &provisionerStub{}
	handler, err := New(store, identityTestSecret, provisioner, notifierStub{})
	require.NoError(t, err)
	register := httptest.NewRecorder()
	handler.Routes().ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"email":"ada@example.test","password":"IdentityPhase5!Pass","full_name":"Ada Example"}`)))
	require.Equal(t, http.StatusCreated, register.Code)
	assert.Equal(t, 1, provisioner.calls)
	require.NotEmpty(t, register.Result().Cookies())
	invalid := httptest.NewRecorder()
	handler.Routes().ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"ada@example.test","password":"wrong"}`)))
	assert.Equal(t, http.StatusUnauthorized, invalid.Code)
	login := httptest.NewRecorder()
	handler.Routes().ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"ada@example.test","password":"IdentityPhase5!Pass"}`)))
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

type resetNotifierStub struct{ tokens []string }

func (stub *resetNotifierStub) SendPasswordReset(_ context.Context, _, _, token string) error {
	stub.tokens = append(stub.tokens, token)
	return nil
}

func TestForgotPasswordKnownAndUnknownReturnSameResponseBeforeWork(t *testing.T) {
	knownID := uuid.New()
	knownStore := &memoryStore{users: map[string]identity.LoginAccount{"known@example.test": {ID: knownID, Email: "known@example.test"}}}
	unknownStore := &memoryStore{users: map[string]identity.LoginAccount{}}
	knownNotifier, unknownNotifier := &resetNotifierStub{}, &resetNotifierStub{}
	known, err := New(knownStore, identityTestSecret, &provisionerStub{}, knownNotifier)
	require.NoError(t, err)
	unknown, err := New(unknownStore, identityTestSecret, &provisionerStub{}, unknownNotifier)
	require.NoError(t, err)
	knownJobs, unknownJobs := []func(){}, []func(){}
	known.dispatch = func(job func()) { knownJobs = append(knownJobs, job) }
	unknown.dispatch = func(job func()) { unknownJobs = append(unknownJobs, job) }

	knownResponse := httptest.NewRecorder()
	known.Routes().ServeHTTP(knownResponse, httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(`{"email":"known@example.test"}`)))
	unknownResponse := httptest.NewRecorder()
	unknown.Routes().ServeHTTP(unknownResponse, httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(`{"email":"unknown@example.test"}`)))

	assert.Equal(t, http.StatusAccepted, knownResponse.Code)
	assert.Equal(t, knownResponse.Body.String(), unknownResponse.Body.String())
	assert.Empty(t, knownNotifier.tokens)
	require.Len(t, knownJobs, 1)
	require.Len(t, unknownJobs, 1)
	knownJobs[0]()
	unknownJobs[0]()
	require.Len(t, knownNotifier.tokens, 1)
	decoded, err := base64.RawURLEncoding.DecodeString(knownNotifier.tokens[0])
	require.NoError(t, err)
	assert.Len(t, decoded, 32)
	assert.Empty(t, unknownNotifier.tokens)
}

func TestResetPasswordRevokesBankingSessions(t *testing.T) {
	userID := uuid.New()
	store := &memoryStore{users: map[string]identity.LoginAccount{"known@example.test": {ID: userID, Email: "known@example.test"}}}
	provisioner := &provisionerStub{}
	handler, err := New(store, identityTestSecret, provisioner, notifierStub{})
	require.NoError(t, err)
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(`{"token":"`+token+`","new_password":"UniqueResetPassword2026!"}`)))
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, []uuid.UUID{userID}, provisioner.revoked)
}

func TestPasswordResetRateLimiter(t *testing.T) {
	handler := newIPRateLimiter(1, time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	for attempt, expected := range []int{http.StatusAccepted, http.StatusTooManyRequests} {
		request := httptest.NewRequest(http.MethodPost, "/forgot-password", nil)
		request.RemoteAddr = "192.0.2.10:1234"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, expected, response.Code, "attempt %d", attempt+1)
	}
}

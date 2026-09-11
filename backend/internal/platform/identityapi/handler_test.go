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

	"github.com/go-chi/jwtauth/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/identity"
)

const identityTestSecret = "a-32-character-identity-test-signing-secret"

type memoryStore struct {
	users    map[string]identity.LoginAccount
	versions map[uuid.UUID]int64
}

func (store *memoryStore) FindLoginAccount(_ context.Context, email string) (identity.LoginAccount, bool, error) {
	account, ok := store.users[email]
	return account, ok, nil
}
func (store *memoryStore) SessionVersion(_ context.Context, userID uuid.UUID) (int64, error) {
	version, ok := store.versions[userID]
	if !ok {
		return 0, errors.New("identity not found")
	}
	return version, nil
}
func (store *memoryStore) Register(_ context.Context, email, name, hash string) (uuid.UUID, error) {
	if _, exists := store.users[email]; exists {
		return uuid.Nil, errors.New("duplicate")
	}
	id := uuid.New()
	store.users[email] = identity.LoginAccount{ID: id, Email: email, HashedPassword: hash}
	if store.versions == nil {
		store.versions = make(map[uuid.UUID]int64)
	}
	store.versions[id] = 0
	return id, nil
}
func (store *memoryStore) Delete(_ context.Context, id uuid.UUID) error {
	for email, user := range store.users {
		if user.ID == id {
			delete(store.users, email)
			delete(store.versions, id)
		}
	}
	return nil
}
func (store *memoryStore) FindLoginAccountByID(_ context.Context, userID uuid.UUID) (identity.LoginAccount, bool, error) {
	for _, user := range store.users {
		if user.ID == userID {
			return user, true, nil
		}
	}
	return identity.LoginAccount{}, false, nil
}
func (store *memoryStore) ChangePassword(_ context.Context, userID uuid.UUID, expectedHash, passwordHash string) (int64, bool, error) {
	for email, user := range store.users {
		if user.ID == userID && user.HashedPassword == expectedHash {
			user.HashedPassword = passwordHash
			store.users[email] = user
			store.versions[userID]++
			return store.versions[userID], true, nil
		}
	}
	return 0, false, nil
}
func (store *memoryStore) RevokeSessions(_ context.Context, userID uuid.UUID) (int64, error) {
	if _, ok := store.versions[userID]; !ok {
		return 0, errors.New("identity not found")
	}
	store.versions[userID]++
	return store.versions[userID], nil
}
func (store *memoryStore) GetByEmail(_ context.Context, email string) (uuid.UUID, string, bool, error) {
	user, ok := store.users[email]
	return user.ID, "Ada Example", ok, nil
}
func (*memoryStore) CreatePasswordReset(context.Context, uuid.UUID, []byte, time.Time) error {
	return nil
}
func (store *memoryStore) ResetPassword(_ context.Context, _ []byte, _ string, _ time.Time) (uuid.UUID, int64, error) {
	for _, user := range store.users {
		store.versions[user.ID]++
		return user.ID, store.versions[user.ID], nil
	}
	return uuid.Nil, 0, errors.New("reset token not found")
}

type provisionerStub struct {
	calls        int
	synchronized map[uuid.UUID]int64
}

func (stub *provisionerStub) ProvisionCustomer(context.Context, uuid.UUID, string, string) error {
	stub.calls++
	return nil
}

func (stub *provisionerStub) SyncCustomerSessionVersion(_ context.Context, id uuid.UUID, version int64) error {
	if stub.synchronized == nil {
		stub.synchronized = make(map[uuid.UUID]int64)
	}
	stub.synchronized[id] = version
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
	store := &memoryStore{users: map[string]identity.LoginAccount{}, versions: map[uuid.UUID]int64{}}
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
	cookie := login.Result().Cookies()[0]
	assert.Equal(t, sessionCookie, cookie.Name)
	assert.True(t, cookie.HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	assert.Equal(t, "no-store", login.Header().Get("Cache-Control"))
	token, err := handler.tokens.Decode(cookie.Value)
	require.NoError(t, err)
	actualIssuer, ok := token.Issuer()
	require.True(t, ok)
	assert.Equal(t, issuer, actualIssuer)
	actualAudience, ok := token.Audience()
	require.True(t, ok)
	assert.Equal(t, []string{audience}, actualAudience)
	expiresAt, ok := token.Expiration()
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(sessionLifetime), expiresAt, 5*time.Second)
	jwtID, ok := token.JwtID()
	assert.True(t, ok)
	assert.NotEmpty(t, jwtID)
	secondLogin := httptest.NewRecorder()
	handler.Routes().ServeHTTP(secondLogin, jsonRequest(http.MethodPost, "/login", `{"email":"ada@example.test","password":"IdentityPhase5!Pass"}`))
	require.Equal(t, http.StatusOK, secondLogin.Code)
	assert.NotEqual(t, cookie.Value, secondLogin.Result().Cookies()[0].Value)
	secureLogin := httptest.NewRecorder()
	secureRequest := jsonRequest(http.MethodPost, "/login", `{"email":"ada@example.test","password":"IdentityPhase5!Pass"}`)
	secureRequest.Header.Set("X-Forwarded-Proto", "https")
	handler.Routes().ServeHTTP(secureLogin, secureRequest)
	require.Equal(t, http.StatusOK, secureLogin.Code)
	assert.True(t, secureLogin.Result().Cookies()[0].Secure)
}

func TestLogoutRevokesSessionAndRejectsReplay(t *testing.T) {
	store := &memoryStore{users: map[string]identity.LoginAccount{}, versions: map[uuid.UUID]int64{}}
	passwordHash, err := identity.HashPassword("IdentityPhase5!Pass")
	require.NoError(t, err)
	userID := uuid.New()
	store.users["ada@example.test"] = identity.LoginAccount{ID: userID, Email: "ada@example.test", HashedPassword: passwordHash}
	store.versions[userID] = 0
	provisioner := &provisionerStub{}
	handler, err := New(store, identityTestSecret, provisioner, notifierStub{})
	require.NoError(t, err)
	_, rawToken, err := handler.tokens.Encode(map[string]any{"user_id": userID.String(), "session_version": int64(0), "iss": issuer, "aud": audience, "jti": uuid.NewString(), "iat": time.Now().Unix(), "exp": time.Now().Add(sessionLifetime).Unix()})
	require.NoError(t, err)

	logout := func() *httptest.ResponseRecorder {
		request := jsonRequest(http.MethodPost, "/logout", "")
		request.AddCookie(&http.Cookie{Name: sessionCookie, Value: rawToken})
		request.Header.Set("X-CSRF-Protection", "1")
		response := httptest.NewRecorder()
		handler.Routes().ServeHTTP(response, request)
		return response
	}
	missingCSRF := jsonRequest(http.MethodPost, "/logout", "")
	missingCSRF.AddCookie(&http.Cookie{Name: sessionCookie, Value: rawToken})
	missingCSRFResponse := httptest.NewRecorder()
	handler.Routes().ServeHTTP(missingCSRFResponse, missingCSRF)
	require.Equal(t, http.StatusForbidden, missingCSRFResponse.Code)
	assert.Equal(t, int64(0), store.versions[userID])
	first := logout()
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Equal(t, int64(1), store.versions[userID])
	assert.Equal(t, int64(1), provisioner.synchronized[userID])
	require.NotEmpty(t, first.Result().Cookies())
	assert.Equal(t, -1, first.Result().Cookies()[0].MaxAge)
	assert.Equal(t, http.StatusUnauthorized, logout().Code)
}

func TestProtectedRoutesRejectWrongJWTContract(t *testing.T) {
	userID := uuid.New()
	store := &memoryStore{
		users:    map[string]identity.LoginAccount{"ada@example.test": {ID: userID, Email: "ada@example.test", HashedPassword: "unused"}},
		versions: map[uuid.UUID]int64{userID: 0},
	}
	provisioner := &provisionerStub{}
	handler, err := New(store, identityTestSecret, provisioner, notifierStub{})
	require.NoError(t, err)

	makeToken := func(algorithm, actualIssuer, actualAudience string, expiresAt time.Time) string {
		auth := jwtauth.New(algorithm, []byte(identityTestSecret), nil)
		_, encoded, encodeErr := auth.Encode(map[string]any{
			"user_id": userID.String(), "session_version": int64(0),
			"iss": actualIssuer, "aud": actualAudience, "jti": uuid.NewString(),
			"iat": time.Now().Add(-time.Minute).Unix(), "exp": expiresAt.Unix(),
		})
		require.NoError(t, encodeErr)
		return encoded
	}
	tests := map[string]string{
		"wrong issuer":    makeToken("HS256", "attacker", audience, time.Now().Add(time.Minute)),
		"wrong audience":  makeToken("HS256", issuer, "other-service", time.Now().Add(time.Minute)),
		"wrong algorithm": makeToken("HS384", issuer, audience, time.Now().Add(time.Minute)),
		"expired":         makeToken("HS256", issuer, audience, time.Now().Add(-time.Minute)),
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			request := jsonRequest(http.MethodPost, "/logout", "")
			request.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
			request.Header.Set("X-CSRF-Protection", "1")
			response := httptest.NewRecorder()
			handler.Routes().ServeHTTP(response, request)
			assert.Equal(t, http.StatusUnauthorized, response.Code)
		})
	}
	assert.Empty(t, provisioner.synchronized)
	assert.Equal(t, int64(0), store.versions[userID])
}

func TestChangePasswordRequiresCurrentPasswordAndRevokesSession(t *testing.T) {
	store := &memoryStore{users: map[string]identity.LoginAccount{}, versions: map[uuid.UUID]int64{}}
	currentHash, err := identity.HashPassword("IdentityPhase5!Pass")
	require.NoError(t, err)
	userID := uuid.New()
	store.users["ada@example.test"] = identity.LoginAccount{ID: userID, Email: "ada@example.test", HashedPassword: currentHash}
	store.versions[userID] = 0
	provisioner := &provisionerStub{}
	handler, err := New(store, identityTestSecret, provisioner, notifierStub{})
	require.NoError(t, err)
	_, rawToken, err := handler.tokens.Encode(map[string]any{"user_id": userID.String(), "session_version": int64(0), "iss": issuer, "aud": audience, "jti": uuid.NewString(), "iat": time.Now().Unix(), "exp": time.Now().Add(sessionLifetime).Unix()})
	require.NoError(t, err)
	wrongCurrent := jsonRequest(http.MethodPost, "/change-password", `{"current_password":"wrong-password","new_password":"IdentityPhase5!NewPass"}`)
	wrongCurrent.AddCookie(&http.Cookie{Name: sessionCookie, Value: rawToken})
	wrongCurrent.Header.Set("X-CSRF-Protection", "1")
	wrongResponse := httptest.NewRecorder()
	handler.Routes().ServeHTTP(wrongResponse, wrongCurrent)
	require.Equal(t, http.StatusUnauthorized, wrongResponse.Code)
	assert.Equal(t, int64(0), store.versions[userID])
	assert.Empty(t, provisioner.synchronized)

	request := jsonRequest(http.MethodPost, "/change-password", `{"current_password":"IdentityPhase5!Pass","new_password":"IdentityPhase5!NewPass"}`)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: rawToken})
	request.Header.Set("X-CSRF-Protection", "1")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, int64(1), store.versions[userID])
	assert.Equal(t, int64(1), provisioner.synchronized[userID])
	changed, found, err := store.FindLoginAccountByID(t.Context(), userID)
	require.NoError(t, err)
	require.True(t, found)
	assert.True(t, identity.VerifyPassword(changed.HashedPassword, "IdentityPhase5!NewPass"))

	replay := jsonRequest(http.MethodPost, "/change-password", `{"current_password":"IdentityPhase5!NewPass","new_password":"IdentityPhase5!Third"}`)
	replay.AddCookie(&http.Cookie{Name: sessionCookie, Value: rawToken})
	replay.Header.Set("X-CSRF-Protection", "1")
	replayResponse := httptest.NewRecorder()
	handler.Routes().ServeHTTP(replayResponse, replay)
	assert.Equal(t, http.StatusUnauthorized, replayResponse.Code)
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
	knownStore := &memoryStore{users: map[string]identity.LoginAccount{"known@example.test": {ID: knownID, Email: "known@example.test"}}, versions: map[uuid.UUID]int64{knownID: 0}}
	unknownStore := &memoryStore{users: map[string]identity.LoginAccount{}, versions: map[uuid.UUID]int64{}}
	knownNotifier, unknownNotifier := &resetNotifierStub{}, &resetNotifierStub{}
	known, err := New(knownStore, identityTestSecret, &provisionerStub{}, knownNotifier)
	require.NoError(t, err)
	unknown, err := New(unknownStore, identityTestSecret, &provisionerStub{}, unknownNotifier)
	require.NoError(t, err)
	knownJobs, unknownJobs := []func(){}, []func(){}
	known.dispatch = func(job func()) { knownJobs = append(knownJobs, job) }
	unknown.dispatch = func(job func()) { unknownJobs = append(unknownJobs, job) }

	knownResponse := httptest.NewRecorder()
	known.Routes().ServeHTTP(knownResponse, jsonRequest(http.MethodPost, "/forgot-password", `{"email":"known@example.test"}`))
	unknownResponse := httptest.NewRecorder()
	unknown.Routes().ServeHTTP(unknownResponse, jsonRequest(http.MethodPost, "/forgot-password", `{"email":"unknown@example.test"}`))

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
	store := &memoryStore{users: map[string]identity.LoginAccount{"known@example.test": {ID: userID, Email: "known@example.test"}}, versions: map[uuid.UUID]int64{userID: 0}}
	provisioner := &provisionerStub{}
	handler, err := New(store, identityTestSecret, provisioner, notifierStub{})
	require.NoError(t, err)
	token := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, jsonRequest(http.MethodPost, "/reset-password", `{"token":"`+token+`","new_password":"UniqueResetPassword2026!"}`))
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, int64(1), provisioner.synchronized[userID])
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

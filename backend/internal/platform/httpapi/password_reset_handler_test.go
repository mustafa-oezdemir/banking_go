package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

type passwordResetStoreStub struct {
	user        sqlc.User
	found       bool
	lookups     []string
	createdHash []byte
	expiresAt   time.Time
	resetHash   []byte
	resetErr    error
}

func (store *passwordResetStoreStub) GetUserByEmail(_ context.Context, email string) (sqlc.User, error) {
	store.lookups = append(store.lookups, email)
	if !store.found || email != store.user.Email {
		return sqlc.User{}, sql.ErrNoRows
	}
	return store.user, nil
}

func (store *passwordResetStoreStub) CreatePasswordResetToken(_ context.Context, _ uuid.UUID, hash []byte, expiresAt time.Time) error {
	store.createdHash = append([]byte(nil), hash...)
	store.expiresAt = expiresAt
	return nil
}

func (store *passwordResetStoreStub) ResetPasswordWithToken(_ context.Context, hash []byte, _ string, _ time.Time) (uuid.UUID, error) {
	store.resetHash = append([]byte(nil), hash...)
	if store.resetErr != nil {
		return uuid.Nil, store.resetErr
	}
	return store.user.ID, nil
}

type passwordResetNotifierStub struct {
	tokens []string
	err    error
}

func (notifier *passwordResetNotifierStub) SendPasswordReset(_ context.Context, _, _, token string) error {
	notifier.tokens = append(notifier.tokens, token)
	return notifier.err
}

func (*passwordResetNotifierStub) NotifyActivity(context.Context, notification.Activity) error {
	return nil
}
func (*passwordResetNotifierStub) Enabled() bool { return true }

func resetHandlerForTest(store *passwordResetStoreStub, notifier *passwordResetNotifierStub) (*Handler, *[]func()) {
	jobs := make([]func(), 0, 1)
	return &Handler{
		passwordResets: store,
		notifier:       notifier,
		dispatchReset: func(job func()) {
			jobs = append(jobs, job)
		},
	}, &jobs
}

func TestForgotPasswordReturnsIdenticalGenericResponseWithoutWaitingForJobs(t *testing.T) {
	knownStore := &passwordResetStoreStub{found: true, user: sqlc.User{
		ID: uuid.New(), Email: "owner@example.test", FullName: "Owner Example",
	}}
	knownNotifier := &passwordResetNotifierStub{}
	knownHandler, knownJobs := resetHandlerForTest(knownStore, knownNotifier)

	unknownStore := &passwordResetStoreStub{}
	unknownNotifier := &passwordResetNotifierStub{}
	unknownHandler, unknownJobs := resetHandlerForTest(unknownStore, unknownNotifier)

	knownResponse := httptest.NewRecorder()
	knownHandler.ForgotPassword(knownResponse, httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(`{"email":"OWNER@example.test"}`)))
	unknownResponse := httptest.NewRecorder()
	unknownHandler.ForgotPassword(unknownResponse, httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(`{"email":"nobody@example.test"}`)))

	assert.Equal(t, http.StatusAccepted, knownResponse.Code)
	assert.Equal(t, knownResponse.Body.String(), unknownResponse.Body.String())
	assert.Equal(t, passwordResetAccepted.Message, responseMessage(t, knownResponse))
	assert.Len(t, *knownJobs, 1, "lookup and delivery must not hold the HTTP response")
	assert.Len(t, *unknownJobs, 1)
	assert.Empty(t, knownNotifier.tokens)

	(*knownJobs)[0]()
	(*unknownJobs)[0]()
	require.Len(t, knownNotifier.tokens, 1)
	assert.Empty(t, unknownNotifier.tokens)
	decoded, err := base64.RawURLEncoding.DecodeString(knownNotifier.tokens[0])
	require.NoError(t, err)
	assert.Len(t, decoded, passwordResetTokenBytes)
	expectedHash := sha256.Sum256([]byte(knownNotifier.tokens[0]))
	assert.Equal(t, expectedHash[:], knownStore.createdHash)
	assert.WithinDuration(t, time.Now().Add(passwordResetLifetime), knownStore.expiresAt, time.Second)
}

func TestForgotPasswordMalformedPayloadUsesSameAcceptedResponse(t *testing.T) {
	store := &passwordResetStoreStub{}
	handler, jobs := resetHandlerForTest(store, &passwordResetNotifierStub{})
	response := httptest.NewRecorder()
	handler.ForgotPassword(response, httptest.NewRequest(http.MethodPost, "/forgot-password", strings.NewReader(`{"email":"x@example.test","admin":true}`)))
	assert.Equal(t, http.StatusAccepted, response.Code)
	assert.Equal(t, passwordResetAccepted.Message, responseMessage(t, response))
	require.Len(t, *jobs, 1)
	(*jobs)[0]()
	assert.Equal(t, "invalid-password-reset@invalid.example", store.lookups[0])
}

func TestResetPasswordHashesOnlyTheCanonicalTokenAndClearsCurrentCookie(t *testing.T) {
	raw := make([]byte, passwordResetTokenBytes)
	for index := range raw {
		raw[index] = byte(index)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	store := &passwordResetStoreStub{user: sqlc.User{ID: uuid.New()}}
	handler, _ := resetHandlerForTest(store, &passwordResetNotifierStub{})
	response := httptest.NewRecorder()
	handler.ResetPassword(response, httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(
		`{"token":"`+token+`","new_password":"UniqueResetPassword2026!"}`,
	)))
	require.Equal(t, http.StatusOK, response.Code)
	expectedHash := sha256.Sum256([]byte(token))
	assert.Equal(t, expectedHash[:], store.resetHash)
	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, sessionCookieName, cookies[0].Name)
	assert.Equal(t, -1, cookies[0].MaxAge)
}

func TestResetPasswordKeepsInvalidTokenMessageGeneric(t *testing.T) {
	store := &passwordResetStoreStub{resetErr: errors.New("database detail")}
	handler, _ := resetHandlerForTest(store, &passwordResetNotifierStub{})
	response := httptest.NewRecorder()
	handler.ResetPassword(response, httptest.NewRequest(http.MethodPost, "/reset-password", strings.NewReader(
		`{"token":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","new_password":"UniqueResetPassword2026!"}`,
	)))
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "reset link is invalid or expired")
	assert.NotContains(t, response.Body.String(), "database detail")
}

func responseMessage(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var response MessageResponse
	require.NoError(t, json.NewDecoder(strings.NewReader(recorder.Body.String())).Decode(&response))
	return response.Message
}

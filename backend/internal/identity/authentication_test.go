package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type authenticationRepositoryStub struct {
	account *LoginAccount
	err     error
	email   string
	found   bool
	version int64
}

func (repository *authenticationRepositoryStub) FindLoginAccount(
	_ context.Context,
	email string,
) (LoginAccount, bool, error) {
	repository.email = email
	if repository.account == nil {
		return LoginAccount{}, repository.found, repository.err
	}
	return *repository.account, repository.found, repository.err
}

func (repository *authenticationRepositoryStub) SessionVersion(
	_ context.Context,
	_ uuid.UUID,
) (int64, error) {
	return repository.version, repository.err
}

func TestAuthenticationServiceAuthenticatesNormalizedIdentity(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	require.NoError(t, err)
	userID := uuid.New()
	repository := &authenticationRepositoryStub{
		account: &LoginAccount{ID: userID, Email: "customer@example.com", HashedPassword: hash},
		found:   true, version: 4,
	}

	authenticated, err := NewAuthenticationService(repository).Authenticate(
		t.Context(), " CUSTOMER@EXAMPLE.COM ", "correct horse battery staple",
	)

	require.NoError(t, err)
	require.Equal(t, "customer@example.com", repository.email)
	require.Equal(t, userID, authenticated.UserID)
	require.EqualValues(t, 4, authenticated.SessionVersion)
}

func TestAuthenticationServiceHidesCredentialFailures(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	require.NoError(t, err)
	tests := []struct {
		name       string
		repository *authenticationRepositoryStub
		email      string
		password   string
	}{
		{name: "unknown account", repository: &authenticationRepositoryStub{}, email: "unknown@example.com", password: "any password"},
		{name: "wrong password", repository: &authenticationRepositoryStub{
			found: true, account: &LoginAccount{ID: uuid.New(), HashedPassword: hash},
		}, email: "known@example.com", password: "wrong password"},
		{name: "invalid email", repository: &authenticationRepositoryStub{}, email: "invalid", password: "any password"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, authenticateErr := NewAuthenticationService(test.repository).Authenticate(
				t.Context(), test.email, test.password,
			)
			require.ErrorIs(t, authenticateErr, ErrInvalidCredentials)
		})
	}
}

func TestAuthenticationServicePropagatesRepositoryFailure(t *testing.T) {
	expected := errors.New("repository unavailable")
	repository := &authenticationRepositoryStub{err: expected}

	_, err := NewAuthenticationService(repository).Authenticate(
		t.Context(), "customer@example.com", "correct horse battery staple",
	)

	require.ErrorIs(t, err, expected)
}

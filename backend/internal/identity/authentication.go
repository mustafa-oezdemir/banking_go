package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrInvalidCredentials deliberately hides whether the email or password was wrong.
var ErrInvalidCredentials = errors.New("invalid credentials")

// LoginAccount contains only the persisted identity data needed to authenticate.
type LoginAccount struct {
	Email          string
	HashedPassword string
	ID             uuid.UUID
}

// AuthenticatedIdentity is the application result used to issue a session token.
type AuthenticatedIdentity struct {
	Email          string
	UserID         uuid.UUID
	SessionVersion int64
}

// AuthenticationRepository is the persistence port required by the login use case.
type AuthenticationRepository interface {
	FindLoginAccount(context.Context, string) (LoginAccount, bool, error)
	SessionVersion(context.Context, uuid.UUID) (int64, error)
}

// AuthenticationService authenticates credentials without exposing persistence details.
type AuthenticationService struct {
	repository AuthenticationRepository
}

// NewAuthenticationService constructs the login use case.
func NewAuthenticationService(repository AuthenticationRepository) *AuthenticationService {
	return &AuthenticationService{repository: repository}
}

// Authenticate validates credentials and returns the data required to issue a session.
func (service *AuthenticationService) Authenticate(
	ctx context.Context,
	rawEmail string,
	password string,
) (AuthenticatedIdentity, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil || password == "" || len([]byte(password)) > MaxPasswordBytes {
		VerifyDummyPassword(password)
		return AuthenticatedIdentity{}, ErrInvalidCredentials
	}

	account, found, err := service.repository.FindLoginAccount(ctx, email)
	if err != nil {
		return AuthenticatedIdentity{}, err
	}
	if !found {
		VerifyDummyPassword(password)
		return AuthenticatedIdentity{}, ErrInvalidCredentials
	}
	if !VerifyPassword(account.HashedPassword, password) {
		return AuthenticatedIdentity{}, ErrInvalidCredentials
	}

	version, err := service.repository.SessionVersion(ctx, account.ID)
	if err != nil {
		return AuthenticatedIdentity{}, err
	}
	return AuthenticatedIdentity{
		UserID: account.ID, Email: account.Email, SessionVersion: version,
	}, nil
}

package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/identity"
)

// IdentityRepository adapts PostgreSQL identity records to the application port.
type IdentityRepository struct {
	store *Store
}

// NewIdentityRepository constructs a PostgreSQL identity adapter.
func NewIdentityRepository(store *Store) *IdentityRepository {
	return &IdentityRepository{store: store}
}

// FindLoginAccount returns the minimum identity record required for authentication.
func (repository *IdentityRepository) FindLoginAccount(
	ctx context.Context,
	email string,
) (identity.LoginAccount, bool, error) {
	user, err := repository.store.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.LoginAccount{}, false, nil
	}
	if err != nil {
		return identity.LoginAccount{}, false, err
	}
	return identity.LoginAccount{
		ID: user.ID, Email: user.Email, HashedPassword: user.HashedPassword,
	}, true, nil
}

// SessionVersion returns the revocation generation used in new tokens.
func (repository *IdentityRepository) SessionVersion(ctx context.Context, userID uuid.UUID) (int64, error) {
	return repository.store.GetUserSessionVersion(ctx, userID)
}

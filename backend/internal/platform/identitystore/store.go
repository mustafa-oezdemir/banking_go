// Package identitystore is the PostgreSQL adapter owned by the Identity service.
package identitystore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/identity"
)

// Store accesses only the identity schema; it must not be imported by Banking.
type Store struct{ db *sql.DB }

// New creates the Identity-owned PostgreSQL adapter.
func New(db *sql.DB) *Store { return &Store{db: db} }

// FindLoginAccount implements identity.AuthenticationRepository.
func (store *Store) FindLoginAccount(ctx context.Context, email string) (identity.LoginAccount, bool, error) {
	var account identity.LoginAccount
	err := store.db.QueryRowContext(ctx, `SELECT id, email, hashed_password FROM identity.users WHERE email = $1`, email).
		Scan(&account.ID, &account.Email, &account.HashedPassword)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.LoginAccount{}, false, nil
	}
	return account, err == nil, err
}

// SessionVersion implements identity.AuthenticationRepository.
func (store *Store) SessionVersion(ctx context.Context, userID uuid.UUID) (int64, error) {
	var version int64
	err := store.db.QueryRowContext(ctx, `SELECT session_version FROM identity.users WHERE id = $1`, userID).Scan(&version)
	return version, err
}

// Register creates an Identity User. Provisioning its Banking Customer remains
// a separate explicit service command.
func (store *Store) Register(ctx context.Context, email, fullName, passwordHash string) (uuid.UUID, error) {
	identifier := uuid.New()
	_, err := store.db.ExecContext(ctx, `INSERT INTO identity.users (id, email, full_name, hashed_password) VALUES ($1, $2, $3, $4)`, identifier, email, fullName, passwordHash)
	return identifier, err
}

// Delete removes a just-created Identity User when Banking provisioning fails.
func (store *Store) Delete(ctx context.Context, userID uuid.UUID) error {
	_, err := store.db.ExecContext(ctx, `DELETE FROM identity.users WHERE id = $1`, userID)
	return err
}

// GetByEmail returns display data for password-reset delivery without exposing a hash.
func (store *Store) GetByEmail(ctx context.Context, email string) (uuid.UUID, string, bool, error) {
	var identifier uuid.UUID
	var fullName string
	err := store.db.QueryRowContext(ctx, `SELECT id, full_name FROM identity.users WHERE email = $1`, email).Scan(&identifier, &fullName)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, "", false, nil
	}
	return identifier, fullName, err == nil, err
}

// CreatePasswordReset stores a one-time hash, never the raw reset secret.
func (store *Store) CreatePasswordReset(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Commit below wins on success.
	if _, err = tx.ExecContext(ctx, `UPDATE identity.password_reset_tokens SET used_at = CURRENT_TIMESTAMP WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO identity.password_reset_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, tokenHash, userID, expiresAt); err != nil {
		return err
	}
	return tx.Commit()
}

// ResetPassword atomically consumes a valid reset token and increments the
// Identity session generation.
func (store *Store) ResetPassword(ctx context.Context, tokenHash []byte, passwordHash string, now time.Time) (uuid.UUID, error) {
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback() //nolint:errcheck // Commit below wins on success.
	var userID uuid.UUID
	err = tx.QueryRowContext(ctx, `DELETE FROM identity.password_reset_tokens WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2 RETURNING user_id`, tokenHash, now).Scan(&userID)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE identity.users SET hashed_password = $1, session_version = session_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, passwordHash, userID); err != nil {
		return uuid.Nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE identity.password_reset_tokens SET used_at = $2 WHERE user_id = $1 AND used_at IS NULL`, userID, now); err != nil {
		return uuid.Nil, err
	}
	if err = tx.Commit(); err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}

// RevokeSessions advances the Identity-side session generation.
func (store *Store) RevokeSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := store.db.ExecContext(ctx, `UPDATE identity.users SET session_version = session_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, userID)
	return err
}

package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// CreatePasswordResetToken invalidates earlier active tokens and stores only
// the hash of the newly issued token.
func (store *Store) CreatePasswordResetToken(
	ctx context.Context,
	userID uuid.UUID,
	tokenHash []byte,
	expiresAt time.Time,
) error {
	return store.ExecTxWithHandle(ctx, func(_ *sqlc.Queries, executor sqlc.DBTX) error {
		if _, err := executor.ExecContext(ctx, `
			DELETE FROM password_reset_tokens
			WHERE expires_at < CURRENT_TIMESTAMP - INTERVAL '7 days'`); err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `
			UPDATE password_reset_tokens
			SET used_at = CURRENT_TIMESTAMP
			WHERE user_id = $1 AND used_at IS NULL`, userID); err != nil {
			return err
		}
		_, err := executor.ExecContext(ctx, `
			INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
			VALUES ($1, $2, $3)`, userID, tokenHash, expiresAt)
		return err
	})
}

// ResetPasswordWithToken consumes one valid token, updates the password, and
// revokes every existing user session in the same transaction.
func (store *Store) ResetPasswordWithToken(
	ctx context.Context,
	tokenHash []byte,
	hashedPassword string,
	now time.Time,
) (uuid.UUID, error) {
	var userID uuid.UUID
	err := store.ExecTxWithHandle(ctx, func(_ *sqlc.Queries, executor sqlc.DBTX) error {
		var tokenID uuid.UUID
		if err := executor.QueryRowContext(ctx, `
			SELECT id, user_id
			FROM password_reset_tokens
			WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
			FOR UPDATE`, tokenHash, now).Scan(&tokenID, &userID); err != nil {
			return err
		}
		result, err := executor.ExecContext(ctx, `
			UPDATE users
			SET hashed_password = $2, session_version = session_version + 1
			WHERE id = $1`, userID, hashedPassword)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return sql.ErrNoRows
		}
		_, err = executor.ExecContext(ctx, `
			UPDATE password_reset_tokens
			SET used_at = $2
			WHERE user_id = $1 AND used_at IS NULL`, userID, now)
		return err
	})
	return userID, err
}

package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

func TestPasswordResetTokenIsSingleUseAndRevokesSessions(t *testing.T) {
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DB_URL")
	}
	if dbURL == "" {
		t.Skip("TEST_DB_URL or DB_URL is required for integration tests")
	}
	database, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	ctx := context.Background()
	store := NewStore(database)
	email := "password-reset-" + uuid.NewString() + "@example.com"
	user, err := store.CreateUser(ctx, sqlc.CreateUserParams{
		Email: email, HashedPassword: "$2a$10$invalid-but-replaced", FullName: "Reset Test",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := database.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
		require.NoError(t, cleanupErr)
	})

	beforeVersion, err := store.GetUserSessionVersion(ctx, user.ID)
	require.NoError(t, err)
	tokenHash := sha256.Sum256([]byte("one-time-token"))
	require.NoError(t, store.CreatePasswordResetToken(ctx, user.ID, tokenHash[:], time.Now().UTC().Add(15*time.Minute)))
	newHash, err := bcrypt.GenerateFromPassword([]byte("A-unique-password-2026!"), bcrypt.MinCost)
	require.NoError(t, err)
	resetUserID, err := store.ResetPasswordWithToken(ctx, tokenHash[:], string(newHash), time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, user.ID, resetUserID)

	updated, err := store.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(updated.HashedPassword), []byte("A-unique-password-2026!")))
	afterVersion, err := store.GetUserSessionVersion(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, beforeVersion+1, afterVersion)

	_, err = store.ResetPasswordWithToken(ctx, tokenHash[:], string(newHash), time.Now().UTC())
	require.True(t, errors.Is(err, sql.ErrNoRows))
}

func TestExpiredPasswordResetTokenIsRejected(t *testing.T) {
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DB_URL")
	}
	if dbURL == "" {
		t.Skip("TEST_DB_URL or DB_URL is required for integration tests")
	}
	database, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	ctx := context.Background()
	store := NewStore(database)
	user, err := store.CreateUser(ctx, sqlc.CreateUserParams{
		Email:          "expired-reset-" + uuid.NewString() + "@example.com",
		HashedPassword: "$2a$10$unchanged", FullName: "Expired Reset",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := database.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
		require.NoError(t, cleanupErr)
	})

	tokenHash := sha256.Sum256([]byte("expired-token"))
	require.NoError(t, store.CreatePasswordResetToken(ctx, user.ID, tokenHash[:], time.Now().UTC().Add(-time.Minute)))
	_, err = store.ResetPasswordWithToken(ctx, tokenHash[:], "replacement", time.Now().UTC())
	require.True(t, errors.Is(err, sql.ErrNoRows))
}

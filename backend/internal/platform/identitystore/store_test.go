package identitystore

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordResetTokenRotationAndSingleUse(t *testing.T) {
	databaseURL := os.Getenv("TEST_DB_URL")
	if databaseURL == "" {
		t.Skip("TEST_DB_URL is required for Identity persistence integration tests")
	}
	database, err := sql.Open("postgres", databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	ctx := context.Background()
	userID := uuid.New()
	_, err = database.ExecContext(ctx, `INSERT INTO identity.users (id, email, full_name, hashed_password) VALUES ($1, $2, $3, $4)`, userID, "reset-"+userID.String()+"@example.test", "Reset Test", "test-only")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM identity.users WHERE id = $1`, userID)
	})
	store := New(database)
	first := sha256.Sum256([]byte("first-reset-token"))
	second := sha256.Sum256([]byte("second-reset-token"))
	now := time.Now().UTC()
	require.NoError(t, store.CreatePasswordReset(ctx, userID, first[:], now.Add(15*time.Minute)))
	require.NoError(t, store.CreatePasswordReset(ctx, userID, second[:], now.Add(15*time.Minute)))

	_, err = store.ResetPassword(ctx, first[:], "replacement", now)
	assert.True(t, errors.Is(err, sql.ErrNoRows), "issuing a new token must invalidate the old token")
	resetUserID, err := store.ResetPassword(ctx, second[:], "replacement", now)
	require.NoError(t, err)
	assert.Equal(t, userID, resetUserID)
	_, err = store.ResetPassword(ctx, second[:], "replacement", now)
	assert.True(t, errors.Is(err, sql.ErrNoRows), "a reset token must be single use")
}

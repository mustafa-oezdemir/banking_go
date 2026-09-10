package db

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/account"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

func TestUpdateCustomerProfilePersistsAndAudits(t *testing.T) {
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		t.Skip("TEST_DB_URL is required for profile integration tests")
	}
	database, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	store := NewStore(database)
	user, err := store.CreateUser(t.Context(), sqlc.CreateUserParams{
		Email: "profile-" + uuid.NewString() + "@example.com", HashedPassword: "test-only", FullName: "Initial Name",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, cleanupErr := database.ExecContext(cleanupCtx, `DELETE FROM audit_events WHERE owner_id = $1 AND event_type = 'PROFILE_UPDATED'`, user.ID)
		require.NoError(t, cleanupErr)
		_, cleanupErr = database.ExecContext(cleanupCtx, `DELETE FROM users WHERE id = $1`, user.ID)
		require.NoError(t, cleanupErr)
	})

	updated, err := store.UpdateCustomerProfile(t.Context(), account.ProfileUpdate{
		UserID: user.ID, FullName: "Anna Beispiel", Phone: "+49 170 1234567",
		BirthDate: time.Date(1990, 5, 12, 0, 0, 0, 0, time.UTC), AddressLine1: "Musterstraße 12",
		AddressLine2: "Wohnung 4", PostalCode: "10115", City: "Berlin", CountryCode: "DE",
	})
	require.NoError(t, err)
	assert.Equal(t, "Anna Beispiel", updated.FullName)
	assert.Equal(t, "1990-05-12", updated.BirthDate)

	loaded, err := store.GetCustomerProfile(t.Context(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, updated, loaded)

	var auditCount int
	require.NoError(t, database.QueryRowContext(t.Context(), `
		SELECT COUNT(*) FROM audit_events
		WHERE owner_id = $1 AND event_type = 'PROFILE_UPDATED'
		  AND event_data = '{"scope":"customer_profile"}'::JSONB`, user.ID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
}

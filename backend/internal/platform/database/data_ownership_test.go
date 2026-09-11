package db

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// TestServiceDatabaseRolesCannotCrossPrivateSchemas verifies migration 000015
// grants each runtime role access only to its owned schema. It runs when the
// integration database is configured (CI and Docker validation).
func TestServiceDatabaseRolesCannotCrossPrivateSchemas(t *testing.T) {
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

	assertPrivilege := func(role, schema string, expected bool) {
		t.Helper()
		var granted bool
		require.NoError(t, database.QueryRowContext(context.Background(), `SELECT has_schema_privilege($1, $2, 'USAGE')`, role, schema).Scan(&granted))
		require.Equalf(t, expected, granted, "role %s schema %s", role, schema)
	}

	assertPrivilege("banking_app", "public", true)
	assertPrivilege("banking_app", "identity", false)
	assertPrivilege("banking_app", "notification", false)
	assertPrivilege("identity_app", "identity", true)
	assertPrivilege("identity_app", "public", false)
	assertPrivilege("identity_app", "notification", false)
	assertPrivilege("notification_app", "notification", true)
	assertPrivilege("notification_app", "public", false)
	assertPrivilege("notification_app", "identity", false)
}

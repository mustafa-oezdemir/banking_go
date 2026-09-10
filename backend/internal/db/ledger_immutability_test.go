package db

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestLedgerEntriesAreAppendOnly(t *testing.T) {
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		t.Skip("TEST_DB_URL is required for migration integration tests")
	}
	database, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	for _, mutation := range []string{
		"UPDATE entries SET description = 'tampered' WHERE id = $1",
		"DELETE FROM entries WHERE id = $1",
	} {
		t.Run(mutation[:6], func(t *testing.T) {
			tx, txErr := database.BeginTx(t.Context(), nil)
			require.NoError(t, txErr)
			defer func() { _ = tx.Rollback() }()

			var accountID uuid.UUID
			txErr = tx.QueryRowContext(context.Background(), `
				INSERT INTO accounts (name, currency, is_system, iban, account_type, status)
				VALUES ($1, 'EUR', TRUE, $2, 'SETTLEMENT', 'ACTIVE') RETURNING id`,
				"Immutability test "+uuid.NewString(), "DE"+uuid.NewString()[:20],
			).Scan(&accountID)
			require.NoError(t, txErr)

			var entryID uuid.UUID
			txErr = tx.QueryRowContext(context.Background(), `
				INSERT INTO entries (account_id, debit, credit, transaction_id, operation_type, description)
				VALUES ($1, 0, 1, $2, 'deposit', 'original') RETURNING id`,
				accountID, uuid.New(),
			).Scan(&entryID)
			require.NoError(t, txErr)

			_, txErr = tx.ExecContext(context.Background(), mutation, entryID)
			require.Error(t, txErr)
			require.Contains(t, txErr.Error(), "ledger entries are append-only")
		})
	}
}

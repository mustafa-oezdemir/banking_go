package payment

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"

	sepa "github.com/mustafa-oezdemir/banking_go/internal/account"
	"github.com/mustafa-oezdemir/banking_go/internal/ledger"
	db "github.com/mustafa-oezdemir/banking_go/internal/platform/database"
)

type testLedger struct {
	*ledger.Service
	store *db.Store
}

func setupTestLedger(t *testing.T) *testLedger {
	t.Helper()
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DB_URL")
	}
	if dbURL == "" {
		dbURL = "postgresql://root:secret@localhost:5433/simple_ledger?sslmode=disable"
	}

	sqlDB, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err = sqlDB.PingContext(ctx); err != nil {
		require.NoError(t, sqlDB.Close())
		t.Skipf("PostgreSQL integration test unavailable: %v", err)
	}
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })

	store := db.NewStore(sqlDB)
	return &testLedger{Service: ledger.NewService(db.NewLedgerRepository(store)), store: store}
}

func mustDemoIBAN(t *testing.T) string {
	t.Helper()
	iban, err := sepa.GenerateGermanDemoIBAN()
	require.NoError(t, err)
	return iban
}

func getAccountBalance(t *testing.T, fixture *testLedger, accountID uuid.UUID) string {
	t.Helper()
	balance, err := fixture.store.GetAccountBalance(context.Background(), accountID)
	require.NoError(t, err)
	return balance
}

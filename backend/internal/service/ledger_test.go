package service

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/lib/pq"

	"github.com/mustafa-oezdemir/banking_go/internal/db"
	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// setupTestLedger and helpers would be implemented to provide a testable LedgerService and test DB.
// For demonstration, these are placeholders. In a real repo, use test containers or a test DB.

func setupTestLedger(t *testing.T) *LedgerService {
	// Keep test database configuration separate from the application database.
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
	ledger := NewLedgerService(store)
	return ledger
}

func createTestAccount(t *testing.T, ledger *LedgerService, balance string) uuid.UUID {
	// Use a unique account name for each test run
	accName := "Test Account " + uuid.New().String()

	// Match settlement currency so deposit/transfer validations pass.
	settlement, err := ledger.store.GetSettlementAccount(context.Background())
	require.NoError(t, err)

	account, err := ledger.store.CreateAccount(context.Background(), sqlc.CreateAccountParams{
		OwnerID: uuid.NullUUID{Valid: false}, // No owner for test accounts
		Name:    accName, Currency: settlement.Currency, IsSystem: false,
		Iban: mustDemoIBAN(t), AccountType: "GIROKONTO", Status: "ACTIVE",
	})
	require.NoError(t, err)
	// Optionally pre-fund account for withdrawal/transfer scenarios.
	if balance != "0.00" && balance != "0" && balance != "" {
		err = ledger.Deposit(context.Background(), account.ID, balance)
		require.NoError(t, err)
	}
	return account.ID
}

func mustDemoIBAN(t *testing.T) string {
	t.Helper()
	iban, err := sepa.GenerateGermanDemoIBAN()
	require.NoError(t, err)
	return iban
}

func getAccountBalance(t *testing.T, ledger *LedgerService, accountID uuid.UUID) string {
	balance, err := ledger.store.GetAccountBalance(context.Background(), accountID)
	require.NoError(t, err)
	return balance
}

func TestDeposit_Success(t *testing.T) {
	// Deposit should increase account balance exactly by the amount.
	ledger := setupTestLedger(t)
	accountID := createTestAccount(t, ledger, "0.00")
	err := ledger.Deposit(context.Background(), accountID, "100.00")
	require.NoError(t, err)
	balance := getAccountBalance(t, ledger, accountID)
	assert.Equal(t, "100.0000", balance)
}

func TestWithdraw_InsufficientFunds(t *testing.T) {
	// Withdrawal over balance should fail with business error.
	ledger := setupTestLedger(t)
	accountID := createTestAccount(t, ledger, "50.00")
	err := ledger.Withdraw(context.Background(), accountID, "100.00")
	assert.Error(t, err)
	// Optionally check for ErrInsufficientFunds
}

func TestConcurrentDeposits(t *testing.T) {
	// Concurrent deposits should both commit without lost updates.
	ledger := setupTestLedger(t)
	accountID := createTestAccount(t, ledger, "0.00")
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errCh <- ledger.Deposit(context.Background(), accountID, "100.00")
	}()
	go func() {
		defer wg.Done()
		errCh <- ledger.Deposit(context.Background(), accountID, "100.00")
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	balance := getAccountBalance(t, ledger, accountID)
	assert.Equal(t, "200.0000", balance)
}

func TestLedgerRejectsBlockedAndSystemAccounts(t *testing.T) {
	ledger := setupTestLedger(t)
	settlement, err := ledger.store.GetSettlementAccount(context.Background())
	require.NoError(t, err)
	for _, status := range []string{"BLOCKED", "CLOSED"} {
		account, createErr := ledger.store.CreateAccount(context.Background(), sqlc.CreateAccountParams{
			OwnerID: uuid.NullUUID{}, Name: status + " " + uuid.NewString(), Currency: settlement.Currency,
			IsSystem: false, Iban: mustDemoIBAN(t), AccountType: "GIROKONTO", Status: status,
		})
		require.NoError(t, createErr)
		require.ErrorIs(t, ledger.Deposit(context.Background(), account.ID, "10.00"), ErrAccountBlocked)
		require.ErrorIs(t, ledger.Withdraw(context.Background(), account.ID, "10.00"), ErrAccountBlocked)
		assert.Equal(t, "0.0000", getAccountBalance(t, ledger, account.ID))
	}
	require.ErrorIs(t, ledger.Deposit(context.Background(), settlement.ID, "10.00"), ErrSystemAccount)
}

func TestTransferRejectsBlockedDestinationWithoutChangingBalances(t *testing.T) {
	ledger := setupTestLedger(t)
	fromID := createTestAccount(t, ledger, "100.00")
	settlement, err := ledger.store.GetSettlementAccount(context.Background())
	require.NoError(t, err)
	blocked, err := ledger.store.CreateAccount(context.Background(), sqlc.CreateAccountParams{
		OwnerID: uuid.NullUUID{}, Name: "Blocked target " + uuid.NewString(), Currency: settlement.Currency,
		IsSystem: false, Iban: mustDemoIBAN(t), AccountType: "GIROKONTO", Status: "BLOCKED",
	})
	require.NoError(t, err)

	require.ErrorIs(t, ledger.Transfer(context.Background(), fromID, blocked.ID, "25.00"), ErrAccountBlocked)
	require.ErrorIs(t, ledger.Transfer(context.Background(), fromID, settlement.ID, "25.00"), ErrSystemAccount)
	assert.Equal(t, "100.0000", getAccountBalance(t, ledger, fromID))
	assert.Equal(t, "0.0000", getAccountBalance(t, ledger, blocked.ID))
}

func TestAdminBalanceAdjustmentCreatesAuditAtomically(t *testing.T) {
	ledger := setupTestLedger(t)
	admin, err := ledger.store.CreateUser(t.Context(), sqlc.CreateUserParams{
		Email: "audit-admin-" + uuid.NewString() + "@example.com", HashedPassword: "test-only", FullName: "Audit Admin",
	})
	require.NoError(t, err)
	accountID := createTestAccount(t, ledger, "0.00")
	beforeCount, err := ledger.store.CountAdminAuditEvents(t.Context())
	require.NoError(t, err)

	require.NoError(t, ledger.AdjustBalanceAsAdmin(
		t.Context(), admin.ID, accountID, "DEPOSIT", "25.00", "test-request-id",
	))
	afterCount, err := ledger.store.CountAdminAuditEvents(t.Context())
	require.NoError(t, err)
	assert.Equal(t, beforeCount+1, afterCount)
	assert.Equal(t, "25.0000", getAccountBalance(t, ledger, accountID))
}

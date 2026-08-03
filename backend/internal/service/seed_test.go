package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSeedDemoDataCanRunRepeatedly(t *testing.T) {
	t.Setenv("DEMO_SEED_PASSWORD", "integration-only-secret")
	ledger := setupTestLedger(t)
	payments := NewPaymentService(ledger.store, nil)

	require.NoError(t, SeedDemoData(context.Background(), ledger.store, ledger, payments))
	require.NoError(t, SeedDemoData(context.Background(), ledger.store, ledger, payments))
}

func TestSeedConfiguredAdminIsIndependentFromDemoSeed(t *testing.T) {
	ledger := setupTestLedger(t)
	email := "configured-admin-" + uuid.NewString() + "@example.com"
	t.Setenv("ADMIN_SEED_EMAIL", email)
	t.Setenv("ADMIN_SEED_PASSWORD", "integration-admin-secret")
	t.Setenv("DEMO_SEED", "false")

	require.NoError(t, SeedConfiguredAdmin(context.Background(), ledger.store, ledger))
	require.NoError(t, SeedConfiguredAdmin(context.Background(), ledger.store, ledger))
	admin, err := ledger.store.GetUserByEmail(context.Background(), email)
	require.NoError(t, err)
	role, err := ledger.store.GetUserRole(context.Background(), admin.ID)
	require.NoError(t, err)
	require.Equal(t, "ADMIN", role)
	accounts, err := ledger.store.ListAccountsByOwner(context.Background(), uuid.NullUUID{UUID: admin.ID, Valid: true})
	require.NoError(t, err)
	require.NotEmpty(t, accounts)
	require.Equal(t, "500.0000", getAccountBalance(t, ledger, accounts[0].ID))
}

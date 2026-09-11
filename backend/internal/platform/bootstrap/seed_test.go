package bootstrap

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/payment"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

func TestSeedDemoDataCanRunRepeatedly(t *testing.T) {
	t.Setenv("DEMO_SEED_PASSWORD", "integration-only-secret")
	ledger := setupTestLedger(t)
	payments := payment.NewService(ledger.store, nil)

	require.NoError(t, SeedDemoData(context.Background(), ledger.store, ledger.Service, payments))
	require.NoError(t, SeedDemoData(context.Background(), ledger.store, ledger.Service, payments))
}

func TestEnsureDemoUserReusesAccountOwnedByLegacyEmail(t *testing.T) {
	ledger := setupTestLedger(t)
	ctx := context.Background()
	unique := uuid.NewString()
	canonicalEmail := "canonical-" + unique + "@example.test"
	legacyEmail := "legacy-" + unique + "@example.test"
	legacy, err := ledger.store.CreateUser(ctx, sqlc.CreateUserParams{
		Email: legacyEmail, HashedPassword: "test-only-hash", FullName: "Legacy Demo",
	})
	require.NoError(t, err)
	accountBase := uint64(uuid.New().ID()) + 4_000_000_000
	_, err = createSeedAccount(ctx, ledger.store, legacy.ID, "Legacy Demo Girokonto", "GIROKONTO", accountBase)
	require.NoError(t, err)
	// Reproduce an interrupted prior startup: the canonical user exists, but
	// the deterministic demo account still belongs to the legacy identity.
	_, err = ledger.store.CreateUser(ctx, sqlc.CreateUserParams{
		Email: canonicalEmail, HashedPassword: "test-only-hash", FullName: "Legacy Demo",
	})
	require.NoError(t, err)

	seeded, err := ensureDemoUser(
		ctx, ledger.store, canonicalEmail, "Legacy Demo", accountBase, "test-only-hash", legacyEmail,
	)
	require.NoError(t, err)
	require.Equal(t, legacy.ID, seeded.user.ID)
	require.Equal(t, legacy.ID, seeded.current.OwnerID.UUID)
}

func TestSeedConfiguredAdminIsIndependentFromDemoSeed(t *testing.T) {
	ledger := setupTestLedger(t)
	email := "configured-admin-" + uuid.NewString() + "@example.com"
	t.Setenv("ADMIN_SEED_EMAIL", email)
	t.Setenv("ADMIN_SEED_PASSWORD", "integration-admin-secret")
	t.Setenv("DEMO_SEED", "false")

	require.NoError(t, SeedConfiguredAdmin(context.Background(), ledger.store, ledger.Service))
	admin, err := ledger.store.GetUserByEmail(context.Background(), email)
	require.NoError(t, err)
	role, err := ledger.store.GetUserRole(context.Background(), admin.ID)
	require.NoError(t, err)
	require.Equal(t, "ADMIN", role)
	versionBeforeRestart, err := ledger.store.GetUserSessionVersion(context.Background(), admin.ID)
	require.NoError(t, err)
	require.NoError(t, SeedConfiguredAdmin(context.Background(), ledger.store, ledger.Service))
	versionAfterRestart, err := ledger.store.GetUserSessionVersion(context.Background(), admin.ID)
	require.NoError(t, err)
	require.Equal(t, versionBeforeRestart, versionAfterRestart)
	accounts, err := ledger.store.ListAccountsByOwner(context.Background(), uuid.NullUUID{UUID: admin.ID, Valid: true})
	require.NoError(t, err)
	require.NotEmpty(t, accounts)
	require.Equal(t, "500.0000", getAccountBalance(t, ledger, accounts[0].ID))
}

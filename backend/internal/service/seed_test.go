package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedDemoDataCanRunRepeatedly(t *testing.T) {
	ledger := setupTestLedger(t)
	payments := NewPaymentService(ledger.store, nil)

	require.NoError(t, SeedDemoData(context.Background(), ledger.store, ledger, payments))
	require.NoError(t, SeedDemoData(context.Background(), ledger.store, ledger, payments))
}

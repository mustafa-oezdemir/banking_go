package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func activeAccount(ownerID uuid.UUID, balance string) AccountSnapshot {
	return AccountSnapshot{
		ID: uuid.New(), OwnerID: ownerID, OwnerAssigned: true, Status: "ACTIVE",
		Currency: "EUR", AvailableBalance: decimal.RequireFromString(balance),
	}
}

func TestPlanCustomerTransferIsBalanced(t *testing.T) {
	ownerID := uuid.New()
	posting, err := PlanCustomerTransfer(ownerID, activeAccount(ownerID, "100.00"), activeAccount(ownerID, "0"),
		decimal.RequireFromString("25.50"), TransferPolicy{RequireDestinationOwnership: true})
	require.NoError(t, err)
	assert.True(t, posting.Balanced())
	assert.True(t, posting.DebitLeg.Debit.Equal(decimal.RequireFromString("25.50")))
	assert.True(t, posting.CreditLeg.Credit.Equal(posting.DebitLeg.Debit))
}

func TestPlanCustomerTransferRejectsInvariantViolations(t *testing.T) {
	ownerID := uuid.New()
	source := activeAccount(ownerID, "10.00")
	destination := activeAccount(ownerID, "0")

	_, err := PlanCustomerTransfer(ownerID, source, destination, decimal.RequireFromString("11.00"), TransferPolicy{RequireDestinationOwnership: true})
	assert.ErrorIs(t, err, ErrInsufficientFunds)

	_, err = PlanCustomerTransfer(uuid.New(), source, destination, decimal.RequireFromString("1.00"), TransferPolicy{RequireDestinationOwnership: true})
	assert.ErrorIs(t, err, ErrAccountOwnership)

	blocked := destination
	blocked.Status = "BLOCKED"
	_, err = PlanCustomerTransfer(ownerID, source, blocked, decimal.RequireFromString("1.00"), TransferPolicy{RequireDestinationOwnership: true})
	assert.ErrorIs(t, err, ErrAccountBlocked)

	destination.ID = source.ID
	_, err = PlanCustomerTransfer(ownerID, source, destination, decimal.RequireFromString("1.00"), TransferPolicy{RequireDestinationOwnership: true})
	assert.ErrorIs(t, err, ErrSameAccountTransfer)
}

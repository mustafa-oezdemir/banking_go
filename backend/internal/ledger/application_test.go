package ledger

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type repositoryStub struct {
	depositAmount  decimal.Decimal
	depositCalled  bool
	transferCalled bool
}

func (repository *repositoryStub) CreateFundedCustomer(
	context.Context, NewCustomer, string, decimal.Decimal,
) (FundedCustomer, error) {
	return FundedCustomer{}, nil
}

func (repository *repositoryStub) Deposit(_ context.Context, _ uuid.UUID, amount decimal.Decimal) error {
	repository.depositCalled = true
	repository.depositAmount = amount
	return nil
}

func (*repositoryStub) Withdraw(context.Context, uuid.UUID, decimal.Decimal) error { return nil }
func (*repositoryStub) AdjustBalanceAsAdmin(context.Context, uuid.UUID, uuid.UUID, string, decimal.Decimal, string) error {
	return nil
}

func (repository *repositoryStub) Transfer(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, decimal.Decimal) error {
	repository.transferCalled = true
	return nil
}
func (*repositoryStub) ReconcileAccount(context.Context, uuid.UUID) (bool, error) { return true, nil }

func TestServiceValidatesMoneyBeforePersistence(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)

	require.NoError(t, service.Deposit(t.Context(), uuid.New(), "12.34"))
	assert.True(t, repository.depositCalled)
	assert.True(t, repository.depositAmount.Equal(decimal.RequireFromString("12.34")))

	repository.depositCalled = false
	require.ErrorIs(t, service.Deposit(t.Context(), uuid.New(), "12.345"), ErrInvalidAmount)
	assert.False(t, repository.depositCalled)
}

func TestServiceRejectsInvalidTransferIdentityBeforePersistence(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository)
	accountID := uuid.New()

	err := service.Transfer(t.Context(), uuid.Nil, uuid.New(), uuid.New(), "1.00")
	require.ErrorIs(t, err, ErrAccountOwnership)
	assert.False(t, repository.transferCalled)

	err = service.Transfer(t.Context(), uuid.New(), accountID, accountID, "1.00")
	require.ErrorIs(t, err, ErrSameAccountTransfer)
	assert.False(t, repository.transferCalled)
}

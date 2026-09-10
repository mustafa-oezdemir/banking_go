// Package domain contains infrastructure-free ledger invariants and posting rules.
package domain

import (
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mustafa-oezdemir/banking_go/internal/account"
)

var (
	// ErrInsufficientFunds rejects a debit larger than the available balance.
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrSameAccountTransfer rejects a posting whose two legs target one account.
	ErrSameAccountTransfer = errors.New("cannot transfer to the same account")
	// ErrInvalidAmount rejects a non-positive amount.
	ErrInvalidAmount = errors.New("amount must be positive, use at most two decimals, and fit the supported range")
	// ErrCurrencyMismatch rejects postings across currencies.
	ErrCurrencyMismatch = errors.New("currency mismatch")
	// ErrSystemAccount rejects customer access to internal settlement accounts.
	ErrSystemAccount = errors.New("system accounts cannot be used for customer operations")
	// ErrAccountOwnership rejects access outside the authenticated customer's ownership boundary.
	ErrAccountOwnership = errors.New("account ownership check failed")
	// ErrAccountBlocked rejects postings involving inactive accounts.
	ErrAccountBlocked = errors.New("account is not active")
)

// AccountSnapshot contains only the account state required by posting policy.
type AccountSnapshot struct {
	AvailableBalance decimal.Decimal
	Currency         string
	Status           string
	ID               uuid.UUID
	OwnerID          uuid.UUID
	OwnerAssigned    bool
	System           bool
}

// TransferPolicy controls the destination rules for a customer debit.
type TransferPolicy struct {
	RequireDestinationOwnership bool
	AllowSystemDestination      bool
}

// Leg is one immutable side of a balanced posting.
type Leg struct {
	Debit     decimal.Decimal
	Credit    decimal.Decimal
	AccountID uuid.UUID
}

// Posting is the balanced debit/credit plan persisted by an adapter.
type Posting struct {
	DebitLeg  Leg
	CreditLeg Leg
}

// Balanced reports whether the plan contains equal, positive, opposite legs.
func (posting Posting) Balanced() bool {
	return posting.DebitLeg.AccountID != posting.CreditLeg.AccountID &&
		posting.DebitLeg.Debit.IsPositive() && posting.DebitLeg.Credit.IsZero() &&
		posting.CreditLeg.Debit.IsZero() && posting.CreditLeg.Credit.Equal(posting.DebitLeg.Debit)
}

// PlanCustomerTransfer applies ownership, account-state, currency, balance,
// and double-entry rules without knowing PostgreSQL or HTTP.
func PlanCustomerTransfer(
	requesterID uuid.UUID,
	source AccountSnapshot,
	destination AccountSnapshot,
	amount decimal.Decimal,
	policy TransferPolicy,
) (Posting, error) {
	if !amount.IsPositive() {
		return Posting{}, ErrInvalidAmount
	}
	if source.ID == destination.ID {
		return Posting{}, ErrSameAccountTransfer
	}
	if !account.CustomerCanOperate(requesterID, source.OwnerID, source.OwnerAssigned, source.System) {
		if source.System {
			return Posting{}, ErrSystemAccount
		}
		return Posting{}, ErrAccountOwnership
	}
	if destination.System && !policy.AllowSystemDestination {
		return Posting{}, ErrSystemAccount
	}
	if policy.RequireDestinationOwnership &&
		!account.CustomerCanOperate(requesterID, destination.OwnerID, destination.OwnerAssigned, destination.System) {
		return Posting{}, ErrAccountOwnership
	}
	if source.Status != "ACTIVE" || destination.Status != "ACTIVE" {
		return Posting{}, ErrAccountBlocked
	}
	if source.Currency != destination.Currency {
		return Posting{}, ErrCurrencyMismatch
	}
	if source.AvailableBalance.LessThan(amount) {
		return Posting{}, ErrInsufficientFunds
	}

	return Posting{
		DebitLeg:  Leg{AccountID: source.ID, Debit: amount, Credit: decimal.Zero},
		CreditLeg: Leg{AccountID: destination.ID, Debit: decimal.Zero, Credit: amount},
	}, nil
}

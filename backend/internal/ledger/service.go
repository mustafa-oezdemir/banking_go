// Package ledger exposes application use cases and stable financial errors.
package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mustafa-oezdemir/banking_go/internal/account"
	ledgerdomain "github.com/mustafa-oezdemir/banking_go/internal/ledger/domain"
)

var (
	// ErrInsufficientFunds is returned when an account balance cannot cover a debit.
	ErrInsufficientFunds = ledgerdomain.ErrInsufficientFunds
	// ErrSameAccountTransfer rejects a transfer using one account for both legs.
	ErrSameAccountTransfer = ledgerdomain.ErrSameAccountTransfer
	// ErrInvalidAmount rejects non-positive, over-precise, or out-of-range amounts.
	ErrInvalidAmount = ledgerdomain.ErrInvalidAmount
	// ErrCurrencyMismatch rejects a posting across currencies.
	ErrCurrencyMismatch = ledgerdomain.ErrCurrencyMismatch
	// ErrSystemAccount rejects direct customer operations on internal accounts.
	ErrSystemAccount = ledgerdomain.ErrSystemAccount
	// ErrAccountOwnership rejects access outside the authenticated customer's accounts.
	ErrAccountOwnership = ledgerdomain.ErrAccountOwnership
	// ErrAccountBlocked rejects operations involving an inactive account.
	ErrAccountBlocked = ledgerdomain.ErrAccountBlocked
	// ErrAccountNotFound is returned when an expected account does not exist.
	ErrAccountNotFound = errors.New("account not found")
	// ErrUnsupportedBalanceOperation rejects unknown administrator operations.
	ErrUnsupportedBalanceOperation = errors.New("unsupported balance operation")
)

const signupOpeningBalance = "500.00"

// NewCustomer is the credential and display data required for atomic customer provisioning.
type NewCustomer struct {
	// IdentityID is the Identity Service subject. It is deliberately optional for
	// legacy bootstrap data but required by the private provisioning endpoint.
	IdentityID     uuid.UUID
	Email          string
	HashedPassword string
	FullName       string
}

// Customer is the application-facing identity created with a funded account.
type Customer struct {
	Email string
	ID    uuid.UUID
}

// Account is the application-facing account created during customer provisioning.
type Account struct {
	IBAN string
	ID   uuid.UUID
}

// FundedCustomer is the atomic customer-provisioning result.
type FundedCustomer struct {
	Account Account
	User    Customer
}

// Repository is the persistence port required by ledger application use cases.
// Its implementation retains the local serializable transaction boundary.
type Repository interface {
	CreateFundedCustomer(context.Context, NewCustomer, string, decimal.Decimal) (FundedCustomer, error)
	Deposit(context.Context, uuid.UUID, decimal.Decimal) error
	Withdraw(context.Context, uuid.UUID, decimal.Decimal) error
	AdjustBalanceAsAdmin(context.Context, uuid.UUID, uuid.UUID, string, decimal.Decimal, string) error
	Transfer(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, decimal.Decimal) error
	ReconcileAccount(context.Context, uuid.UUID) (bool, error)
}

// Service coordinates ledger application use cases through a persistence port.
type Service struct {
	repository Repository
}

// NewService constructs a ledger application service.
func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// CreateFundedCustomer atomically provisions a customer, account, and balanced opening credit.
func (service *Service) CreateFundedCustomer(ctx context.Context, input NewCustomer) (FundedCustomer, error) {
	iban, err := account.GenerateGermanDemoIBAN()
	if err != nil {
		return FundedCustomer{}, fmt.Errorf("generate signup account IBAN: %w", err)
	}
	return service.repository.CreateFundedCustomer(
		ctx, input, iban, decimal.RequireFromString(signupOpeningBalance),
	)
}

// Deposit posts an external credit after exact amount validation.
func (service *Service) Deposit(ctx context.Context, accountID uuid.UUID, amount string) error {
	parsed, err := ParseEURAmount(amount)
	if err != nil {
		return err
	}
	return service.repository.Deposit(ctx, accountID, parsed)
}

// Withdraw posts an external debit after exact amount validation.
func (service *Service) Withdraw(ctx context.Context, accountID uuid.UUID, amount string) error {
	parsed, err := ParseEURAmount(amount)
	if err != nil {
		return err
	}
	return service.repository.Withdraw(ctx, accountID, parsed)
}

// AdjustBalanceAsAdmin applies an audited balance operation.
func (service *Service) AdjustBalanceAsAdmin(
	ctx context.Context,
	actorID, accountID uuid.UUID,
	operation, amount, requestID string,
) error {
	parsed, err := ParseEURAmount(amount)
	if err != nil {
		return err
	}
	if operation != "DEPOSIT" && operation != "WITHDRAW" {
		return ErrUnsupportedBalanceOperation
	}
	return service.repository.AdjustBalanceAsAdmin(ctx, actorID, accountID, operation, parsed, requestID)
}

// Transfer moves funds only between accounts owned by the authenticated customer.
func (service *Service) Transfer(ctx context.Context, ownerID, fromID, toID uuid.UUID, amount string) error {
	if ownerID == uuid.Nil {
		return ErrAccountOwnership
	}
	parsed, err := ParseEURAmount(amount)
	if err != nil {
		return err
	}
	if fromID == toID {
		return ErrSameAccountTransfer
	}
	return service.repository.Transfer(ctx, ownerID, fromID, toID, parsed)
}

// ReconcileAccount compares the cached balance with immutable ledger truth.
func (service *Service) ReconcileAccount(ctx context.Context, accountID uuid.UUID) (bool, error) {
	return service.repository.ReconcileAccount(ctx, accountID)
}

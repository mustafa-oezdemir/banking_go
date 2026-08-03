// Package service contains the core ledger business logic.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	"github.com/mustafa-oezdemir/banking_go/internal/db"
	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

const signupOpeningBalance = "500.00"

var (
	// ErrInsufficientFunds is returned when an account balance cannot cover a debit.
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrSameAccountTransfer is returned when a transfer uses the same source and destination account.
	ErrSameAccountTransfer = errors.New("cannot transfer to the same account")
	// ErrInvalidAmount is returned when the provided amount is zero or negative.
	ErrInvalidAmount = errors.New("amount must be positive")
	// ErrCurrencyMismatch is returned when accounts involved in an operation use different currencies.
	ErrCurrencyMismatch = errors.New("currency mismatch")
	// ErrAccountNotFound is returned when an expected account does not exist.
	ErrAccountNotFound = errors.New("account not found")
	// ErrSystemAccount is returned when a customer operation targets an internal ledger account.
	ErrSystemAccount = errors.New("system accounts cannot be used for customer operations")
)

// LedgerService coordinates double-entry operations on accounts.
type LedgerService struct {
	store *db.Store
}

// NewLedgerService constructs a LedgerService backed by the provided store.
func NewLedgerService(store *db.Store) *LedgerService {
	return &LedgerService{store: store}
}

// CreateFundedCustomer atomically creates a customer, their default EUR account,
// and the balanced signup credit. A failed account or ledger write rolls back the user too.
func (s *LedgerService) CreateFundedCustomer(ctx context.Context, input sqlc.CreateUserParams) (sqlc.CreateUserRow, error) {
	iban, err := sepa.GenerateGermanDemoIBAN()
	if err != nil {
		return sqlc.CreateUserRow{}, fmt.Errorf("generate signup account IBAN: %w", err)
	}
	openingBalance := decimal.RequireFromString(signupOpeningBalance)
	var user sqlc.CreateUserRow
	err = s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		var txErr error
		user, txErr = q.CreateUser(ctx, input)
		if txErr != nil {
			return txErr
		}
		account, txErr := q.CreateAccount(ctx, sqlc.CreateAccountParams{
			OwnerID: uuid.NullUUID{UUID: user.ID, Valid: true},
			Name:    "Girokonto", Currency: "EUR", IsSystem: false,
			Iban: iban, AccountType: "GIROKONTO", Status: "ACTIVE",
		})
		if txErr != nil {
			return txErr
		}
		return s.depositTx(ctx, q, account.ID, openingBalance)
	})
	if err != nil {
		return sqlc.CreateUserRow{}, err
	}
	return user, nil
}

// Deposit external money into user account
func (s *LedgerService) Deposit(ctx context.Context, accountID uuid.UUID, amountStr string) error {
	// Step 1: Validate amount once at service boundary.
	amount, err := validatePositiveAmount(amountStr)
	if err != nil {
		return err
	}

	return s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		return s.depositTx(ctx, q, accountID, amount)
	})
}

func (s *LedgerService) depositTx(ctx context.Context, q *sqlc.Queries, accountID uuid.UUID, amount decimal.Decimal) error {
	// Step 2: Lock settlement + target account rows for this transaction.
	settlement, err := q.GetSettlementAccountForUpdate(ctx)
	if err != nil {
		return fmt.Errorf("settlement account not found: %w", err)
	}

	account, err := q.GetAccountForUpdate(ctx, accountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}
	if account.IsSystem {
		return ErrSystemAccount
	}
	if account.Status != "ACTIVE" {
		return ErrAccountBlocked
	}

	if account.Currency != settlement.Currency {
		return ErrCurrencyMismatch
	}

	// Step 3: Use one transaction ID to tie both ledger legs together.
	txID := uuid.New()

	// 1. Credit user account (entry)
	_, err = q.CreateEntry(ctx, sqlc.CreateEntryParams{
		AccountID:     accountID,
		Debit:         decimal.Zero.StringFixed(4),
		Credit:        amount.StringFixed(4),
		TransactionID: txID,
		OperationType: "deposit",
		Description:   sql.NullString{String: "External deposit", Valid: true},
	})
	if err != nil {
		return err
	}

	// 2. Debit settlement (opposing entry)
	_, err = q.CreateEntry(ctx, sqlc.CreateEntryParams{
		AccountID:     settlement.ID,
		Debit:         amount.StringFixed(4),
		Credit:        decimal.Zero.StringFixed(4),
		TransactionID: txID,
		OperationType: "deposit",
		Description:   sql.NullString{String: fmt.Sprintf("Deposit to account %s", accountID), Valid: true},
	})
	if err != nil {
		return err
	}

	// 3. Update cached balances atomically in the same DB transaction.
	err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
		Balance: amount.StringFixed(4),
		ID:      accountID,
	})
	if err != nil {
		return err
	}

	err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
		Balance: amount.Neg().StringFixed(4),
		ID:      settlement.ID,
	})
	if err != nil {
		return err
	}

	log.Info().Str("tx_id", txID.String()).Msg("Deposit completed")

	return nil
}

// Withdraw external money from user account
func (s *LedgerService) Withdraw(ctx context.Context, accountID uuid.UUID, amountStr string) error {
	// Step 1: Validate amount before opening expensive DB work.
	amount, err := validatePositiveAmount(amountStr)
	if err != nil {
		return err
	}

	return s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		return s.withdrawTx(ctx, q, accountID, amount)
	})
}

func (s *LedgerService) withdrawTx(ctx context.Context, q *sqlc.Queries, accountID uuid.UUID, amount decimal.Decimal) error {
	// Step 2: Lock settlement + user account to prevent concurrent balance races.
	settlement, err := q.GetSettlementAccountForUpdate(ctx)
	if err != nil {
		return fmt.Errorf("settlement account not found: %w", err)
	}

	account, err := q.GetAccountForUpdate(ctx, accountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}
	if account.IsSystem {
		return ErrSystemAccount
	}
	if account.Status != "ACTIVE" {
		return ErrAccountBlocked
	}

	if account.Currency != settlement.Currency {
		return ErrCurrencyMismatch
	}

	balanceDec, err := decimal.NewFromString(account.Balance)
	if err != nil {
		return errors.New("invalid balance")
	}

	if balanceDec.LessThan(amount) {
		// Business invariant: withdrawals cannot overdraw user funds.
		return ErrInsufficientFunds
	}

	txID := uuid.New()

	// 1. Debit user
	_, err = q.CreateEntry(ctx, sqlc.CreateEntryParams{
		AccountID:     accountID,
		Debit:         amount.StringFixed(4),
		Credit:        decimal.Zero.StringFixed(4),
		TransactionID: txID,
		OperationType: "withdrawal",
		Description:   sql.NullString{String: "External withdrawal", Valid: true},
	})
	if err != nil {
		return err
	}

	// 2. Credit settlement
	_, err = q.CreateEntry(ctx, sqlc.CreateEntryParams{
		AccountID:     settlement.ID,
		Debit:         decimal.Zero.StringFixed(4),
		Credit:        amount.StringFixed(4),
		TransactionID: txID,
		OperationType: "withdrawal",
		Description:   sql.NullString{String: fmt.Sprintf("Withdrawal from %s", accountID), Valid: true},
	})
	if err != nil {
		return err
	}

	// 3. Update cached balances after entries are written.
	err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
		Balance: amount.Neg().StringFixed(4),
		ID:      accountID,
	})
	if err != nil {
		return err
	}

	err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
		Balance: amount.StringFixed(4),
		ID:      settlement.ID,
	})
	if err != nil {
		return err
	}

	log.Info().Str("tx_id", txID.String()).Msg("Withdrawal completed")

	return nil
}

// AdjustBalanceAsAdmin performs the ledger mutation and actor-aware audit insert atomically.
func (s *LedgerService) AdjustBalanceAsAdmin(
	ctx context.Context,
	actorID, accountID uuid.UUID,
	operation, amountStr, requestID string,
) error {
	amount, err := validatePositiveAmount(amountStr)
	if err != nil {
		return err
	}
	if operation != "DEPOSIT" && operation != "WITHDRAW" {
		return errors.New("unsupported balance operation")
	}

	return s.store.ExecTxWithHandle(ctx, func(q *sqlc.Queries, executor sqlc.DBTX) error {
		before, txErr := q.GetAccount(ctx, accountID)
		if txErr != nil {
			return txErr
		}
		if operation == "DEPOSIT" {
			txErr = s.depositTx(ctx, q, accountID, amount)
		} else {
			txErr = s.withdrawTx(ctx, q, accountID, amount)
		}
		if txErr != nil {
			return txErr
		}
		after, txErr := q.GetAccount(ctx, accountID)
		if txErr != nil {
			return txErr
		}
		return db.RecordAdminAuditTx(ctx, executor, actorID, nil, &accountID,
			"ACCOUNT_BALANCE_"+operation, before.Balance, after.Balance, requestID)
	})
}

// Transfer between two user accounts
func (s *LedgerService) Transfer(ctx context.Context, fromID, toID uuid.UUID, amountStr string) error {
	// Step 1: Validate amount and reject self-transfers immediately.
	amount, err := validatePositiveAmount(amountStr)
	if err != nil {
		return err
	}

	if fromID == toID {
		return ErrSameAccountTransfer
	}

	return s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		// Step 2: Lock both accounts in the same transaction.
		fromAcc, err := q.GetAccountForUpdate(ctx, fromID)
		if err != nil {
			return err
		}

		toAcc, err := q.GetAccountForUpdate(ctx, toID)
		if err != nil {
			return err
		}
		if fromAcc.IsSystem || toAcc.IsSystem {
			return ErrSystemAccount
		}
		if fromAcc.Status != "ACTIVE" || toAcc.Status != "ACTIVE" {
			return ErrAccountBlocked
		}

		if fromAcc.Currency != toAcc.Currency {
			return ErrCurrencyMismatch
		}

		fromBalance, err := decimal.NewFromString(fromAcc.Balance)
		if err != nil {
			return errors.New("invalid from balance")
		}

		if fromBalance.LessThan(amount) {
			// Sender must have enough balance to cover transfer amount.
			return ErrInsufficientFunds
		}

		// Step 3: Single transaction ID links debit and credit entries.
		txID := uuid.New()

		// 1. Debit from
		_, err = q.CreateEntry(ctx, sqlc.CreateEntryParams{
			AccountID:     fromID,
			Debit:         amount.StringFixed(4),
			Credit:        decimal.Zero.StringFixed(4),
			TransactionID: txID,
			OperationType: "transfer",
			Description:   sql.NullString{String: fmt.Sprintf("Transfer to %s", toID), Valid: true},
		})
		if err != nil {
			return err
		}

		// 2. Credit to
		_, err = q.CreateEntry(ctx, sqlc.CreateEntryParams{
			AccountID:     toID,
			Debit:         decimal.Zero.StringFixed(4),
			Credit:        amount.StringFixed(4),
			TransactionID: txID,
			OperationType: "transfer",
			Description:   sql.NullString{String: fmt.Sprintf("Transfer from %s", fromID), Valid: true},
		})
		if err != nil {
			return err
		}

		// 3. Update cached balances for both sides of the transfer.
		err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
			Balance: amount.Neg().StringFixed(4),
			ID:      fromID,
		})
		if err != nil {
			return err
		}

		err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
			Balance: amount.StringFixed(4),
			ID:      toID,
		})
		if err != nil {
			return err
		}

		log.Info().Str("tx_id", txID.String()).Msg("Transfer completed")

		return nil
	})
}

// ReconcileAccount verifies stored balance == SUM(credits) - SUM(debits)
func (s *LedgerService) ReconcileAccount(ctx context.Context, accountID uuid.UUID) (bool, error) {
	// Step 1: Read stored balance snapshot from accounts table.
	account, err := s.store.GetAccount(ctx, accountID)
	if err != nil {
		return false, fmt.Errorf("account not found: %w", err)
	}

	// Step 2: Compute authoritative balance from immutable ledger entries.
	calculatedStr, err := s.store.GetAccountBalance(ctx, accountID)
	if err != nil {
		return false, fmt.Errorf("failed to calculate balance: %w", err)
	}

	calculated, err := decimal.NewFromString(calculatedStr)
	if err != nil {
		return false, fmt.Errorf("invalid calculated balance: %w", err)
	}

	stored, err := decimal.NewFromString(account.Balance)
	if err != nil {
		return false, fmt.Errorf("invalid stored balance: %w", err)
	}

	if !stored.Equal(calculated) {
		// Mismatch means denormalized cache drifted from ledger truth.
		log.Error().Msg("Balance mismatch detected")
		return false, fmt.Errorf("balance mismatch: stored %s, calculated %s",
			account.Balance, calculated.StringFixed(4))
	}

	log.Info().Msg("Account reconciled successfully")

	return true, nil
}

// validatePositiveAmount parses and validates that amount > 0
func validatePositiveAmount(amountStr string) (decimal.Decimal, error) {
	// Parse decimal as exact value; never use floating-point for money.
	amt, err := decimal.NewFromString(amountStr)
	if err != nil {
		return decimal.Zero, ErrInvalidAmount
	}
	if amt.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ErrInvalidAmount
	}
	return amt, nil
}

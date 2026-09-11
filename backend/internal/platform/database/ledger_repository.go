package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	"github.com/mustafa-oezdemir/banking_go/internal/ledger"
	ledgerdomain "github.com/mustafa-oezdemir/banking_go/internal/ledger/domain"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// LedgerRepository implements the ledger persistence port with PostgreSQL/sqlc.
type LedgerRepository struct {
	store *Store
}

// NewLedgerRepository constructs the PostgreSQL ledger adapter.
func NewLedgerRepository(store *Store) *LedgerRepository {
	return &LedgerRepository{store: store}
}

// CreateFundedCustomer atomically creates a customer, their default EUR account,
// and the balanced signup credit. A failed account or ledger write rolls back the user too.
func (s *LedgerRepository) CreateFundedCustomer(
	ctx context.Context,
	input ledger.NewCustomer,
	iban string,
	openingBalance decimal.Decimal,
) (ledger.FundedCustomer, error) {
	var user sqlc.CreateUserRow
	var customerAccount sqlc.Account
	err := s.store.ExecTxWithHandle(ctx, func(q *sqlc.Queries, executor sqlc.DBTX) error {
		var txErr error
		if input.IdentityID == uuid.Nil {
			user, txErr = q.CreateUser(ctx, sqlc.CreateUserParams{
				Email: input.Email, HashedPassword: input.HashedPassword, FullName: input.FullName,
			})
		} else {
			txErr = executor.QueryRowContext(ctx, `INSERT INTO users (id, email, hashed_password, full_name) VALUES ($1, $2, $3, $4) RETURNING id, email, full_name, created_at`, input.IdentityID, input.Email, "identity-managed", input.FullName).
				Scan(&user.ID, &user.Email, &user.FullName, &user.CreatedAt)
		}
		if txErr != nil {
			return txErr
		}
		customerAccount, txErr = q.CreateAccount(ctx, sqlc.CreateAccountParams{
			OwnerID: uuid.NullUUID{UUID: user.ID, Valid: true},
			Name:    "Girokonto", Currency: "EUR", IsSystem: false,
			Iban: iban, AccountType: "GIROKONTO", Status: "ACTIVE",
		})
		if txErr != nil {
			return txErr
		}
		return s.depositTx(ctx, q, customerAccount.ID, openingBalance)
	})
	if err != nil {
		return ledger.FundedCustomer{}, err
	}
	return ledger.FundedCustomer{
		User:    ledger.Customer{ID: user.ID, Email: user.Email},
		Account: ledger.Account{ID: customerAccount.ID, IBAN: customerAccount.Iban},
	}, nil
}

// Deposit external money into user account
func (s *LedgerRepository) Deposit(ctx context.Context, accountID uuid.UUID, amount decimal.Decimal) error {
	return s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		return s.depositTx(ctx, q, accountID, amount)
	})
}

func (s *LedgerRepository) depositTx(ctx context.Context, q *sqlc.Queries, accountID uuid.UUID, amount decimal.Decimal) error {
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
		return ledger.ErrSystemAccount
	}
	if account.Status != "ACTIVE" {
		return ledger.ErrAccountBlocked
	}

	if account.Currency != settlement.Currency {
		return ledger.ErrCurrencyMismatch
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
func (s *LedgerRepository) Withdraw(ctx context.Context, accountID uuid.UUID, amount decimal.Decimal) error {
	return s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		return s.withdrawTx(ctx, q, accountID, amount)
	})
}

func (s *LedgerRepository) withdrawTx(ctx context.Context, q *sqlc.Queries, accountID uuid.UUID, amount decimal.Decimal) error {
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
		return ledger.ErrSystemAccount
	}
	if account.Status != "ACTIVE" {
		return ledger.ErrAccountBlocked
	}

	if account.Currency != settlement.Currency {
		return ledger.ErrCurrencyMismatch
	}

	balanceDec, err := decimal.NewFromString(account.Balance)
	if err != nil {
		return errors.New("invalid balance")
	}

	if balanceDec.LessThan(amount) {
		// Business invariant: withdrawals cannot overdraw user funds.
		return ledger.ErrInsufficientFunds
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
func (s *LedgerRepository) AdjustBalanceAsAdmin(
	ctx context.Context,
	actorID, accountID uuid.UUID,
	operation string,
	amount decimal.Decimal,
	requestID string,
) error {
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
		return RecordAdminAuditTx(ctx, executor, actorID, nil, &accountID,
			"ACCOUNT_BALANCE_"+operation, before.Balance, after.Balance, requestID)
	})
}

// Transfer moves money only between accounts owned by the same authenticated
// customer. Transfers to another customer must use payment.Service so VoP,
// explicit confirmation, idempotency, and the payment state machine cannot be
// bypassed through this legacy endpoint.
func (s *LedgerRepository) Transfer(ctx context.Context, ownerID, fromID, toID uuid.UUID, amount decimal.Decimal) error {
	return s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		// Step 2: Lock both accounts in deterministic UUID order. This avoids
		// deadlocks when two concurrent transfers move money in opposite directions.
		accounts, err := q.ListAccountsForUpdate(ctx, []uuid.UUID{fromID, toID})
		if err != nil {
			return err
		}
		if len(accounts) != 2 {
			return ledger.ErrAccountNotFound
		}
		accountByID := map[uuid.UUID]sqlc.Account{
			accounts[0].ID: accounts[0],
			accounts[1].ID: accounts[1],
		}
		fromAcc := accountByID[fromID]
		toAcc := accountByID[toID]
		fromBalance, err := decimal.NewFromString(fromAcc.Balance)
		if err != nil {
			return errors.New("invalid from balance")
		}
		posting, err := ledgerdomain.PlanCustomerTransfer(
			ownerID,
			ledgerdomain.AccountSnapshot{
				ID: fromAcc.ID, OwnerID: fromAcc.OwnerID.UUID, OwnerAssigned: fromAcc.OwnerID.Valid,
				Currency: fromAcc.Currency, Status: fromAcc.Status, AvailableBalance: fromBalance, System: fromAcc.IsSystem,
			},
			ledgerdomain.AccountSnapshot{
				ID: toAcc.ID, OwnerID: toAcc.OwnerID.UUID, OwnerAssigned: toAcc.OwnerID.Valid,
				Currency: toAcc.Currency, Status: toAcc.Status, System: toAcc.IsSystem,
			},
			amount,
			ledgerdomain.TransferPolicy{RequireDestinationOwnership: true},
		)
		if err != nil {
			return err
		}

		// Step 3: Single transaction ID links debit and credit entries.
		txID := uuid.New()

		// 1. Debit from
		_, err = q.CreateEntry(ctx, sqlc.CreateEntryParams{
			AccountID:     posting.DebitLeg.AccountID,
			Debit:         posting.DebitLeg.Debit.StringFixed(4),
			Credit:        posting.DebitLeg.Credit.StringFixed(4),
			TransactionID: txID,
			OperationType: "transfer",
			Description:   sql.NullString{String: fmt.Sprintf("Transfer to %s", toID), Valid: true},
		})
		if err != nil {
			return err
		}

		// 2. Credit to
		_, err = q.CreateEntry(ctx, sqlc.CreateEntryParams{
			AccountID:     posting.CreditLeg.AccountID,
			Debit:         posting.CreditLeg.Debit.StringFixed(4),
			Credit:        posting.CreditLeg.Credit.StringFixed(4),
			TransactionID: txID,
			OperationType: "transfer",
			Description:   sql.NullString{String: fmt.Sprintf("Transfer from %s", fromID), Valid: true},
		})
		if err != nil {
			return err
		}

		// 3. Update cached balances for both sides of the transfer.
		err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
			Balance: posting.DebitLeg.Debit.Neg().StringFixed(4),
			ID:      posting.DebitLeg.AccountID,
		})
		if err != nil {
			return err
		}

		err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
			Balance: posting.CreditLeg.Credit.StringFixed(4),
			ID:      posting.CreditLeg.AccountID,
		})
		if err != nil {
			return err
		}

		log.Info().Str("tx_id", txID.String()).Msg("Transfer completed")

		return nil
	})
}

// ReconcileAccount verifies stored balance == SUM(credits) - SUM(debits)
func (s *LedgerRepository) ReconcileAccount(ctx context.Context, accountID uuid.UUID) (bool, error) {
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

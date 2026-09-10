package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// AdminUser is the user summary exposed to administrators.
type AdminUser struct {
	CreatedAt    time.Time
	Email        string
	FullName     string
	Role         string
	TotalBalance string
	ID           uuid.UUID
	AccountCount int64
}

// AdminAccount is the cross-customer account summary exposed to administrators.
type AdminAccount struct {
	CreatedAt        time.Time
	UpdatedAt        time.Time
	OwnerEmail       string
	OwnerName        string
	Name             string
	IBAN             string
	AccountType      string
	Status           string
	Balance          string
	AvailableBalance string
	ID               uuid.UUID
	OwnerID          uuid.UUID
}

// UpsertAdminUser creates or refreshes the configured administrator account.
func (store *Store) UpsertAdminUser(ctx context.Context, email, hashedPassword, fullName string) (uuid.UUID, error) {
	var id uuid.UUID
	err := store.db.QueryRowContext(ctx, `
		INSERT INTO users (email, hashed_password, full_name, role)
		VALUES ($1, $2, $3, 'ADMIN')
		ON CONFLICT (email) DO UPDATE SET
			hashed_password = EXCLUDED.hashed_password,
			full_name = EXCLUDED.full_name,
			role = 'ADMIN',
			session_version = users.session_version + CASE
				WHEN users.hashed_password <> EXCLUDED.hashed_password OR users.role <> 'ADMIN' THEN 1
				ELSE 0
			END
		RETURNING id`, email, hashedPassword, fullName).Scan(&id)
	return id, err
}

// GetUserRole returns the persisted authorization role for a user.
func (store *Store) GetUserRole(ctx context.Context, userID uuid.UUID) (string, error) {
	var role string
	err := store.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&role)
	return role, err
}

// ListAdminUsers returns every customer and administrator with aggregate balances.
func (store *Store) ListAdminUsers(ctx context.Context) (users []AdminUser, err error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT u.id, u.email, u.full_name, u.role, COALESCE(u.created_at, CURRENT_TIMESTAMP),
		       COUNT(a.id), COALESCE(SUM(a.balance), 0)::TEXT
		FROM users u
		LEFT JOIN accounts a ON a.owner_id = u.id AND NOT a.is_system
		GROUP BY u.id, u.email, u.full_name, u.role, u.created_at
		ORDER BY u.created_at, u.email`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil {
			err = closeErr
		}
	}()

	users = make([]AdminUser, 0)
	for rows.Next() {
		var user AdminUser
		if err = rows.Scan(&user.ID, &user.Email, &user.FullName, &user.Role, &user.CreatedAt, &user.AccountCount, &user.TotalBalance); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// ListAdminAccounts returns all non-system accounts across customers.
func (store *Store) ListAdminAccounts(ctx context.Context) (accounts []AdminAccount, err error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT a.id, a.owner_id, u.email, u.full_name, a.name, a.iban, a.account_type,
		       a.status, a.balance::TEXT, a.available_balance::TEXT, a.created_at, a.updated_at
		FROM accounts a
		JOIN users u ON u.id = a.owner_id
		WHERE NOT a.is_system
		ORDER BY u.email, a.created_at`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil {
			err = closeErr
		}
	}()

	accounts = make([]AdminAccount, 0)
	for rows.Next() {
		var account AdminAccount
		if err = rows.Scan(
			&account.ID, &account.OwnerID, &account.OwnerEmail, &account.OwnerName,
			&account.Name, &account.IBAN, &account.AccountType, &account.Status,
			&account.Balance, &account.AvailableBalance, &account.CreatedAt, &account.UpdatedAt,
		); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// CountPaymentOrders reports the number of payment orders in the demo.
func (store *Store) CountPaymentOrders(ctx context.Context) (int64, error) {
	var count int64
	err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM payment_orders`).Scan(&count)
	return count, err
}

// GetUserSessionVersion returns the server-side session generation for token revocation.
func (store *Store) GetUserSessionVersion(ctx context.Context, userID uuid.UUID) (int64, error) {
	var version int64
	err := store.db.QueryRowContext(ctx, `SELECT session_version FROM users WHERE id = $1`, userID).Scan(&version)
	return version, err
}

// RevokeUserSessions invalidates every JWT issued for a user before this call.
func (store *Store) RevokeUserSessions(ctx context.Context, userID uuid.UUID) error {
	result, err := store.db.ExecContext(ctx, `UPDATE users SET session_version = session_version + 1 WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateUserRole changes a role, revokes existing sessions, and appends a durable audit event atomically.
func (store *Store) UpdateUserRole(ctx context.Context, actorID, userID uuid.UUID, role, requestID string) error {
	return store.ExecTxWithHandle(ctx, func(_ *sqlc.Queries, executor sqlc.DBTX) error {
		var previous string
		if err := executor.QueryRowContext(ctx, `SELECT role FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&previous); err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `
			UPDATE users SET role = $2, session_version = session_version + 1 WHERE id = $1`, userID, role); err != nil {
			return err
		}
		return insertAdminAudit(ctx, executor, actorID, &userID, nil, "USER_ROLE_UPDATED", previous, role, requestID)
	})
}

// UpdateAdminAccountStatus updates a customer account and appends a durable audit event atomically.
func (store *Store) UpdateAdminAccountStatus(ctx context.Context, actorID, accountID uuid.UUID, status, requestID string) error {
	return store.ExecTxWithHandle(ctx, func(_ *sqlc.Queries, executor sqlc.DBTX) error {
		var previous string
		if err := executor.QueryRowContext(ctx, `
			SELECT status FROM accounts WHERE id = $1 AND NOT is_system FOR UPDATE`, accountID).Scan(&previous); err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `
			UPDATE accounts SET status = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, accountID, status); err != nil {
			return err
		}
		return insertAdminAudit(ctx, executor, actorID, nil, &accountID, "ACCOUNT_STATUS_UPDATED", previous, status, requestID)
	})
}

// CountAdminAuditEvents is used by integration tests and operational checks.
func (store *Store) CountAdminAuditEvents(ctx context.Context) (int64, error) {
	var count int64
	err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_audit_events`).Scan(&count)
	return count, err
}

func insertAdminAudit(
	ctx context.Context,
	executor sqlc.DBTX,
	actorID uuid.UUID,
	targetUserID, targetAccountID *uuid.UUID,
	action, previous, next, requestID string,
) error {
	_, err := executor.ExecContext(ctx, `
		INSERT INTO admin_audit_events (
			actor_user_id, target_user_id, target_account_id, action,
			before_data, after_data, request_id
		) VALUES ($1, $2, $3, $4,
			jsonb_build_object('value', $5::TEXT),
			jsonb_build_object('value', $6::TEXT), NULLIF($7, ''))`,
		actorID, targetUserID, targetAccountID, action, previous, next, requestID)
	return err
}

// RecordAdminAuditTx appends an audit event using the caller's transaction handle.
func RecordAdminAuditTx(
	ctx context.Context,
	executor sqlc.DBTX,
	actorID uuid.UUID,
	targetUserID, targetAccountID *uuid.UUID,
	action, previous, next, requestID string,
) error {
	return insertAdminAudit(ctx, executor, actorID, targetUserID, targetAccountID,
		action, previous, next, requestID)
}

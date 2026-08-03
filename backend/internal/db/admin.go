package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// AdminUser is the user summary exposed to administrators.
type AdminUser struct {
	CreatedAt    time.Time
	ID           uuid.UUID
	Email        string
	FullName     string
	Role         string
	TotalBalance string
	AccountCount int64
}

// AdminAccount is the cross-customer account summary exposed to administrators.
type AdminAccount struct {
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ID               uuid.UUID
	OwnerID          uuid.UUID
	OwnerEmail       string
	OwnerName        string
	Name             string
	IBAN             string
	AccountType      string
	Status           string
	Balance          string
	AvailableBalance string
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
			role = 'ADMIN'
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
func (store *Store) ListAdminUsers(ctx context.Context) ([]AdminUser, error) {
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
	defer rows.Close()

	users := make([]AdminUser, 0)
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
func (store *Store) ListAdminAccounts(ctx context.Context) ([]AdminAccount, error) {
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
	defer rows.Close()

	accounts := make([]AdminAccount, 0)
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

// UpdateUserRole changes a user's authorization role.
func (store *Store) UpdateUserRole(ctx context.Context, userID uuid.UUID, role string) error {
	result, err := store.db.ExecContext(ctx, `UPDATE users SET role = $2 WHERE id = $1`, userID, role)
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

// UpdateAdminAccountStatus activates or blocks any customer account.
func (store *Store) UpdateAdminAccountStatus(ctx context.Context, accountID uuid.UUID, status string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE accounts SET status = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND NOT is_system`, accountID, status)
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

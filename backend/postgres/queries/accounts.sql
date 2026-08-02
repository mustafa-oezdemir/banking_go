-- name: CreateAccount :one
INSERT INTO accounts (owner_id, name, currency, is_system, iban, account_type, status, available_balance)
VALUES ($1, $2, $3, $4, $5, $6, $7, 0.0000)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts
WHERE id = $1
LIMIT 1;

-- name: UpdateAccount :one
UPDATE accounts
SET name = sqlc.arg(name)
WHERE accounts.id = sqlc.arg(account_id)
  AND accounts.owner_id = sqlc.arg(owner_id)
  AND accounts.is_system = FALSE
RETURNING accounts.*;

-- name: AccountHasEntries :one
SELECT EXISTS (
    SELECT 1
    FROM entries
    WHERE account_id = sqlc.arg(account_id)
);

-- name: DeleteAccount :one
DELETE FROM accounts
WHERE accounts.id = sqlc.arg(account_id)
  AND accounts.owner_id = sqlc.arg(owner_id)
  AND accounts.is_system = FALSE
  AND accounts.balance = 0
  AND NOT EXISTS (
      SELECT 1
      FROM entries
      WHERE entries.account_id = accounts.id
  )
RETURNING accounts.*;

-- name: GetAccountForUpdate :one
SELECT * FROM accounts
WHERE id = $1
LIMIT 1
FOR UPDATE; -- locks row for update, prevents TOCTOU races

-- name: ListAccountsByOwner :many
SELECT * FROM accounts
WHERE owner_id = $1
ORDER BY created_at DESC;

-- name: UpdateAccountBalance :exec
UPDATE accounts
SET balance = balance + $1,
    available_balance = available_balance + $1,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $2;

-- name: GetAccountByIBAN :one
SELECT * FROM accounts
WHERE iban = $1
LIMIT 1;

-- name: GetAccountByIBANForUpdate :one
SELECT * FROM accounts
WHERE iban = $1
LIMIT 1
FOR UPDATE;

-- name: ListAccountsForUpdate :many
SELECT * FROM accounts
WHERE id = ANY(sqlc.arg(account_ids)::UUID[])
ORDER BY id
FOR UPDATE;

-- name: GetSettlementAccount :one
SELECT * FROM accounts
WHERE is_system = TRUE AND account_type = 'SETTLEMENT'
LIMIT 1;

-- name: GetSettlementAccountForUpdate :one
SELECT * FROM accounts
WHERE is_system = TRUE AND account_type = 'SETTLEMENT'
LIMIT 1
FOR UPDATE; -- lock prevents concurrent transactions from reading a stale balance.

-- name: GetAccountBalance :one
SELECT CAST((COALESCE(SUM(credit), 0::NUMERIC) - COALESCE(SUM(debit), 0::NUMERIC)) AS NUMERIC(19,4)) AS calculated_balance
FROM entries
WHERE account_id = $1;

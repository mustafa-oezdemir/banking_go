-- name: CreateEntry :one
INSERT INTO entries (account_id, debit, credit, transaction_id, operation_type, description)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CreatePaymentEntry :one
INSERT INTO entries (
    account_id, debit, credit, transaction_id, operation_type, description,
    payment_order_id, counterparty_name, counterparty_iban, purpose, category,
    booking_date, execution_date
)
VALUES (
    $1, $2, $3, $4, 'transfer', $5,
    $6, $7, $8, $9, $10, CURRENT_TIMESTAMP, $11
)
RETURNING *;

-- name: ListEntriesByAccount :many
SELECT * FROM entries
WHERE account_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAccountTransactions :many
SELECT * FROM entries
WHERE account_id = sqlc.arg(account_id)
  AND (sqlc.narg(direction)::TEXT IS NULL OR
       (sqlc.narg(direction) = 'INCOMING' AND credit > 0) OR
       (sqlc.narg(direction) = 'OUTGOING' AND debit > 0))
  AND (sqlc.narg(status)::TEXT IS NULL OR EXISTS (
        SELECT 1 FROM payment_orders po
        WHERE po.id = entries.payment_order_id AND po.status = sqlc.narg(status)
      ))
  AND (sqlc.narg(category)::TEXT IS NULL OR entries.category = sqlc.narg(category))
  AND (sqlc.narg(date_from)::TIMESTAMPTZ IS NULL OR entries.created_at >= sqlc.narg(date_from))
  AND (sqlc.narg(date_to)::TIMESTAMPTZ IS NULL OR entries.created_at <= sqlc.narg(date_to))
  AND (sqlc.narg(min_amount)::NUMERIC IS NULL OR GREATEST(entries.debit, entries.credit) >= sqlc.narg(min_amount))
  AND (sqlc.narg(max_amount)::NUMERIC IS NULL OR GREATEST(entries.debit, entries.credit) <= sqlc.narg(max_amount))
ORDER BY entries.created_at DESC
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: ListEntriesByTransaction :many
SELECT * FROM entries
WHERE transaction_id = $1
ORDER BY created_at;

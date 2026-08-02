-- name: CreateStandingOrder :one
INSERT INTO standing_orders (
    owner_id, source_account_id, beneficiary_account_id, beneficiary_name,
    beneficiary_iban, beneficiary_bic, amount, currency, purpose,
    creditor_reference, transfer_type, frequency, start_date, end_date,
    max_occurrences, next_execution_at
)
VALUES (
    sqlc.arg(owner_id), sqlc.arg(source_account_id), sqlc.narg(beneficiary_account_id), sqlc.arg(beneficiary_name),
    sqlc.arg(beneficiary_iban), sqlc.narg(beneficiary_bic), sqlc.arg(amount), 'EUR', sqlc.narg(purpose),
    sqlc.narg(creditor_reference), sqlc.arg(transfer_type), sqlc.arg(frequency), sqlc.arg(start_date), sqlc.narg(end_date),
    sqlc.narg(max_occurrences), sqlc.arg(next_execution_at)
)
RETURNING *;

-- name: GetStandingOrder :one
SELECT * FROM standing_orders WHERE id = $1 LIMIT 1;

-- name: ListStandingOrdersByOwner :many
SELECT * FROM standing_orders WHERE owner_id = $1 ORDER BY created_at DESC;

-- name: UpdateStandingOrder :one
UPDATE standing_orders
SET amount = sqlc.arg(amount), purpose = sqlc.narg(purpose),
    end_date = sqlc.narg(end_date), max_occurrences = sqlc.narg(max_occurrences),
    status = sqlc.arg(status), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(standing_order_id) AND owner_id = sqlc.arg(owner_id)
RETURNING *;

-- name: DeleteStandingOrder :one
UPDATE standing_orders
SET status = 'CANCELLED', updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(standing_order_id) AND owner_id = sqlc.arg(owner_id)
  AND status IN ('ACTIVE', 'PAUSED')
RETURNING *;

-- name: ClaimDueStandingOrders :many
SELECT * FROM standing_orders
WHERE status = 'ACTIVE' AND next_execution_at <= CURRENT_TIMESTAMP
ORDER BY next_execution_at, id
FOR UPDATE SKIP LOCKED
LIMIT sqlc.arg(batch_size);

-- name: AdvanceStandingOrder :one
UPDATE standing_orders
SET occurrences_created = occurrences_created + 1,
    next_execution_at = sqlc.arg(next_execution_at),
    status = sqlc.arg(status), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(standing_order_id) AND status = 'ACTIVE'
RETURNING *;

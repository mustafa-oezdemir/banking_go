-- name: CreatePaymentOrder :one
INSERT INTO payment_orders (
    owner_id, source_account_id, beneficiary_account_id, standing_order_id,
    beneficiary_name, beneficiary_iban, beneficiary_bic, amount, currency,
    payment_kind, schedule_type, purpose, creditor_reference, end_to_end_id,
    idempotency_key, requested_execution_at, vop_result, vop_suggested_name,
    status
)
VALUES (
    sqlc.arg(owner_id), sqlc.arg(source_account_id), sqlc.narg(beneficiary_account_id), sqlc.narg(standing_order_id),
    sqlc.arg(beneficiary_name), sqlc.arg(beneficiary_iban), sqlc.narg(beneficiary_bic), sqlc.arg(amount), 'EUR',
    sqlc.arg(payment_kind), sqlc.arg(schedule_type), sqlc.narg(purpose), sqlc.narg(creditor_reference), sqlc.arg(end_to_end_id),
    sqlc.arg(idempotency_key), sqlc.arg(requested_execution_at), sqlc.arg(vop_result), sqlc.narg(vop_suggested_name),
    sqlc.arg(status)
)
RETURNING *;

-- name: GetPaymentOrder :one
SELECT * FROM payment_orders WHERE id = $1 LIMIT 1;

-- name: GetPaymentOrderForUpdate :one
SELECT * FROM payment_orders WHERE id = $1 LIMIT 1 FOR UPDATE;

-- name: GetPaymentOrderByIdempotency :one
SELECT * FROM payment_orders
WHERE owner_id = sqlc.arg(owner_id) AND idempotency_key = sqlc.arg(idempotency_key)
LIMIT 1;

-- name: ListPaymentOrdersByOwner :many
SELECT * FROM payment_orders
WHERE owner_id = sqlc.arg(owner_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(result_limit) OFFSET sqlc.arg(result_offset);

-- name: ConfirmPaymentOrder :one
UPDATE payment_orders
SET status = sqlc.arg(status),
    vop_overridden = sqlc.arg(vop_overridden),
    vop_decision_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(payment_order_id)
  AND owner_id = sqlc.arg(owner_id)
  AND status = 'AWAITING_CONFIRMATION'
RETURNING *;

-- name: CancelPaymentOrder :one
UPDATE payment_orders
SET status = 'CANCELLED', updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(payment_order_id)
  AND owner_id = sqlc.arg(owner_id)
  AND status IN ('DRAFT', 'AWAITING_CONFIRMATION', 'SCHEDULED')
RETURNING *;

-- name: MarkPaymentProcessing :one
UPDATE payment_orders
SET status = 'PROCESSING', processing_started_at = CURRENT_TIMESTAMP,
    attempt_count = attempt_count + 1, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(payment_order_id)
  AND status IN ('AWAITING_CONFIRMATION', 'SCHEDULED')
RETURNING *;

-- name: MarkPaymentBooked :one
UPDATE payment_orders
SET status = 'BOOKED', ledger_transaction_id = sqlc.arg(ledger_transaction_id),
    processed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP,
    failure_reason = NULL, reject_code = NULL
WHERE id = sqlc.arg(payment_order_id)
  AND status = 'PROCESSING'
  AND ledger_transaction_id IS NULL
RETURNING *;

-- name: MarkPaymentFailed :one
UPDATE payment_orders
SET status = 'FAILED', failure_reason = sqlc.arg(failure_reason),
    reject_code = sqlc.arg(reject_code), processed_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(payment_order_id) AND status = 'PROCESSING'
RETURNING *;

-- name: ClaimDuePaymentOrders :many
WITH due AS (
    SELECT id
    FROM payment_orders
    WHERE status = 'SCHEDULED' AND requested_execution_at <= CURRENT_TIMESTAMP
    ORDER BY requested_execution_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE payment_orders AS payment
SET status = 'PROCESSING', processing_started_at = CURRENT_TIMESTAMP,
    attempt_count = payment.attempt_count + 1, updated_at = CURRENT_TIMESTAMP
FROM due
WHERE payment.id = due.id
RETURNING payment.*;

-- name: RecoverStalePayments :execrows
UPDATE payment_orders
SET status = 'SCHEDULED', processing_started_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE status = 'PROCESSING'
  AND ledger_transaction_id IS NULL
  AND processing_started_at < CURRENT_TIMESTAMP - make_interval(secs => sqlc.arg(stale_seconds)::INT);

-- name: LookupPayeeByIBAN :one
SELECT accounts.id AS account_id, accounts.owner_id, users.full_name, accounts.status
FROM accounts
JOIN users ON users.id = accounts.owner_id
WHERE accounts.iban = sqlc.arg(iban) AND accounts.is_system = FALSE
LIMIT 1;

-- name: GetBeneficiaryByIBAN :one
SELECT * FROM beneficiaries
WHERE owner_id = sqlc.arg(owner_id) AND iban = sqlc.arg(iban)
LIMIT 1;

-- name: CreateAuditEvent :one
INSERT INTO audit_events (owner_id, payment_order_id, event_type, event_data)
VALUES (sqlc.narg(owner_id), sqlc.narg(payment_order_id), sqlc.arg(event_type), sqlc.arg(event_data))
RETURNING *;

-- name: ListAuditEventsAfter :many
SELECT * FROM audit_events
WHERE owner_id = sqlc.arg(owner_id) AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(result_limit);

-- name: CreateBeneficiary :one
INSERT INTO beneficiaries (owner_id, name, iban, bic, category)
VALUES (sqlc.arg(owner_id), sqlc.arg(name), sqlc.arg(iban), sqlc.narg(bic), sqlc.narg(category))
ON CONFLICT (owner_id, iban) DO UPDATE
SET name = EXCLUDED.name, bic = EXCLUDED.bic, category = EXCLUDED.category,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: ListBeneficiaries :many
SELECT * FROM beneficiaries WHERE owner_id = $1 ORDER BY name;

-- name: DeleteBeneficiary :execrows
DELETE FROM beneficiaries WHERE id = sqlc.arg(beneficiary_id) AND owner_id = sqlc.arg(owner_id);

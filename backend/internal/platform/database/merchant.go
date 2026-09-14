package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ErrMerchantNotFound is returned when a merchant or intent does not exist.
var ErrMerchantNotFound = errors.New("merchant resource not found")

// Merchant is the trusted server-side payment destination configuration.
type Merchant struct {
	ID                   string
	Name                 string
	BeneficiaryAccountID uuid.UUID
	BeneficiaryIBAN      string
	ReturnURL            string
	WebhookURL           string
	WebhookSecret        string
	Active               bool
}

// MerchantPaymentIntent is the immutable checkout instruction stored by Banking.
type MerchantPaymentIntent struct {
	ID                uuid.UUID
	MerchantID        string
	MerchantName      string
	BeneficiaryIBAN   string
	MerchantReference string
	Amount            decimal.Decimal
	Currency          string
	Status            string
	PaymentOrderID    uuid.NullUUID
	ReturnURL         string
	WebhookURL        string
	WebhookSecret     string
	ExpiresAt         time.Time
	CreatedAt         time.Time
}

// GetMerchant returns an active or inactive merchant configuration by stable ID.
func (store *Store) GetMerchant(ctx context.Context, merchantID string) (Merchant, error) {
	var merchant Merchant
	err := store.db.QueryRowContext(ctx, `
		SELECT m.id, m.name, m.beneficiary_account_id, a.iban, m.return_url, COALESCE(m.webhook_url, ''), COALESCE(m.webhook_secret, ''), m.active
		FROM merchants m
		JOIN accounts a ON a.id = m.beneficiary_account_id
		WHERE m.id = $1
	`, merchantID).Scan(
		&merchant.ID, &merchant.Name, &merchant.BeneficiaryAccountID, &merchant.BeneficiaryIBAN,
		&merchant.ReturnURL, &merchant.WebhookURL, &merchant.WebhookSecret, &merchant.Active,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Merchant{}, ErrMerchantNotFound
	}
	return merchant, err
}

// CreateMerchantPaymentIntent inserts one immutable checkout instruction.
func (store *Store) CreateMerchantPaymentIntent(
	ctx context.Context, merchant Merchant, reference string, amount decimal.Decimal, expiresAt time.Time,
) (MerchantPaymentIntent, error) {
	var intent MerchantPaymentIntent
	err := store.db.QueryRowContext(ctx, `
		INSERT INTO merchant_payment_intents (merchant_id, merchant_reference, amount, currency, expires_at)
		VALUES ($1, $2, $3, 'EUR', $4)
		RETURNING id, merchant_id, merchant_reference, amount, currency, status, payment_order_id, expires_at, created_at
	`, merchant.ID, reference, amount, expiresAt).Scan(
		&intent.ID, &intent.MerchantID, &intent.MerchantReference, &intent.Amount, &intent.Currency,
		&intent.Status, &intent.PaymentOrderID, &intent.ExpiresAt, &intent.CreatedAt,
	)
	if err != nil {
		return MerchantPaymentIntent{}, err
	}
	intent.MerchantName, intent.BeneficiaryIBAN, intent.ReturnURL, intent.WebhookURL, intent.WebhookSecret = merchant.Name, merchant.BeneficiaryIBAN, merchant.ReturnURL, merchant.WebhookURL, merchant.WebhookSecret
	return intent, nil
}

// GetMerchantPaymentIntent returns checkout data joined with its merchant config.
func (store *Store) GetMerchantPaymentIntent(ctx context.Context, intentID uuid.UUID) (MerchantPaymentIntent, error) {
	var intent MerchantPaymentIntent
	err := store.db.QueryRowContext(ctx, `
		SELECT i.id, i.merchant_id, m.name, a.iban, i.merchant_reference, i.amount, i.currency,
		       i.status, i.payment_order_id, m.return_url, COALESCE(m.webhook_url, ''), COALESCE(m.webhook_secret, ''), i.expires_at, i.created_at
		FROM merchant_payment_intents i
		JOIN merchants m ON m.id = i.merchant_id
		JOIN accounts a ON a.id = m.beneficiary_account_id
		WHERE i.id = $1
	`, intentID).Scan(
		&intent.ID, &intent.MerchantID, &intent.MerchantName, &intent.BeneficiaryIBAN,
		&intent.MerchantReference, &intent.Amount, &intent.Currency, &intent.Status,
		&intent.PaymentOrderID, &intent.ReturnURL, &intent.WebhookURL, &intent.WebhookSecret, &intent.ExpiresAt, &intent.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MerchantPaymentIntent{}, ErrMerchantNotFound
	}
	return intent, err
}

// GetMerchantPaymentIntentByMerchantReference supports safe merchant retries.
func (store *Store) GetMerchantPaymentIntentByMerchantReference(ctx context.Context, merchantID, reference string) (MerchantPaymentIntent, error) {
	var intent MerchantPaymentIntent
	err := store.db.QueryRowContext(ctx, `
		SELECT i.id, i.merchant_id, m.name, a.iban, i.merchant_reference, i.amount, i.currency,
		       i.status, i.payment_order_id, m.return_url, COALESCE(m.webhook_url, ''), COALESCE(m.webhook_secret, ''), i.expires_at, i.created_at
		FROM merchant_payment_intents i
		JOIN merchants m ON m.id = i.merchant_id
		JOIN accounts a ON a.id = m.beneficiary_account_id
		WHERE i.merchant_id = $1 AND i.merchant_reference = $2
	`, merchantID, reference).Scan(
		&intent.ID, &intent.MerchantID, &intent.MerchantName, &intent.BeneficiaryIBAN,
		&intent.MerchantReference, &intent.Amount, &intent.Currency, &intent.Status,
		&intent.PaymentOrderID, &intent.ReturnURL, &intent.WebhookURL, &intent.WebhookSecret, &intent.ExpiresAt, &intent.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MerchantPaymentIntent{}, ErrMerchantNotFound
	}
	return intent, err
}

// ConfigureMerchantWebhook updates the local merchant callback configuration at boot.
func (store *Store) ConfigureMerchantWebhook(ctx context.Context, merchantID, webhookURL, webhookSecret string) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE merchants SET webhook_url = $2, webhook_secret = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $1
	`, merchantID, webhookURL, webhookSecret)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrMerchantNotFound
	}
	return nil
}

// LinkMerchantIntentPayment binds an intent to the single payment order created from it.
func (store *Store) LinkMerchantIntentPayment(ctx context.Context, intentID, paymentID uuid.UUID) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE merchant_payment_intents
		SET payment_order_id = $2, status = 'PROCESSING', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = 'AWAITING_CUSTOMER' AND payment_order_id IS NULL
	`, intentID, paymentID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrMerchantNotFound
	}
	return nil
}

// SetMerchantIntentStatus records the result after the payment service commits.
func (store *Store) SetMerchantIntentStatus(ctx context.Context, intentID uuid.UUID, status string) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE merchant_payment_intents
		SET status = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, intentID, status)
	return err
}

// ExpireMerchantPaymentIntent marks an unapproved expired intent as terminal.
func (store *Store) ExpireMerchantPaymentIntent(ctx context.Context, intentID uuid.UUID, now time.Time) error {
	_, err := store.db.ExecContext(ctx, `
		UPDATE merchant_payment_intents
		SET status = 'EXPIRED', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status = 'AWAITING_CUSTOMER' AND expires_at <= $2
	`, intentID, now)
	return err
}

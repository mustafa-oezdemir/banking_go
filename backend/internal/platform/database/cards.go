package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrCardNotFound = errors.New("card resource not found")

type PaymentCard struct {
	ID                uuid.UUID
	OwnerID           uuid.UUID
	AccountID         uuid.UUID
	PANFingerprint    []byte
	LastFour          string
	Brand             string
	ExpMonth          int
	ExpYear           int
	CVCHash           string
	CredentialVersion int
	EncryptedPAN      []byte
	EncryptedCVC      []byte
	Status            string
	CreatedAt         time.Time
}

type MerchantCardToken struct {
	ID               uuid.UUID
	MerchantID       string
	Card             PaymentCard
	TokenFingerprint []byte
}

func (store *Store) CreatePaymentCard(ctx context.Context, card PaymentCard) (PaymentCard, error) {
	err := store.db.QueryRowContext(ctx, `
		INSERT INTO payment_cards (id, owner_id, account_id, pan_fingerprint, last_four, brand, exp_month, exp_year, cvc_hash, credential_version, encrypted_pan, encrypted_cvc)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at
	`, card.ID, card.OwnerID, card.AccountID, card.PANFingerprint, card.LastFour, card.Brand, card.ExpMonth, card.ExpYear, card.CVCHash, card.CredentialVersion, card.EncryptedPAN, card.EncryptedCVC).
		Scan(&card.ID, &card.CreatedAt)
	return card, err
}

func (store *Store) ListPaymentCardsByOwner(ctx context.Context, ownerID uuid.UUID) ([]PaymentCard, error) {
	rows, err := store.db.QueryContext(ctx, `
		SELECT id, owner_id, account_id, last_four, brand, exp_month, exp_year, status, credential_version, created_at
		FROM payment_cards WHERE owner_id = $1 ORDER BY created_at DESC
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []PaymentCard
	for rows.Next() {
		var card PaymentCard
		if err := rows.Scan(&card.ID, &card.OwnerID, &card.AccountID, &card.LastFour, &card.Brand, &card.ExpMonth, &card.ExpYear, &card.Status, &card.CredentialVersion, &card.CreatedAt); err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func (store *Store) GetPaymentCardByFingerprint(ctx context.Context, fingerprint []byte) (PaymentCard, error) {
	var card PaymentCard
	err := store.db.QueryRowContext(ctx, `
		SELECT id, owner_id, account_id, pan_fingerprint, last_four, brand, exp_month, exp_year, cvc_hash, status, credential_version, encrypted_pan, encrypted_cvc, created_at
		FROM payment_cards WHERE pan_fingerprint = $1
	`, fingerprint).Scan(&card.ID, &card.OwnerID, &card.AccountID, &card.PANFingerprint, &card.LastFour, &card.Brand, &card.ExpMonth, &card.ExpYear, &card.CVCHash, &card.Status, &card.CredentialVersion, &card.EncryptedPAN, &card.EncryptedCVC, &card.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentCard{}, ErrCardNotFound
	}
	return card, err
}

// GetPaymentCardByIDAndOwner keeps card ownership enforcement at the database boundary.
func (store *Store) GetPaymentCardByIDAndOwner(ctx context.Context, cardID, ownerID uuid.UUID) (PaymentCard, error) {
	var card PaymentCard
	err := store.db.QueryRowContext(ctx, `
		SELECT id, owner_id, account_id, pan_fingerprint, last_four, brand, exp_month, exp_year, cvc_hash, status, credential_version, encrypted_pan, encrypted_cvc, created_at
		FROM payment_cards WHERE id = $1 AND owner_id = $2
	`, cardID, ownerID).Scan(&card.ID, &card.OwnerID, &card.AccountID, &card.PANFingerprint, &card.LastFour, &card.Brand, &card.ExpMonth, &card.ExpYear, &card.CVCHash, &card.Status, &card.CredentialVersion, &card.EncryptedPAN, &card.EncryptedCVC, &card.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentCard{}, ErrCardNotFound
	}
	return card, err
}

func (store *Store) CancelPaymentCard(ctx context.Context, ownerID, cardID uuid.UUID) error {
	result, err := store.db.ExecContext(ctx, `UPDATE payment_cards SET status = 'CANCELLED', updated_at = CURRENT_TIMESTAMP WHERE id = $1 AND owner_id = $2 AND status IN ('ACTIVE', 'BLOCKED', 'EXPIRED')`, cardID, ownerID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return ErrCardNotFound
	}
	return nil
}

func (store *Store) DeletePaymentCard(ctx context.Context, ownerID, cardID uuid.UUID) error {
	result, err := store.db.ExecContext(ctx, `DELETE FROM payment_cards WHERE id = $1 AND owner_id = $2 AND status = 'CANCELLED'`, cardID, ownerID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return ErrCardNotFound
	}
	return nil
}

func (store *Store) UpsertMerchantCardToken(ctx context.Context, merchantID string, cardID uuid.UUID, fingerprint []byte) error {
	_, err := store.db.ExecContext(ctx, `
		INSERT INTO merchant_card_tokens (merchant_id, card_id, token_fingerprint)
		VALUES ($1, $2, $3)
		ON CONFLICT (merchant_id, card_id) DO UPDATE SET token_fingerprint = EXCLUDED.token_fingerprint, created_at = CURRENT_TIMESTAMP
	`, merchantID, cardID, fingerprint)
	return err
}

func (store *Store) GetCardByMerchantToken(ctx context.Context, merchantID string, fingerprint []byte) (MerchantCardToken, error) {
	var token MerchantCardToken
	err := store.db.QueryRowContext(ctx, `
		SELECT t.id, t.merchant_id, t.token_fingerprint,
		       c.id, c.owner_id, c.account_id, c.pan_fingerprint, c.last_four, c.brand, c.exp_month, c.exp_year, c.cvc_hash, c.status, c.credential_version, c.encrypted_pan, c.encrypted_cvc, c.created_at
		FROM merchant_card_tokens t JOIN payment_cards c ON c.id = t.card_id
		WHERE t.merchant_id = $1 AND t.token_fingerprint = $2
	`, merchantID, fingerprint).Scan(&token.ID, &token.MerchantID, &token.TokenFingerprint,
		&token.Card.ID, &token.Card.OwnerID, &token.Card.AccountID, &token.Card.PANFingerprint, &token.Card.LastFour, &token.Card.Brand,
		&token.Card.ExpMonth, &token.Card.ExpYear, &token.Card.CVCHash, &token.Card.Status, &token.Card.CredentialVersion, &token.Card.EncryptedPAN, &token.Card.EncryptedCVC, &token.Card.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MerchantCardToken{}, ErrCardNotFound
	}
	return token, err
}

func (store *Store) TouchMerchantCardToken(ctx context.Context, tokenID uuid.UUID) error {
	_, err := store.db.ExecContext(ctx, `UPDATE merchant_card_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = $1`, tokenID)
	return err
}

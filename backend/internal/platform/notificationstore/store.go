// Package notificationstore persists Notification-owned consumer state.
package notificationstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const processingLease = 2 * time.Minute

// Store only accesses notification.processed_events. It never reads Banking
// users, accounts, payments, or ledger data.
type Store struct{ db *sql.DB }

// ClaimState describes whether a consumer may deliver an event.
type ClaimState string

const (
	Claimed          ClaimState = "claimed"
	AlreadyProcessed ClaimState = "already_processed"
	Busy             ClaimState = "busy"
)

// New creates the Notification-owned idempotency store.
func New(database *sql.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("notification store database is required")
	}
	return &Store{db: database}, nil
}

// Claim atomically reserves an event. Busy records are not acknowledged: the
// broker must retry them after the other consumer's lease resolves.
func (store *Store) Claim(ctx context.Context, eventID uuid.UUID) (ClaimState, error) {
	if eventID == uuid.Nil {
		return "", errors.New("notification event ID is required")
	}
	var claimed uuid.UUID
	err := store.db.QueryRowContext(ctx, `
		INSERT INTO notification.processed_events (event_id, processing_until)
		VALUES ($1, CURRENT_TIMESTAMP + $2::INTERVAL)
		ON CONFLICT (event_id) DO UPDATE
		SET processing_until = CURRENT_TIMESTAMP + $2::INTERVAL,
			attempts = notification.processed_events.attempts + 1,
			last_error = NULL
		WHERE notification.processed_events.processed_at IS NULL
		  AND notification.processed_events.processing_until < CURRENT_TIMESTAMP
		RETURNING event_id`, eventID, processingLease.String()).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		var processedAt sql.NullTime
		var processingUntil time.Time
		if lookupErr := store.db.QueryRowContext(ctx, `
			SELECT processed_at, processing_until FROM notification.processed_events WHERE event_id = $1`, eventID,
		).Scan(&processedAt, &processingUntil); lookupErr != nil {
			return "", fmt.Errorf("inspect notification event claim: %w", lookupErr)
		}
		if processedAt.Valid {
			return AlreadyProcessed, nil
		}
		return Busy, nil
	}
	if err != nil {
		return "", fmt.Errorf("claim notification event: %w", err)
	}
	if claimed != eventID {
		return "", errors.New("unexpected notification claim ID")
	}
	return Claimed, nil
}

// MarkProcessed permanently records successful delivery before the RabbitMQ ack.
func (store *Store) MarkProcessed(ctx context.Context, eventID uuid.UUID) error {
	result, err := store.db.ExecContext(ctx, `
		UPDATE notification.processed_events
		SET processed_at = CURRENT_TIMESTAMP, processing_until = CURRENT_TIMESTAMP, last_error = NULL
		WHERE event_id = $1 AND processed_at IS NULL`, eventID)
	if err != nil {
		return fmt.Errorf("mark notification event processed: %w", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return fmt.Errorf("read processed notification rows: %w", err)
		}
		return errors.New("notification event claim was lost")
	}
	return nil
}

// Release ends a failed lease so a delayed RabbitMQ retry can claim it.
func (store *Store) Release(ctx context.Context, eventID uuid.UUID, reason string) error {
	if len(reason) > 300 {
		reason = reason[:300]
	}
	_, err := store.db.ExecContext(ctx, `
		UPDATE notification.processed_events
		SET processing_until = CURRENT_TIMESTAMP, last_error = $2
		WHERE event_id = $1 AND processed_at IS NULL`, eventID, reason)
	if err != nil {
		return fmt.Errorf("release notification event: %w", err)
	}
	return nil
}

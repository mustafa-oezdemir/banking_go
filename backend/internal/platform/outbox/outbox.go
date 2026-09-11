// Package outbox persists Banking events atomically and publishes them only
// after their database transaction has committed.
package outbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

const (
	defaultLease    = 30 * time.Second
	defaultBatch    = 25
	publishInterval = 500 * time.Millisecond
)

// Writer is the transaction-bound port used by Payment.
type Writer struct{}

// Enqueue inserts an event through the transaction executor supplied by Store.
// It never publishes to RabbitMQ and therefore cannot create a dual write.
func (Writer) Enqueue(ctx context.Context, executor sqlc.DBTX, event notification.EventEnvelope) error {
	if executor == nil {
		return errors.New("outbox transaction executor is required")
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate outbox event: %w", err)
	}
	_, err := executor.ExecContext(ctx, `
		INSERT INTO outbox_events (
			id, event_type, event_version, aggregate_id, correlation_id, payload, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.EventID, event.EventType, event.EventVersion, event.AggregateID,
		event.CorrelationID, []byte(event.Payload), event.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

// Repository owns outbox polling and publication state transitions.
type Repository struct{ db *sql.DB }

// NewRepository creates an outbox repository using Banking's database connection.
func NewRepository(database *sql.DB) (*Repository, error) {
	if database == nil {
		return nil, errors.New("outbox database is required")
	}
	return &Repository{db: database}, nil
}

// Backlog reports committed events that have not yet received broker
// confirmation. It is intentionally a count only: payloads remain out of
// operational telemetry.
func (repository *Repository) Backlog(ctx context.Context) (int64, error) {
	var count int64
	if err := repository.db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE published_at IS NULL`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count outbox backlog: %w", err)
	}
	return count, nil
}

// ClaimBatch leases unpublished rows. A crashed publisher's lease expires,
// allowing another publisher to deliver the same event at least once.
func (repository *Repository) ClaimBatch(ctx context.Context, limit int, lease time.Duration) ([]notification.EventEnvelope, error) {
	if limit <= 0 || limit > 100 {
		limit = defaultBatch
	}
	if lease <= 0 {
		lease = defaultLease
	}
	rows, err := repository.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id FROM outbox_events
			WHERE published_at IS NULL AND (lease_until IS NULL OR lease_until < CURRENT_TIMESTAMP)
			ORDER BY created_at, id
			LIMIT $1 FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events AS event
		SET lease_until = CURRENT_TIMESTAMP + $2::INTERVAL,
			publish_attempts = event.publish_attempts + 1,
			last_error = NULL
		FROM candidates
		WHERE event.id = candidates.id
		RETURNING event.id, event.event_type, event.event_version, event.aggregate_id,
			event.correlation_id, event.payload, event.occurred_at`, limit, lease.String())
	if err != nil {
		return nil, fmt.Errorf("claim outbox events: %w", err)
	}
	defer rows.Close()

	events := make([]notification.EventEnvelope, 0, limit)
	for rows.Next() {
		var event notification.EventEnvelope
		if err := rows.Scan(&event.EventID, &event.EventType, &event.EventVersion, &event.AggregateID,
			&event.CorrelationID, &event.Payload, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("stored outbox event %s is invalid: %w", event.EventID, err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox events: %w", err)
	}
	return events, nil
}

// MarkPublished makes a confirmed RabbitMQ publish ineligible for polling.
func (repository *Repository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE outbox_events
		SET published_at = CURRENT_TIMESTAMP, lease_until = NULL, last_error = NULL
		WHERE id = $1 AND published_at IS NULL`, eventID)
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return fmt.Errorf("read published outbox rows: %w", err)
		}
		return errors.New("outbox event was not publishable")
	}
	return nil
}

// Release records a bounded diagnostic and makes a failed publish retryable.
func (repository *Repository) Release(ctx context.Context, eventID uuid.UUID, retryAfter time.Duration, reason string) error {
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	if len(reason) > 300 {
		reason = reason[:300]
	}
	_, err := repository.db.ExecContext(ctx, `
		UPDATE outbox_events
		SET lease_until = CURRENT_TIMESTAMP + $2::INTERVAL, last_error = $3
		WHERE id = $1 AND published_at IS NULL`, eventID, retryAfter.String(), reason)
	if err != nil {
		return fmt.Errorf("release outbox event: %w", err)
	}
	return nil
}

// Publisher is the narrow RabbitMQ-facing port used by Runner.
type Publisher interface {
	Publish(context.Context, notification.EventEnvelope) error
	Enabled() bool
}

// Runner continuously publishes committed events. Failure leaves an outbox row
// retryable; a restart or broker recovery resumes delivery without data loss.
type Runner struct {
	repository *Repository
	publisher  Publisher
}

// NewRunner constructs an outbox publisher loop.
func NewRunner(repository *Repository, publisher Publisher) (*Runner, error) {
	if repository == nil || publisher == nil {
		return nil, errors.New("outbox repository and publisher are required")
	}
	return &Runner{repository: repository, publisher: publisher}, nil
}

// Run polls until context cancellation. It deliberately never marks a row
// published before Publish reports broker confirmation.
func (runner *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(publishInterval)
	defer ticker.Stop()
	for {
		if err := runner.Flush(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Warn().Err(err).Msg("Outbox publish cycle failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Flush publishes one bounded batch and is exposed for deterministic tests.
func (runner *Runner) Flush(ctx context.Context) error {
	if !runner.publisher.Enabled() {
		return errors.New("RabbitMQ publisher is disabled")
	}
	events, err := runner.repository.ClaimBatch(ctx, defaultBatch, defaultLease)
	if err != nil || len(events) == 0 {
		return err
	}
	for _, event := range events {
		if err = runner.publisher.Publish(ctx, event); err != nil {
			releaseErr := runner.repository.Release(ctx, event.EventID, time.Second, "broker publish failed")
			if releaseErr != nil {
				return releaseErr
			}
			return fmt.Errorf("publish outbox event %s: %w", event.EventID, err)
		}
		if err = runner.repository.MarkPublished(ctx, event.EventID); err != nil {
			return err
		}
	}
	return nil
}

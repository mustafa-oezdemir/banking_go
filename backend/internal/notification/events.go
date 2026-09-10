package notification

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// EventTypePaymentBooked is emitted only after a payment and its ledger postings commit.
	EventTypePaymentBooked = "payment.booked.v1"
	// EventTypePaymentFailed is emitted only after a terminal payment failure commits.
	EventTypePaymentFailed = "payment.failed.v1"
	EventVersion           = 1
)

// ErrInvalidEvent classifies malformed cross-service event envelopes.
var ErrInvalidEvent = errors.New("invalid notification event")

// EventEnvelope is the stable Banking-to-Notification event contract. Payload
// is a fully resolved ActivityCommand so Notification never reads Banking data.
type EventEnvelope struct {
	EventID       uuid.UUID       `json:"event_id"`
	EventType     string          `json:"event_type"`
	EventVersion  int             `json:"event_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	AggregateID   uuid.UUID       `json:"aggregate_id"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}

// NewPaymentEvent creates a versioned event with a validated activity payload.
func NewPaymentEvent(eventType string, aggregateID uuid.UUID, correlationID string, command ActivityCommand) (EventEnvelope, error) {
	if err := command.Validate(); err != nil {
		return EventEnvelope{}, err
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return EventEnvelope{}, err
	}
	event := EventEnvelope{
		EventID: uuid.New(), EventType: eventType, EventVersion: EventVersion,
		OccurredAt: time.Now().UTC(), AggregateID: aggregateID,
		CorrelationID: strings.TrimSpace(correlationID), Payload: payload,
	}
	if err := event.Validate(); err != nil {
		return EventEnvelope{}, err
	}
	return event, nil
}

// Validate checks envelope metadata and the versioned payment payload schema.
func (event EventEnvelope) Validate() error {
	if event.EventID == uuid.Nil || event.AggregateID == uuid.Nil ||
		(event.EventType != EventTypePaymentBooked && event.EventType != EventTypePaymentFailed) ||
		event.EventVersion != EventVersion || event.OccurredAt.IsZero() ||
		strings.TrimSpace(event.CorrelationID) == "" || utf8.RuneCountInString(event.CorrelationID) > 128 ||
		len(event.Payload) == 0 || len(event.Payload) > 64<<10 {
		return ErrInvalidEvent
	}
	var command ActivityCommand
	decoder := json.NewDecoder(strings.NewReader(string(event.Payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&command); err != nil || command.Validate() != nil {
		return ErrInvalidEvent
	}
	return nil
}

// ActivityCommand returns the typed event payload after schema validation.
func (event EventEnvelope) ActivityCommand() (ActivityCommand, error) {
	if err := event.Validate(); err != nil {
		return ActivityCommand{}, err
	}
	var command ActivityCommand
	if err := json.Unmarshal(event.Payload, &command); err != nil {
		return ActivityCommand{}, ErrInvalidEvent
	}
	return command, nil
}

// Package domain contains infrastructure-free payment lifecycle and intent rules.
package domain

import "errors"

// Status is a payment lifecycle state.
type Status string

const (
	// StatusDraft is a locally prepared payment that is not ready for confirmation.
	StatusDraft Status = "DRAFT"
	// StatusAwaitingConfirmation is a validated payment awaiting customer confirmation.
	StatusAwaitingConfirmation Status = "AWAITING_CONFIRMATION"
	// StatusScheduled is ready to be claimed at its requested execution time.
	StatusScheduled Status = "SCHEDULED"
	// StatusProcessing has been claimed for an atomic booking attempt.
	StatusProcessing Status = "PROCESSING"
	// StatusBooked is a terminal payment with a committed ledger transaction.
	StatusBooked Status = "BOOKED"
	// StatusFailed is a terminal rejected booking attempt.
	StatusFailed Status = "FAILED"
	// StatusCancelled is a terminal customer cancellation.
	StatusCancelled Status = "CANCELLED"
)

// ErrInvalidTransition rejects a lifecycle transition not declared by the domain.
var ErrInvalidTransition = errors.New("payment cannot transition from its current state")

// ValidateTransition enforces the payment state machine independently of SQL WHERE clauses.
func ValidateTransition(from, to Status) error {
	if allowedTransitions[from][to] {
		return nil
	}
	return ErrInvalidTransition
}

var allowedTransitions = map[Status]map[Status]bool{
	StatusDraft: {
		StatusCancelled: true,
	},
	StatusAwaitingConfirmation: {
		StatusScheduled:  true,
		StatusProcessing: true,
		StatusCancelled:  true,
	},
	StatusScheduled: {
		StatusProcessing: true,
		StatusCancelled:  true,
	},
	StatusProcessing: {
		StatusScheduled: true, // stale worker claim recovery
		StatusBooked:    true,
		StatusFailed:    true,
	},
}

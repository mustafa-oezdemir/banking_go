package rabbitmq

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/notificationstore"
)

type memoryEventStore struct {
	mu        sync.Mutex
	processed map[uuid.UUID]bool
	claimed   map[uuid.UUID]bool
	released  int
}

func newMemoryEventStore() *memoryEventStore {
	return &memoryEventStore{processed: map[uuid.UUID]bool{}, claimed: map[uuid.UUID]bool{}}
}

func (store *memoryEventStore) Claim(_ context.Context, eventID uuid.UUID) (notificationstore.ClaimState, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.processed[eventID] {
		return notificationstore.AlreadyProcessed, nil
	}
	if store.claimed[eventID] {
		return notificationstore.Busy, nil
	}
	store.claimed[eventID] = true
	return notificationstore.Claimed, nil
}

func (store *memoryEventStore) MarkProcessed(_ context.Context, eventID uuid.UUID) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.processed[eventID] = true
	delete(store.claimed, eventID)
	return nil
}

func (store *memoryEventStore) Release(_ context.Context, eventID uuid.UUID, _ string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.released++
	delete(store.claimed, eventID)
	return nil
}

type deliveryStub struct {
	calls int
	err   error
}

func (delivery *deliveryStub) DeliverActivity(_ context.Context, _ notification.ActivityCommand) error {
	delivery.calls++
	return delivery.err
}

func paymentEvent(t *testing.T) notification.EventEnvelope {
	t.Helper()
	event, err := notification.NewPaymentEvent(notification.EventTypePaymentBooked, uuid.New(), "request-1", notification.ActivityCommand{
		RecipientEmail: "owner@example.test", RecipientName: "Ada", AccountName: "Girokonto",
		MaskedIBAN: "DE89 ••••", Balance: "10.00", Kind: "SEPA_PAYMENT_SENT", Direction: "DEBIT",
		Amount: "2.00", Currency: "EUR",
	})
	require.NoError(t, err)
	return event
}

func TestProcessorIgnoresDuplicateDelivery(t *testing.T) {
	store := newMemoryEventStore()
	delivery := &deliveryStub{}
	processor, err := NewProcessor(store, delivery, time.Second)
	require.NoError(t, err)
	event := paymentEvent(t)

	assert.Equal(t, Processed, processor.Process(context.Background(), event))
	assert.Equal(t, Duplicate, processor.Process(context.Background(), event))
	assert.Equal(t, 1, delivery.calls, "duplicate broker delivery must not send a second email")
}

func TestProcessorReleasesTransientDeliveryFailureForRetry(t *testing.T) {
	store := newMemoryEventStore()
	delivery := &deliveryStub{err: errors.New("smtp unavailable")}
	processor, err := NewProcessor(store, delivery, time.Second)
	require.NoError(t, err)

	assert.Equal(t, Retry, processor.Process(context.Background(), paymentEvent(t)))
	assert.Equal(t, 1, store.released)
}

func TestProcessorDeadLettersMalformedEventWithoutCallingProvider(t *testing.T) {
	store := newMemoryEventStore()
	delivery := &deliveryStub{}
	processor, err := NewProcessor(store, delivery, time.Second)
	require.NoError(t, err)

	malformed := paymentEvent(t)
	malformed.Payload = []byte(`{"recipient_email":"owner@example.test","unexpected":true}`)
	assert.Equal(t, Dead, processor.Process(context.Background(), malformed))
	assert.Equal(t, 0, delivery.calls, "a malformed command must never reach the email provider")
}

func TestRetryHeadersAreBounded(t *testing.T) {
	headers := retryHeaders(amqp.Table{"x-retry-count": int32(2)})
	assert.Equal(t, int32(3), headers["x-retry-count"])
	assert.Equal(t, 0, retryCount(nil))
}

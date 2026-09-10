package notification

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentEventContractRoundTrip(t *testing.T) {
	command := ActivityCommand{
		RecipientEmail: "owner@example.test", RecipientName: "Ada Example", AccountName: "Girokonto",
		MaskedIBAN: "DE89 •••• •••• •••• ••00", Balance: "87.6600", Kind: "SEPA_PAYMENT_SENT",
		Direction: "DEBIT", Amount: "12.34", Currency: "EUR", Reference: "Miete",
	}
	event, err := NewPaymentEvent(EventTypePaymentBooked, uuid.New(), "request-123", command)
	require.NoError(t, err)
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	var decoded EventEnvelope
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.NoError(t, decoded.Validate())
	actual, err := decoded.ActivityCommand()
	require.NoError(t, err)
	assert.Equal(t, command, actual)
}

func TestPaymentEventContractRejectsUnknownPayloadFields(t *testing.T) {
	event := EventEnvelope{
		EventID: uuid.New(), EventType: EventTypePaymentBooked, EventVersion: EventVersion,
		OccurredAt: time.Now().UTC(), AggregateID: uuid.New(), CorrelationID: "request-123",
		Payload: []byte(`{"recipient_email":"owner@example.test","recipient_name":"Ada","account_name":"Girokonto","masked_iban":"DE89 ••••","balance":"1.00","kind":"BOOKED","direction":"DEBIT","amount":"1.00","currency":"EUR","unexpected":true}`),
	}
	assert.ErrorIs(t, event.Validate(), ErrInvalidEvent)
}

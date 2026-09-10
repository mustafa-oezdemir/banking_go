package payment

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

type recordingOutboxWriter struct {
	events []notification.EventEnvelope
}

func (writer *recordingOutboxWriter) Enqueue(_ context.Context, _ sqlc.DBTX, event notification.EventEnvelope) error {
	writer.events = append(writer.events, event)
	return nil
}

func TestScheduledExternalPaymentIsIdempotentAndBalanced(t *testing.T) {
	ledger := setupTestLedger(t)
	ctx := context.Background()
	owner, err := ledger.store.CreateUser(ctx, sqlc.CreateUserParams{
		Email:          fmt.Sprintf("payment-%s@demo.invalid", uuid.NewString()),
		HashedPassword: "not-used-in-this-test",
		FullName:       "Integration Demo",
	})
	require.NoError(t, err)
	source, err := ledger.store.CreateAccount(ctx, sqlc.CreateAccountParams{
		OwnerID: uuid.NullUUID{UUID: owner.ID, Valid: true}, Name: "Payment Test",
		Currency: "EUR", IsSystem: false, Iban: mustDemoIBAN(t),
		AccountType: "GIROKONTO", Status: "ACTIVE",
	})
	require.NoError(t, err)
	require.NoError(t, ledger.Deposit(ctx, source.ID, "50.00"))

	destinationIBAN := mustDemoIBAN(t)
	service := NewService(ledger.store, nil)
	recordingOutbox := &recordingOutboxWriter{}
	service.SetOutboxWriter(recordingOutbox)
	input := CreatePaymentInput{
		OwnerID: owner.ID, SourceAccountID: source.ID, BeneficiaryName: "External Demo",
		BeneficiaryIBAN: destinationIBAN, Amount: "12.34", TransferType: PaymentStandard,
		ScheduleType: ScheduleScheduled, Purpose: "Integration test",
		RequestedExecution: time.Now().UTC().Add(1100 * time.Millisecond),
		IdempotencyKey:     "integration-" + uuid.NewString(),
	}
	created, err := service.CreatePayment(ctx, input)
	require.NoError(t, err)
	replayed, err := service.CreatePayment(ctx, input)
	require.NoError(t, err)
	assert.True(t, replayed.Replayed)
	assert.Equal(t, created.Order.ID, replayed.Order.ID)

	order, err := service.ConfirmPayment(ctx, owner.ID, created.Order.ID, true)
	require.NoError(t, err)
	require.Equal(t, PaymentScheduled, order.Status)
	time.Sleep(1200 * time.Millisecond)

	var workers sync.WaitGroup
	workerErrors := make(chan error, 2)
	workers.Add(2)
	for range 2 {
		go func() {
			defer workers.Done()
			_, runErr := service.RunDuePayments(ctx, 25)
			workerErrors <- runErr
		}()
	}
	workers.Wait()
	close(workerErrors)
	for workerErr := range workerErrors {
		require.NoError(t, workerErr)
	}
	_, err = service.RunDuePayments(ctx, 25)
	require.NoError(t, err)

	booked, err := service.GetPayment(ctx, owner.ID, created.Order.ID)
	require.NoError(t, err)
	require.Equal(t, PaymentBooked, booked.Status)
	require.Len(t, recordingOutbox.events, 1)
	assert.Equal(t, notification.EventTypePaymentBooked, recordingOutbox.events[0].EventType)
	assert.Equal(t, created.Order.ID, recordingOutbox.events[0].AggregateID)
	assert.NoError(t, recordingOutbox.events[0].Validate())
	require.True(t, booked.LedgerTransactionID.Valid)
	entries, err := ledger.store.ListEntriesByTransaction(ctx, booked.LedgerTransactionID.UUID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	totalDebit := decimal.Zero
	totalCredit := decimal.Zero
	for _, entry := range entries {
		totalDebit = totalDebit.Add(decimal.RequireFromString(entry.Debit))
		totalCredit = totalCredit.Add(decimal.RequireFromString(entry.Credit))
	}
	assert.True(t, totalDebit.Equal(decimal.RequireFromString("12.34")))
	assert.True(t, totalCredit.Equal(totalDebit))
	assert.Equal(t, "37.6600", getAccountBalance(t, ledger, source.ID))

	cancellable := input
	cancellable.IdempotencyKey = "cancel-" + uuid.NewString()
	cancellable.RequestedExecution = time.Now().UTC().Add(time.Hour)
	cancellable.Amount = "1.00"
	cancelCreated, err := service.CreatePayment(ctx, cancellable)
	require.NoError(t, err)
	_, err = service.ConfirmPayment(ctx, owner.ID, cancelCreated.Order.ID, true)
	require.NoError(t, err)
	cancelled, err := service.CancelPayment(ctx, owner.ID, cancelCreated.Order.ID)
	require.NoError(t, err)
	assert.Equal(t, "CANCELLED", cancelled.Status)

	unauthorized := input
	unauthorized.OwnerID = uuid.New()
	unauthorized.IdempotencyKey = "unauthorized-" + uuid.NewString()
	_, err = service.CreatePayment(ctx, unauthorized)
	assert.ErrorIs(t, err, ErrPaymentUnauthorized)
}

func TestBookingWritesVersionedOutboxEventInThePaymentTransaction(t *testing.T) {
	ledger := setupTestLedger(t)
	ctx := context.Background()
	owner, err := ledger.store.CreateUser(ctx, sqlc.CreateUserParams{
		Email: fmt.Sprintf("outbox-%s@demo.invalid", uuid.NewString()), HashedPassword: "not-used", FullName: "Outbox Demo",
	})
	require.NoError(t, err)
	source, err := ledger.store.CreateAccount(ctx, sqlc.CreateAccountParams{
		OwnerID: uuid.NullUUID{UUID: owner.ID, Valid: true}, Name: "Outbox Current", Currency: "EUR",
		IsSystem: false, Iban: mustDemoIBAN(t), AccountType: "GIROKONTO", Status: "ACTIVE",
	})
	require.NoError(t, err)
	require.NoError(t, ledger.Deposit(ctx, source.ID, "20.00"))
	service := NewService(ledger.store, nil)
	created, err := service.CreatePayment(ctx, CreatePaymentInput{
		OwnerID: owner.ID, SourceAccountID: source.ID, BeneficiaryName: "External Recipient", BeneficiaryIBAN: mustDemoIBAN(t),
		Amount: "5.00", TransferType: PaymentStandard, ScheduleType: ScheduleImmediate, Purpose: "Outbox test",
		RequestedExecution: time.Now().UTC(), IdempotencyKey: "outbox-" + uuid.NewString(),
	})
	require.NoError(t, err)
	booked, err := service.ConfirmPayment(ctx, owner.ID, created.Order.ID, true)
	require.NoError(t, err)
	require.Equal(t, PaymentBooked, booked.Status)

	var event notification.EventEnvelope
	require.NoError(t, ledger.store.ExecTxWithHandle(ctx, func(_ *sqlc.Queries, executor sqlc.DBTX) error {
		return executor.QueryRowContext(ctx, `
			SELECT id, event_type, event_version, aggregate_id, correlation_id, payload, occurred_at
			FROM outbox_events WHERE aggregate_id = $1`, booked.ID,
		).Scan(&event.EventID, &event.EventType, &event.EventVersion, &event.AggregateID,
			&event.CorrelationID, &event.Payload, &event.OccurredAt)
	}))
	assert.Equal(t, notification.EventTypePaymentBooked, event.EventType)
	assert.NoError(t, event.Validate())
}

func TestFailedPaymentWritesFailureOutboxEvent(t *testing.T) {
	ledger := setupTestLedger(t)
	ctx := context.Background()
	owner, err := ledger.store.CreateUser(ctx, sqlc.CreateUserParams{
		Email: fmt.Sprintf("outbox-failed-%s@demo.invalid", uuid.NewString()), HashedPassword: "not-used", FullName: "Failure Demo",
	})
	require.NoError(t, err)
	source, err := ledger.store.CreateAccount(ctx, sqlc.CreateAccountParams{
		OwnerID: uuid.NullUUID{UUID: owner.ID, Valid: true}, Name: "Empty Current", Currency: "EUR",
		IsSystem: false, Iban: mustDemoIBAN(t), AccountType: "GIROKONTO", Status: "ACTIVE",
	})
	require.NoError(t, err)
	service := NewService(ledger.store, nil)
	created, err := service.CreatePayment(ctx, CreatePaymentInput{
		OwnerID: owner.ID, SourceAccountID: source.ID, BeneficiaryName: "External Recipient", BeneficiaryIBAN: mustDemoIBAN(t),
		Amount: "5.00", TransferType: PaymentStandard, ScheduleType: ScheduleImmediate, Purpose: "Failure outbox test",
		RequestedExecution: time.Now().UTC(), IdempotencyKey: "outbox-failed-" + uuid.NewString(),
	})
	require.NoError(t, err)
	failed, confirmErr := service.ConfirmPayment(ctx, owner.ID, created.Order.ID, true)
	require.ErrorIs(t, confirmErr, ErrInsufficientFunds)
	require.Equal(t, PaymentFailed, failed.Status)

	var eventType string
	require.NoError(t, ledger.store.ExecTxWithHandle(ctx, func(_ *sqlc.Queries, executor sqlc.DBTX) error {
		return executor.QueryRowContext(ctx, `SELECT event_type FROM outbox_events WHERE aggregate_id = $1`, failed.ID).Scan(&eventType)
	}))
	assert.Equal(t, notification.EventTypePaymentFailed, eventType)
}

func TestValidatePaymentInputMoneyRules(t *testing.T) {
	base := CreatePaymentInput{
		OwnerID: uuid.New(), SourceAccountID: uuid.New(), BeneficiaryName: "Anna Müller",
		BeneficiaryIBAN: "DE89370400440532013000", Amount: "12.34",
		TransferType: PaymentStandard, ScheduleType: ScheduleImmediate,
		IdempotencyKey: "test-key-123", RequestedExecution: time.Now(),
	}
	amount, err := validatePaymentInput(base, time.Now())
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.RequireFromString("12.34")))

	for _, invalid := range []string{"0", "-1", "1.001", "NaN", ""} {
		input := base
		input.Amount = invalid
		_, err = validatePaymentInput(input, time.Now())
		assert.Error(t, err, invalid)
	}
}

func TestSamePaymentIntentRejectsIdempotencyPayloadChange(t *testing.T) {
	execution := time.Date(2026, time.August, 12, 9, 30, 0, 0, time.UTC)
	input := CreatePaymentInput{
		SourceAccountID: uuid.New(), BeneficiaryName: "Anna Müller",
		BeneficiaryIBAN: "DE89370400440532013000", BeneficiaryBIC: "DEMODEFFXXX",
		ScheduleType: ScheduleScheduled, TransferType: PaymentStandard,
		Purpose: "Miete", CreditorReference: "RF18539007547034",
		RequestedExecution: execution,
	}
	order := sqlc.PaymentOrder{
		SourceAccountID: input.SourceAccountID, BeneficiaryName: input.BeneficiaryName,
		BeneficiaryIban: input.BeneficiaryIBAN, BeneficiaryBic: sql.NullString{String: input.BeneficiaryBIC, Valid: true},
		Amount: "10.00", ScheduleType: input.ScheduleType, PaymentKind: "SEPA",
		Purpose:              sql.NullString{String: input.Purpose, Valid: true},
		CreditorReference:    sql.NullString{String: input.CreditorReference, Valid: true},
		RequestedExecutionAt: execution,
	}
	assert.True(t, samePaymentIntent(order, input, decimal.RequireFromString("10")))

	postgresRounded := input
	postgresRounded.RequestedExecution = execution.Add(600 * time.Nanosecond)
	order.RequestedExecutionAt = postgresRounded.RequestedExecution.Round(time.Microsecond)
	assert.True(t, samePaymentIntent(order, postgresRounded, decimal.RequireFromString("10")))
	order.RequestedExecutionAt = execution

	changes := []func(*CreatePaymentInput){
		func(value *CreatePaymentInput) { value.BeneficiaryName = "Different Name" },
		func(value *CreatePaymentInput) { value.Purpose = "Geänderter Zweck" },
		func(value *CreatePaymentInput) { value.CreditorReference = "RF001" },
		func(value *CreatePaymentInput) { value.RequestedExecution = execution.Add(time.Hour) },
		func(value *CreatePaymentInput) { value.TransferType = PaymentInstant },
	}
	for _, change := range changes {
		changed := input
		change(&changed)
		assert.False(t, samePaymentIntent(order, changed, decimal.RequireFromString("10")))
	}
}

func TestStandingOrderRecurrence(t *testing.T) {
	start := time.Date(2026, time.January, 31, 9, 0, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, time.February, 28, 9, 0, 0, 0, time.UTC), nextStandingExecution(start, "MONTHLY"))
	assert.Equal(t, start.AddDate(0, 0, 7), nextStandingExecution(start, "WEEKLY"))

	maxOccurrences := int32(2)
	order := sqlc.StandingOrder{MaxOccurrences: sql.NullInt32{Int32: maxOccurrences, Valid: true}, OccurrencesCreated: 1}
	assert.True(t, shouldCompleteStanding(order, start.AddDate(0, 1, 0)))
}

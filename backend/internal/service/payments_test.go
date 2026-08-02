package service

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

func TestValidatePaymentInputMoneyRules(t *testing.T) {
	base := CreatePaymentInput{
		OwnerID: uuid.New(), SourceAccountID: uuid.New(), BeneficiaryName: "Anna Müller",
		BeneficiaryIBAN: "DE89370400440532013000", Amount: "12.34",
		TransferType: PaymentStandard, ScheduleType: ScheduleImmediate,
		IdempotencyKey: "test-key-123", RequestedExecution: time.Now(),
	}
	amount, err := validatePaymentInput(base)
	require.NoError(t, err)
	assert.True(t, amount.Equal(decimal.RequireFromString("12.34")))

	for _, invalid := range []string{"0", "-1", "1.001", "NaN", ""} {
		input := base
		input.Amount = invalid
		_, err = validatePaymentInput(input)
		assert.Error(t, err, invalid)
	}
}

func TestSamePaymentIntentRejectsIdempotencyPayloadChange(t *testing.T) {
	input := CreatePaymentInput{
		SourceAccountID: uuid.New(), BeneficiaryName: "Anna Müller",
		BeneficiaryIBAN: "DE89370400440532013000", ScheduleType: ScheduleImmediate,
	}
	order := sqlc.PaymentOrder{
		SourceAccountID: input.SourceAccountID, BeneficiaryName: input.BeneficiaryName,
		BeneficiaryIban: input.BeneficiaryIBAN, Amount: "10.00", ScheduleType: input.ScheduleType,
	}
	assert.True(t, samePaymentIntent(order, input, decimal.RequireFromString("10")))
	input.BeneficiaryName = "Different Name"
	assert.False(t, samePaymentIntent(order, input, decimal.RequireFromString("10")))
}

func TestStandingOrderRecurrence(t *testing.T) {
	start := time.Date(2026, time.January, 31, 9, 0, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, time.February, 28, 9, 0, 0, 0, time.UTC), nextStandingExecution(start, "MONTHLY"))
	assert.Equal(t, start.AddDate(0, 0, 7), nextStandingExecution(start, "WEEKLY"))

	maxOccurrences := int32(2)
	order := sqlc.StandingOrder{MaxOccurrences: sql.NullInt32{Int32: maxOccurrences, Valid: true}, OccurrencesCreated: 1}
	assert.True(t, shouldCompleteStanding(order, start.AddDate(0, 1, 0)))
}

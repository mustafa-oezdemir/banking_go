package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestEquivalentIntentProtectsIdempotency(t *testing.T) {
	base := Intent{
		SourceAccountID: uuid.New(), BeneficiaryName: "Anna Müller",
		BeneficiaryIBAN: "DE89370400440532013000", BeneficiaryBIC: "DEMODEFFXXX",
		Amount: decimal.RequireFromString("10.00"), ScheduleType: "SCHEDULED",
		Purpose: "Miete", CreditorReference: "RF18539007547034",
		RequestedExecution: time.Date(2026, time.August, 12, 9, 30, 0, 0, time.UTC),
	}
	stored := base
	stored.RequestedExecution = stored.RequestedExecution.Add(time.Microsecond)
	assert.True(t, EquivalentIntent(stored, base))

	changedAmount := base
	changedAmount.Amount = decimal.RequireFromString("10.01")
	assert.False(t, EquivalentIntent(stored, changedAmount))

	changedBeneficiary := base
	changedBeneficiary.BeneficiaryIBAN = "DE75512108001245126199"
	assert.False(t, EquivalentIntent(stored, changedBeneficiary))
}

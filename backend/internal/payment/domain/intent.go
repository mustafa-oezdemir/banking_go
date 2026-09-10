package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Intent is the normalized business identity protected by an idempotency key.
type Intent struct {
	RequestedExecution time.Time
	Amount             decimal.Decimal
	BeneficiaryName    string
	BeneficiaryIBAN    string
	BeneficiaryBIC     string
	ScheduleType       string
	Purpose            string
	CreditorReference  string
	SourceAccountID    uuid.UUID
	Instant            bool
	Internal           bool
}

// EquivalentIntent reports whether a replay represents the original business
// intent. PostgreSQL timestamp precision is explicitly accounted for.
func EquivalentIntent(stored, requested Intent) bool {
	sameExecution := true
	if requested.ScheduleType == "SCHEDULED" {
		executionDelta := stored.RequestedExecution.UTC().Sub(requested.RequestedExecution.UTC())
		if executionDelta < 0 {
			executionDelta = -executionDelta
		}
		sameExecution = executionDelta <= time.Microsecond
	}
	return stored.SourceAccountID == requested.SourceAccountID &&
		stored.BeneficiaryIBAN == requested.BeneficiaryIBAN &&
		stored.BeneficiaryName == requested.BeneficiaryName &&
		stored.BeneficiaryBIC == requested.BeneficiaryBIC &&
		stored.Amount.Equal(requested.Amount) &&
		stored.ScheduleType == requested.ScheduleType &&
		stored.Purpose == requested.Purpose &&
		stored.CreditorReference == requested.CreditorReference &&
		sameExecution &&
		(stored.Instant == requested.Instant || stored.Internal)
}

package ledger

import (
	"strings"

	"github.com/shopspring/decimal"
)

const maxEURAmountInputLength = 18

var maxEURAmount = decimal.RequireFromString("999999999999999.99")

// ParseEURAmount applies one exact monetary boundary across every write path.
// PostgreSQL NUMERIC(19,4) can hold four fractional digits, but customer-facing
// EUR operations intentionally accept cents only.
func ParseEURAmount(raw string) (decimal.Decimal, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxEURAmountInputLength {
		return decimal.Zero, ErrInvalidAmount
	}
	amount, err := decimal.NewFromString(value)
	if err != nil || !amount.IsPositive() || amount.Exponent() < -2 || amount.GreaterThan(maxEURAmount) {
		return decimal.Zero, ErrInvalidAmount
	}
	return amount, nil
}

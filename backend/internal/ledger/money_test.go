package ledger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEURAmount(t *testing.T) {
	for _, value := range []string{"0.01", "1", " 25.50 ", "999999999999999.99"} {
		t.Run("accepts_"+value, func(t *testing.T) {
			amount, err := ParseEURAmount(value)
			require.NoError(t, err)
			assert.True(t, amount.IsPositive())
		})
	}

	for _, value := range []string{
		"", "0", "-1", "1.001", "999999999999999.999", "1000000000000000.00", "not-a-number",
	} {
		t.Run("rejects_"+value, func(t *testing.T) {
			_, err := ParseEURAmount(value)
			require.ErrorIs(t, err, ErrInvalidAmount)
		})
	}
}

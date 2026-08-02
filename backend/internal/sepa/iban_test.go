package sepa

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateGermanDemoIBAN(t *testing.T) {
	seen := make(map[string]struct{})
	for range 100 {
		iban, err := GenerateGermanDemoIBAN()
		require.NoError(t, err)
		assert.Len(t, iban, GermanIBANLength)
		assert.True(t, strings.HasPrefix(iban, "DE"))
		assert.Equal(t, DemoBankCode, iban[4:12])
		require.NoError(t, ValidateIBAN(iban))
		_, duplicate := seen[iban]
		assert.False(t, duplicate)
		seen[iban] = struct{}{}
	}
}

func TestValidateIBAN(t *testing.T) {
	tests := []struct {
		name  string
		iban  string
		valid bool
	}{
		{name: "known German IBAN", iban: "DE89 3704 0044 0532 0130 00", valid: true},
		{name: "normalized lowercase", iban: "de89370400440532013000", valid: true},
		{name: "bad checksum", iban: "DE88 3704 0044 0532 0130 00", valid: false},
		{name: "bad German length", iban: "DE8937040044053201300", valid: false},
		{name: "illegal character", iban: "DE89-70400440532013000", valid: false},
		{name: "empty", iban: "", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIBAN(tt.iban)
			if tt.valid {
				require.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrInvalidIBAN)
			}
		})
	}
}

func TestMaskIBAN(t *testing.T) {
	assert.Equal(t, "DE89**************3000", MaskIBAN("DE89370400440532013000"))
}

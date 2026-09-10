package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validProfileRequest() updateProfileRequest {
	return updateProfileRequest{
		FullName: "Anna Beispiel", Phone: "+49 170 1234567", BirthDate: "1990-05-12",
		AddressLine1: "Musterstraße 12", AddressLine2: "Wohnung 4", PostalCode: "10115",
		City: "Berlin", CountryCode: "de",
	}
}

func TestValidateProfileInput(t *testing.T) {
	input := validProfileRequest()
	birthDate, err := validateProfileInput(&input, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, "DE", input.CountryCode)
	assert.Equal(t, "1990-05-12", birthDate.Format("2006-01-02"))
}

func TestValidateProfileInputRejectsInvalidData(t *testing.T) {
	tests := map[string]func(*updateProfileRequest){
		"missing name":  func(input *updateProfileRequest) { input.FullName = "" },
		"invalid phone": func(input *updateProfileRequest) { input.Phone = "call-me<script>" },
		"future birth":  func(input *updateProfileRequest) { input.BirthDate = "2030-01-01" },
		"short address": func(input *updateProfileRequest) { input.AddressLine1 = "x" },
		"country":       func(input *updateProfileRequest) { input.CountryCode = "DEU" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := validProfileRequest()
			mutate(&input)
			_, err := validateProfileInput(&input, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
			require.Error(t, err)
		})
	}
}

package account

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateName(t *testing.T) {
	name, err := ValidateName("  Urlaubskonto  ")
	require.NoError(t, err)
	assert.Equal(t, "Urlaubskonto", name)
	_, err = ValidateName(strings.Repeat("x", 101))
	assert.Error(t, err)
}

func TestNormalizeProfile(t *testing.T) {
	now := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	profile, birthDate, err := NormalizeProfile(ProfileInput{
		FullName: " Helga Müller ", Phone: " +49 30 1234567 ", BirthDate: "1980-05-17",
		AddressLine1: " Musterstraße 1 ", PostalCode: " 10115 ", City: " Berlin ", CountryCode: " de ",
	}, now)
	require.NoError(t, err)
	assert.Equal(t, "Helga Müller", profile.FullName)
	assert.Equal(t, "DE", profile.CountryCode)
	assert.Equal(t, "1980-05-17", birthDate.Format("2006-01-02"))
}

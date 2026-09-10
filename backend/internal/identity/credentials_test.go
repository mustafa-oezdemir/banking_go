package identity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeEmail(t *testing.T) {
	email, err := NormalizeEmail("  USER@Example.com ")
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", email)

	email, err = NormalizeEmail("Helga.Müller@Pehlione.com")
	require.NoError(t, err)
	assert.Equal(t, "helga.müller@pehlione.com", email)

	for _, invalid := range []string{"", "not-an-email", "Name <user@example.com>"} {
		_, err = NormalizeEmail(invalid)
		assert.Error(t, err)
	}
}

func TestValidateRegistrationPassword(t *testing.T) {
	assert.Error(t, ValidatePassword("short"))
	assert.Error(t, ValidatePassword("password123456"))
	assert.Error(t, ValidatePassword(strings.Repeat("ü", maxPasswordBytes)))
	assert.NoError(t, ValidatePassword("correct horse battery staple"))
}

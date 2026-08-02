package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeEmail(t *testing.T) {
	email, err := normalizeEmail("  USER@Example.com ")
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", email)

	for _, invalid := range []string{"", "not-an-email", "Name <user@example.com>"} {
		_, err = normalizeEmail(invalid)
		assert.Error(t, err)
	}
}

func TestValidateRegistrationPassword(t *testing.T) {
	assert.Error(t, validateRegistrationPassword("short"))
	assert.Error(t, validateRegistrationPassword("password123456"))
	assert.Error(t, validateRegistrationPassword(strings.Repeat("ü", maxPasswordBytes)))
	assert.NoError(t, validateRegistrationPassword("correct horse battery staple"))
}

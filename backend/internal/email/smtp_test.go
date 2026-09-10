package email

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSMTPProviderTakesPrecedenceOverResend(t *testing.T) {
	t.Parallel()
	service := NewService(nil, Config{
		APIKey: "re_test", From: "Pehlione <banking@example.invalid>",
		FrontendURL: "http://localhost:3000", SMTPHost: "mailhog", SMTPPort: "1025",
	}, nil)

	assert.True(t, service.Enabled())
	assert.Equal(t, "smtp", service.Provider())
}

func TestBuildSMTPMessageCreatesSafeMultipartContent(t *testing.T) {
	t.Parallel()
	message := string(buildSMTPMessage(
		"Pehlione <banking@example.invalid>", "kunde@example.invalid",
		"Buchung\r\nBcc: attacker@example.invalid", "<p>Hallo</p>", "Hallo",
	))

	assert.Contains(t, message, "Content-Type: multipart/alternative")
	assert.Contains(t, message, "Content-Type: text/plain; charset=UTF-8")
	assert.Contains(t, message, "Content-Type: text/html; charset=UTF-8")
	assert.Contains(t, message, "<p>Hallo</p>")
	assert.NotContains(t, message, "\r\nBcc: attacker@example.invalid")
	assert.Equal(t, 1, strings.Count(message, "Bcc:"))
}

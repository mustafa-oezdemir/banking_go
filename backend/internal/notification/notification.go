// Package notification defines account-activity messages independently of
// their delivery provider.
package notification

import (
	"context"

	"github.com/google/uuid"
)

// Activity describes a successfully committed customer account movement.
type Activity struct {
	Kind         string
	Direction    string
	Amount       string
	Currency     string
	Counterparty string
	Reference    string
	UserID       uuid.UUID
	AccountID    uuid.UUID
}

// Sender delivers security and account-activity messages.
type Sender interface {
	SendPasswordReset(ctx context.Context, email, fullName, token string) error
	NotifyActivity(activity Activity)
	Enabled() bool
}

// NoopSender safely disables email in tests and local environments without a key.
type NoopSender struct{}

// SendPasswordReset implements Sender without external delivery.
func (NoopSender) SendPasswordReset(context.Context, string, string, string) error { return nil }

// NotifyActivity implements Sender without external delivery.
func (NoopSender) NotifyActivity(Activity) {}

// Enabled reports that delivery is disabled.
func (NoopSender) Enabled() bool { return false }

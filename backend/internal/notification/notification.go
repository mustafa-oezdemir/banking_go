// Package notification defines account-activity messages independently of
// their delivery provider.
package notification

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ErrInvalidCommand classifies malformed notification commands at the service boundary.
var ErrInvalidCommand = errors.New("invalid notification command")

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

// PasswordResetCommand is the versioned HTTP contract consumed by the notification service.
type PasswordResetCommand struct {
	RecipientEmail string `json:"recipient_email"`
	RecipientName  string `json:"recipient_name"`
	ResetToken     string `json:"reset_token"`
}

// ActivityCommand contains all recipient and account data needed to render an
// activity email. The notification service never reads Banking Core tables.
type ActivityCommand struct {
	RecipientEmail string `json:"recipient_email"`
	RecipientName  string `json:"recipient_name"`
	AccountName    string `json:"account_name"`
	MaskedIBAN     string `json:"masked_iban"`
	Balance        string `json:"balance"`
	Kind           string `json:"kind"`
	Direction      string `json:"direction"`
	Amount         string `json:"amount"`
	Currency       string `json:"currency"`
	Counterparty   string `json:"counterparty,omitempty"`
	Reference      string `json:"reference,omitempty"`
}

// Validate rejects malformed or oversized password-reset commands.
func (command PasswordResetCommand) Validate() error {
	if !validEmail(command.RecipientEmail) || utf8.RuneCountInString(command.RecipientName) > 140 ||
		strings.TrimSpace(command.ResetToken) == "" || len(command.ResetToken) > 512 {
		return ErrInvalidCommand
	}
	return nil
}

// Validate rejects malformed or oversized account-activity commands.
func (command ActivityCommand) Validate() error {
	amount, amountErr := decimal.NewFromString(command.Amount)
	_, balanceErr := decimal.NewFromString(command.Balance)
	direction := strings.ToUpper(strings.TrimSpace(command.Direction))
	if !validEmail(command.RecipientEmail) || utf8.RuneCountInString(command.RecipientName) > 140 ||
		strings.TrimSpace(command.AccountName) == "" || utf8.RuneCountInString(command.AccountName) > 100 ||
		strings.TrimSpace(command.MaskedIBAN) == "" || utf8.RuneCountInString(command.MaskedIBAN) > 40 ||
		strings.TrimSpace(command.Kind) == "" || utf8.RuneCountInString(command.Kind) > 64 ||
		(direction != "DEBIT" && direction != "CREDIT") ||
		strings.ToUpper(strings.TrimSpace(command.Currency)) != "EUR" ||
		amountErr != nil || !amount.IsPositive() || balanceErr != nil ||
		utf8.RuneCountInString(command.Counterparty) > 140 || utf8.RuneCountInString(command.Reference) > 280 {
		return ErrInvalidCommand
	}
	return nil
}

func validEmail(value string) bool {
	recipient := strings.TrimSpace(value)
	if recipient == "" || len(recipient) > 254 {
		return false
	}
	parsed, err := mail.ParseAddress(recipient)
	return err == nil && parsed.Address == recipient
}

// Delivery sends fully resolved commands through a provider such as SMTP.
type Delivery interface {
	DeliverPasswordReset(context.Context, PasswordResetCommand) error
	DeliverActivity(context.Context, ActivityCommand) error
	Enabled() bool
}

// Sender delivers security and account-activity messages.
type Sender interface {
	SendPasswordReset(ctx context.Context, email, fullName, token string) error
	NotifyActivity(context.Context, Activity) error
	Enabled() bool
}

// NoopSender safely disables email in tests and local environments without a key.
type NoopSender struct{}

// SendPasswordReset implements Sender without external delivery.
func (NoopSender) SendPasswordReset(context.Context, string, string, string) error { return nil }

// NotifyActivity implements Sender without external delivery.
func (NoopSender) NotifyActivity(context.Context, Activity) error { return nil }

// Enabled reports that delivery is disabled.
func (NoopSender) Enabled() bool { return false }

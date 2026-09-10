package api

import "time"

// AccountResponse represents an account returned by the API.
//
//nolint:govet // This layout keeps the JSON response fields grouped for readability.
type AccountResponse struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Balance          string    `json:"balance"`
	AvailableBalance string    `json:"available_balance"`
	Currency         string    `json:"currency"`
	IBAN             string    `json:"iban,omitempty"`
	MaskedIBAN       string    `json:"masked_iban"`
	AccountType      string    `json:"account_type"`
	Status           string    `json:"status"`
	OwnerID          *string   `json:"owner_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	IsSystem         bool      `json:"is_system"`
}

// EntryResponse represents a ledger entry returned by the API.
//
//nolint:govet // This layout follows the transaction JSON schema.
type EntryResponse struct {
	CreatedAt        time.Time  `json:"created_at"`
	ID               string     `json:"id"`
	AccountID        string     `json:"account_id"`
	Debit            string     `json:"debit"`
	Credit           string     `json:"credit"`
	TransactionID    string     `json:"transaction_id"`
	OperationType    string     `json:"operation_type"`
	Description      string     `json:"description,omitempty"`
	PaymentOrderID   *string    `json:"payment_order_id,omitempty"`
	CounterpartyName string     `json:"counterparty_name,omitempty"`
	CounterpartyIBAN string     `json:"counterparty_iban,omitempty"`
	Purpose          string     `json:"purpose,omitempty"`
	Category         string     `json:"category,omitempty"`
	BookingDate      *time.Time `json:"booking_date,omitempty"`
	ExecutionDate    *time.Time `json:"execution_date,omitempty"`
}

// RegisterResponse is returned after successful registration.
type RegisterResponse struct {
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	AccountID  string `json:"account_id"`
	MaskedIBAN string `json:"masked_iban"`
}

// SessionResponse describes the authenticated browser session.
type SessionResponse struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

// MessageResponse contains a simple status message.
type MessageResponse struct {
	Message string `json:"message"`
}

// ErrorResponse contains an API error message.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ReconcileResponse reports whether stored and computed balances match.
type ReconcileResponse struct {
	Message string `json:"message"`
	Matched bool   `json:"matched"`
}

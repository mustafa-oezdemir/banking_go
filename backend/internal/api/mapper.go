package api

import (
	"strings"

	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

func toAccountResponse(acc sqlc.Account) AccountResponse {
	var ownerID *string
	if acc.OwnerID.Valid {
		// Convert nullable UUID into pointer so omitempty works in JSON output.
		s := acc.OwnerID.UUID.String()
		ownerID = &s
	}

	return AccountResponse{
		ID:               acc.ID.String(),
		OwnerID:          ownerID,
		Name:             acc.Name,
		Balance:          acc.Balance,
		AvailableBalance: acc.AvailableBalance,
		Currency:         acc.Currency,
		IBAN:             acc.Iban,
		MaskedIBAN:       sepa.MaskIBAN(acc.Iban),
		AccountType:      acc.AccountType,
		Status:           acc.Status,
		IsSystem:         acc.IsSystem,
		CreatedAt:        acc.CreatedAt.Time,
		UpdatedAt:        acc.UpdatedAt,
	}
}

func toAccountListResponse(acc sqlc.Account) AccountResponse {
	response := toAccountResponse(acc)
	response.IBAN = ""
	return response
}

func toEntryResponse(entry sqlc.Entry) EntryResponse {
	var description string
	if entry.Description.Valid {
		// Preserve optional descriptions only when present in DB rows.
		description = entry.Description.String
	}

	operationType := operationTypeToString(entry.OperationType)

	response := EntryResponse{
		ID:               entry.ID.String(),
		AccountID:        entry.AccountID.String(),
		Debit:            entry.Debit,
		Credit:           entry.Credit,
		TransactionID:    entry.TransactionID.String(),
		OperationType:    operationType,
		Description:      description,
		CreatedAt:        entry.CreatedAt.Time,
		CounterpartyName: entry.CounterpartyName.String,
		CounterpartyIBAN: entry.CounterpartyIban.String,
		Purpose:          entry.Purpose.String,
		Category:         entry.Category.String,
	}
	if entry.PaymentOrderID.Valid {
		id := entry.PaymentOrderID.UUID.String()
		response.PaymentOrderID = &id
	}
	if entry.BookingDate.Valid {
		value := entry.BookingDate.Time
		response.BookingDate = &value
	}
	if entry.ExecutionDate.Valid {
		value := entry.ExecutionDate.Time
		response.ExecutionDate = &value
	}
	return response
}

func defaultFullName(fullName, email string) string {
	if value := strings.TrimSpace(fullName); value != "" {
		return value
	}
	local := strings.SplitN(email, "@", 2)[0]
	local = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(local)
	return strings.Title(local) //nolint:staticcheck // Stable fallback for existing demo registrations.
}

func operationTypeToString(v interface{}) string {
	// sqlc enum decoding can arrive as string or []byte depending on driver path.
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}

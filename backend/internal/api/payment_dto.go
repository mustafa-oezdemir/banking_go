package api

import (
	"time"

	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

type PaymentResponse struct {
	ID                    string     `json:"id"`
	SourceAccountID       string     `json:"source_account_id"`
	BeneficiaryName       string     `json:"beneficiary_name"`
	BeneficiaryIBAN       string     `json:"beneficiary_iban,omitempty"`
	MaskedBeneficiaryIBAN string     `json:"masked_beneficiary_iban"`
	BeneficiaryBIC        string     `json:"beneficiary_bic,omitempty"`
	Amount                string     `json:"amount"`
	Currency              string     `json:"currency"`
	PaymentKind           string     `json:"payment_kind"`
	ScheduleType          string     `json:"schedule_type"`
	Purpose               string     `json:"purpose,omitempty"`
	CreditorReference     string     `json:"creditor_reference,omitempty"`
	EndToEndID            string     `json:"end_to_end_id"`
	RequestedExecutionAt  time.Time  `json:"requested_execution_at"`
	VoPResult             string     `json:"vop_result"`
	VoPSuggestedName      string     `json:"vop_suggested_name,omitempty"`
	VoPOverridden         bool       `json:"vop_overridden"`
	Status                string     `json:"status"`
	RejectCode            string     `json:"reject_code,omitempty"`
	FailureReason         string     `json:"failure_reason,omitempty"`
	LedgerTransactionID   *string    `json:"ledger_transaction_id,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	ProcessedAt           *time.Time `json:"processed_at,omitempty"`
	Demo                  bool       `json:"demo"`
}

func toPaymentResponse(order sqlc.PaymentOrder, detail bool) PaymentResponse {
	response := PaymentResponse{
		ID: order.ID.String(), SourceAccountID: order.SourceAccountID.String(),
		BeneficiaryName: order.BeneficiaryName, MaskedBeneficiaryIBAN: sepa.MaskIBAN(order.BeneficiaryIban),
		BeneficiaryBIC: order.BeneficiaryBic.String, Amount: order.Amount, Currency: order.Currency,
		PaymentKind: order.PaymentKind, ScheduleType: order.ScheduleType, Purpose: order.Purpose.String,
		CreditorReference: order.CreditorReference.String, EndToEndID: order.EndToEndID,
		RequestedExecutionAt: order.RequestedExecutionAt, VoPResult: order.VopResult,
		VoPSuggestedName: order.VopSuggestedName.String, VoPOverridden: order.VopOverridden,
		Status: order.Status, RejectCode: order.RejectCode.String, FailureReason: order.FailureReason.String,
		CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt, Demo: true,
	}
	if detail {
		response.BeneficiaryIBAN = order.BeneficiaryIban
	}
	if order.LedgerTransactionID.Valid {
		value := order.LedgerTransactionID.UUID.String()
		response.LedgerTransactionID = &value
	}
	if order.ProcessedAt.Valid {
		value := order.ProcessedAt.Time
		response.ProcessedAt = &value
	}
	return response
}

type StandingOrderRequest struct {
	SourceAccountID   string `json:"source_account_id"`
	BeneficiaryName   string `json:"beneficiary_name"`
	BeneficiaryIBAN   string `json:"beneficiary_iban"`
	BeneficiaryBIC    string `json:"beneficiary_bic"`
	Amount            string `json:"amount"`
	Purpose           string `json:"purpose"`
	CreditorReference string `json:"creditor_reference"`
	TransferType      string `json:"transfer_type"`
	Frequency         string `json:"frequency"`
	StartDate         string `json:"start_date"`
	EndDate           string `json:"end_date"`
	MaxOccurrences    *int32 `json:"max_occurrences"`
}

type StandingOrderResponse struct {
	ID                    string     `json:"id"`
	SourceAccountID       string     `json:"source_account_id"`
	BeneficiaryName       string     `json:"beneficiary_name"`
	MaskedBeneficiaryIBAN string     `json:"masked_beneficiary_iban"`
	Amount                string     `json:"amount"`
	Currency              string     `json:"currency"`
	Purpose               string     `json:"purpose,omitempty"`
	TransferType          string     `json:"transfer_type"`
	Frequency             string     `json:"frequency"`
	StartDate             time.Time  `json:"start_date"`
	EndDate               *time.Time `json:"end_date,omitempty"`
	MaxOccurrences        *int32     `json:"max_occurrences,omitempty"`
	OccurrencesCreated    int32      `json:"occurrences_created"`
	NextExecutionAt       time.Time  `json:"next_execution_at"`
	Status                string     `json:"status"`
}

func toStandingOrderResponse(order sqlc.StandingOrder) StandingOrderResponse {
	response := StandingOrderResponse{
		ID: order.ID.String(), SourceAccountID: order.SourceAccountID.String(),
		BeneficiaryName: order.BeneficiaryName, MaskedBeneficiaryIBAN: sepa.MaskIBAN(order.BeneficiaryIban),
		Amount: order.Amount, Currency: order.Currency, Purpose: order.Purpose.String,
		TransferType: order.TransferType, Frequency: order.Frequency, StartDate: order.StartDate,
		OccurrencesCreated: order.OccurrencesCreated, NextExecutionAt: order.NextExecutionAt, Status: order.Status,
	}
	if order.EndDate.Valid {
		value := order.EndDate.Time
		response.EndDate = &value
	}
	if order.MaxOccurrences.Valid {
		value := order.MaxOccurrences.Int32
		response.MaxOccurrences = &value
	}
	return response
}

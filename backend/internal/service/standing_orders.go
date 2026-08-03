package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// CreateStandingOrderInput contains validated recurring-payment intent fields.
//
//nolint:govet // Group fields by recurring-payment semantics instead of memory layout.
type CreateStandingOrderInput struct {
	OwnerID           uuid.UUID
	SourceAccountID   uuid.UUID
	BeneficiaryName   string
	BeneficiaryIBAN   string
	BeneficiaryBIC    string
	Amount            string
	Purpose           string
	CreditorReference string
	TransferType      string
	Frequency         string
	StartDate         time.Time
	EndDate           *time.Time
	MaxOccurrences    *int32
}

// CreateStandingOrder creates an active recurring payment owned by the source-account holder.
func (s *PaymentService) CreateStandingOrder(ctx context.Context, input CreateStandingOrderInput) (sqlc.StandingOrder, error) {
	input.BeneficiaryName = strings.TrimSpace(input.BeneficiaryName)
	input.BeneficiaryIBAN = sepa.NormalizeIBAN(input.BeneficiaryIBAN)
	input.TransferType = strings.ToUpper(strings.TrimSpace(input.TransferType))
	input.Frequency = strings.ToUpper(strings.TrimSpace(input.Frequency))
	amount, err := decimal.NewFromString(input.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) || amount.Exponent() < -2 || input.BeneficiaryName == "" {
		return sqlc.StandingOrder{}, ErrStandingOrderInvalid
	}
	if validationErr := sepa.ValidateIBAN(input.BeneficiaryIBAN); validationErr != nil {
		return sqlc.StandingOrder{}, validationErr
	}
	if input.TransferType != PaymentStandard && input.TransferType != PaymentInstant {
		return sqlc.StandingOrder{}, ErrStandingOrderInvalid
	}
	if !validFrequency(input.Frequency) || input.StartDate.Before(s.now().AddDate(0, 0, -1)) {
		return sqlc.StandingOrder{}, ErrStandingOrderInvalid
	}
	if input.EndDate != nil && input.EndDate.Before(input.StartDate) {
		return sqlc.StandingOrder{}, ErrStandingOrderInvalid
	}
	source, err := s.store.GetAccount(ctx, input.SourceAccountID)
	if err != nil || source.IsSystem || !source.OwnerID.Valid || source.OwnerID.UUID != input.OwnerID {
		return sqlc.StandingOrder{}, ErrPaymentUnauthorized
	}
	var beneficiaryAccount uuid.NullUUID
	if destination, destinationErr := s.store.GetAccountByIBAN(ctx, input.BeneficiaryIBAN); destinationErr == nil && !destination.IsSystem {
		if destination.ID == source.ID {
			return sqlc.StandingOrder{}, ErrSameAccountTransfer
		}
		beneficiaryAccount = uuid.NullUUID{UUID: destination.ID, Valid: true}
	} else if destinationErr != nil && destinationErr != sql.ErrNoRows {
		return sqlc.StandingOrder{}, destinationErr
	}

	params := sqlc.CreateStandingOrderParams{
		OwnerID: input.OwnerID, SourceAccountID: input.SourceAccountID,
		BeneficiaryAccountID: beneficiaryAccount, BeneficiaryName: input.BeneficiaryName,
		BeneficiaryIban: input.BeneficiaryIBAN, BeneficiaryBic: nullString(input.BeneficiaryBIC),
		Amount: amount.StringFixed(2), Purpose: nullString(input.Purpose),
		CreditorReference: nullString(input.CreditorReference), TransferType: input.TransferType,
		Frequency: input.Frequency, StartDate: input.StartDate,
		NextExecutionAt: input.StartDate,
	}
	if input.EndDate != nil {
		params.EndDate = sql.NullTime{Time: *input.EndDate, Valid: true}
	}
	if input.MaxOccurrences != nil {
		params.MaxOccurrences = sql.NullInt32{Int32: *input.MaxOccurrences, Valid: true}
	}
	order, err := s.store.CreateStandingOrder(ctx, params)
	if err != nil {
		return order, err
	}
	if auditErr := s.audit(ctx, input.OwnerID, uuid.Nil, "STANDING_ORDER_CREATED", map[string]any{"standing_order_id": order.ID}); auditErr != nil {
		return order, auditErr
	}
	s.hub.Publish(input.OwnerID)
	return order, nil
}

// UpdateStandingOrder changes the mutable fields of an owner-authorized standing order.
func (s *PaymentService) UpdateStandingOrder(ctx context.Context, ownerID, orderID uuid.UUID, amount, purpose, status string, endDate *time.Time, maxOccurrences *int32) (sqlc.StandingOrder, error) {
	value, err := decimal.NewFromString(amount)
	if err != nil || value.LessThanOrEqual(decimal.Zero) || value.Exponent() < -2 {
		return sqlc.StandingOrder{}, ErrStandingOrderInvalid
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "ACTIVE" && status != "PAUSED" {
		return sqlc.StandingOrder{}, ErrStandingOrderInvalid
	}
	params := sqlc.UpdateStandingOrderParams{
		Amount: value.StringFixed(2), Purpose: nullString(purpose), Status: status,
		StandingOrderID: orderID, OwnerID: ownerID,
	}
	if endDate != nil {
		params.EndDate = sql.NullTime{Time: *endDate, Valid: true}
	}
	if maxOccurrences != nil {
		params.MaxOccurrences = sql.NullInt32{Int32: *maxOccurrences, Valid: true}
	}
	order, err := s.store.UpdateStandingOrder(ctx, params)
	if err == sql.ErrNoRows {
		return sqlc.StandingOrder{}, ErrPaymentNotFound
	}
	if err == nil {
		s.hub.Publish(ownerID)
	}
	return order, err
}

// CancelStandingOrder prevents a standing order from creating new occurrences.
func (s *PaymentService) CancelStandingOrder(ctx context.Context, ownerID, orderID uuid.UUID) (sqlc.StandingOrder, error) {
	order, err := s.store.DeleteStandingOrder(ctx, sqlc.DeleteStandingOrderParams{StandingOrderID: orderID, OwnerID: ownerID})
	if err == sql.ErrNoRows {
		return sqlc.StandingOrder{}, ErrInvalidPaymentState
	}
	if err == nil {
		s.hub.Publish(ownerID)
	}
	return order, err
}

func validFrequency(value string) bool {
	return value == "WEEKLY" || value == "MONTHLY" || value == "QUARTERLY" || value == "YEARLY"
}

func nextStandingExecution(current time.Time, frequency string) time.Time {
	switch frequency {
	case "WEEKLY":
		return current.AddDate(0, 0, 7)
	case "MONTHLY":
		return addMonthsClamped(current, 1)
	case "QUARTERLY":
		return addMonthsClamped(current, 3)
	default:
		return addMonthsClamped(current, 12)
	}
}

func addMonthsClamped(current time.Time, months int) time.Time {
	targetMonthStart := time.Date(current.Year(), current.Month()+time.Month(months), 1, current.Hour(), current.Minute(), current.Second(), current.Nanosecond(), current.Location())
	lastDay := time.Date(targetMonthStart.Year(), targetMonthStart.Month()+1, 0, current.Hour(), current.Minute(), current.Second(), current.Nanosecond(), current.Location()).Day()
	day := min(current.Day(), lastDay)
	return time.Date(targetMonthStart.Year(), targetMonthStart.Month(), day, current.Hour(), current.Minute(), current.Second(), current.Nanosecond(), current.Location())
}

func shouldCompleteStanding(order sqlc.StandingOrder, next time.Time) bool {
	if order.MaxOccurrences.Valid && order.OccurrencesCreated+1 >= order.MaxOccurrences.Int32 {
		return true
	}
	return order.EndDate.Valid && next.After(order.EndDate.Time)
}

var _ = errors.Is

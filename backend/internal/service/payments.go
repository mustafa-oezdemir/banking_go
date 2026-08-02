package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/mustafa-oezdemir/banking_go/internal/db"
	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

const (
	PaymentAwaitingConfirmation = "AWAITING_CONFIRMATION"
	PaymentScheduled            = "SCHEDULED"
	PaymentProcessing           = "PROCESSING"
	PaymentBooked               = "BOOKED"
	PaymentFailed               = "FAILED"

	ScheduleImmediate = "IMMEDIATE"
	ScheduleScheduled = "SCHEDULED"

	PaymentStandard = "STANDARD"
	PaymentInstant  = "INSTANT"
)

var (
	ErrPaymentNotFound      = errors.New("payment order not found")
	ErrInvalidPaymentState  = errors.New("payment cannot transition from its current state")
	ErrVoPOverrideRequired  = errors.New("explicit confirmation is required for this payee verification result")
	ErrIdempotencyConflict  = errors.New("idempotency key was already used for a different payment")
	ErrAccountBlocked       = errors.New("account is not active")
	ErrPaymentUnauthorized  = errors.New("source account does not belong to the authenticated user")
	ErrInvalidPaymentInput  = errors.New("invalid payment input")
	ErrStandingOrderInvalid = errors.New("invalid standing order")
)

// PaymentService owns payment state transitions and double-entry booking.
type PaymentService struct {
	store *db.Store
	hub   *EventHub
	now   func() time.Time
}

func NewPaymentService(store *db.Store, hub *EventHub) *PaymentService {
	if hub == nil {
		hub = NewEventHub()
	}
	return &PaymentService{store: store, hub: hub, now: time.Now}
}

func (s *PaymentService) EventHub() *EventHub { return s.hub }

type CreatePaymentInput struct {
	OwnerID            uuid.UUID
	SourceAccountID    uuid.UUID
	BeneficiaryName    string
	BeneficiaryIBAN    string
	BeneficiaryBIC     string
	Amount             string
	TransferType       string
	ScheduleType       string
	Purpose            string
	CreditorReference  string
	RequestedExecution time.Time
	IdempotencyKey     string
	StandingOrderID    uuid.NullUUID
}

type CreatePaymentResult struct {
	Order    sqlc.PaymentOrder
	Replayed bool
}

// CreatePayment creates one awaiting-confirmation order. The idempotency key is
// scoped to the authenticated owner and never books funds by itself.
func (s *PaymentService) CreatePayment(ctx context.Context, input CreatePaymentInput) (CreatePaymentResult, error) {
	input.BeneficiaryName = strings.TrimSpace(input.BeneficiaryName)
	input.BeneficiaryIBAN = sepa.NormalizeIBAN(input.BeneficiaryIBAN)
	input.BeneficiaryBIC = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(input.BeneficiaryBIC), " ", ""))
	input.Purpose = strings.TrimSpace(input.Purpose)
	input.CreditorReference = strings.TrimSpace(input.CreditorReference)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.TransferType = strings.ToUpper(strings.TrimSpace(input.TransferType))
	input.ScheduleType = strings.ToUpper(strings.TrimSpace(input.ScheduleType))

	amount, err := validatePaymentInput(input)
	if err != nil {
		return CreatePaymentResult{}, err
	}

	if existing, lookupErr := s.store.GetPaymentOrderByIdempotency(ctx, sqlc.GetPaymentOrderByIdempotencyParams{
		OwnerID: input.OwnerID, IdempotencyKey: input.IdempotencyKey,
	}); lookupErr == nil {
		if !samePaymentIntent(existing, input, amount) {
			return CreatePaymentResult{}, ErrIdempotencyConflict
		}
		return CreatePaymentResult{Order: existing, Replayed: true}, nil
	} else if lookupErr != sql.ErrNoRows {
		return CreatePaymentResult{}, lookupErr
	}

	source, err := s.store.GetAccount(ctx, input.SourceAccountID)
	if err != nil {
		return CreatePaymentResult{}, ErrAccountNotFound
	}
	if source.IsSystem || !source.OwnerID.Valid || source.OwnerID.UUID != input.OwnerID {
		return CreatePaymentResult{}, ErrPaymentUnauthorized
	}
	if source.Status != "ACTIVE" {
		return CreatePaymentResult{}, ErrAccountBlocked
	}
	if source.Currency != "EUR" {
		return CreatePaymentResult{}, ErrCurrencyMismatch
	}

	var beneficiaryAccount uuid.NullUUID
	paymentKind := "SEPA"
	if destination, destinationErr := s.store.GetAccountByIBAN(ctx, input.BeneficiaryIBAN); destinationErr == nil && !destination.IsSystem {
		beneficiaryAccount = uuid.NullUUID{UUID: destination.ID, Valid: true}
		if destination.ID == source.ID {
			return CreatePaymentResult{}, ErrSameAccountTransfer
		}
		if destination.OwnerID.Valid && destination.OwnerID.UUID == input.OwnerID {
			paymentKind = "UMBUCHUNG"
		} else {
			paymentKind = "INTERNAL"
		}
	} else if destinationErr != nil && destinationErr != sql.ErrNoRows {
		return CreatePaymentResult{}, destinationErr
	} else if input.TransferType == PaymentInstant {
		paymentKind = "SEPA_INSTANT"
	}

	vop, err := s.VerifyPayee(ctx, input.OwnerID, input.BeneficiaryName, input.BeneficiaryIBAN)
	if err != nil {
		return CreatePaymentResult{}, err
	}

	execution := input.RequestedExecution
	if input.ScheduleType == ScheduleImmediate {
		execution = s.now().UTC()
	}
	endToEndID := "DEMO-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:27]
	params := sqlc.CreatePaymentOrderParams{
		OwnerID:              input.OwnerID,
		SourceAccountID:      input.SourceAccountID,
		BeneficiaryAccountID: beneficiaryAccount,
		StandingOrderID:      input.StandingOrderID,
		BeneficiaryName:      input.BeneficiaryName,
		BeneficiaryIban:      input.BeneficiaryIBAN,
		BeneficiaryBic:       nullString(input.BeneficiaryBIC),
		Amount:               amount.StringFixed(2),
		PaymentKind:          paymentKind,
		ScheduleType:         input.ScheduleType,
		Purpose:              nullString(input.Purpose),
		CreditorReference:    nullString(input.CreditorReference),
		EndToEndID:           endToEndID,
		IdempotencyKey:       input.IdempotencyKey,
		RequestedExecutionAt: execution,
		VopResult:            vop.Result,
		VopSuggestedName:     optionalString(vop.SuggestedName),
		Status:               PaymentAwaitingConfirmation,
	}
	order, err := s.store.CreatePaymentOrder(ctx, params)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			existing, lookupErr := s.store.GetPaymentOrderByIdempotency(ctx, sqlc.GetPaymentOrderByIdempotencyParams{
				OwnerID: input.OwnerID, IdempotencyKey: input.IdempotencyKey,
			})
			if lookupErr == nil && samePaymentIntent(existing, input, amount) {
				return CreatePaymentResult{Order: existing, Replayed: true}, nil
			}
			return CreatePaymentResult{}, ErrIdempotencyConflict
		}
		return CreatePaymentResult{}, err
	}
	if err := s.audit(ctx, order.OwnerID, order.ID, "PAYMENT_CREATED", map[string]any{
		"status": order.Status, "vop_result": order.VopResult, "payment_kind": order.PaymentKind,
	}); err != nil {
		return CreatePaymentResult{}, err
	}
	s.hub.Publish(order.OwnerID)
	return CreatePaymentResult{Order: order}, nil
}

// ConfirmPayment records the user's VoP decision and either schedules or books
// the order. Any booking failure leaves no partial ledger entry.
func (s *PaymentService) ConfirmPayment(ctx context.Context, ownerID, paymentID uuid.UUID, acceptMismatch bool) (sqlc.PaymentOrder, error) {
	var result sqlc.PaymentOrder
	var businessErr error
	err := s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		order, err := q.GetPaymentOrderForUpdate(ctx, paymentID)
		if err == sql.ErrNoRows || order.OwnerID != ownerID {
			return ErrPaymentNotFound
		}
		if err != nil {
			return err
		}
		if order.Status != PaymentAwaitingConfirmation {
			return ErrInvalidPaymentState
		}
		requiresOverride := order.VopResult != VoPMatch
		if requiresOverride && !acceptMismatch {
			return ErrVoPOverrideRequired
		}

		if order.ScheduleType == ScheduleScheduled && order.RequestedExecutionAt.After(s.now()) {
			result, err = q.ConfirmPaymentOrder(ctx, sqlc.ConfirmPaymentOrderParams{
				Status: PaymentScheduled, VopOverridden: requiresOverride,
				PaymentOrderID: paymentID, OwnerID: ownerID,
			})
			if err != nil {
				return err
			}
			return s.auditWithQueries(ctx, q, ownerID, paymentID, "PAYMENT_SCHEDULED", map[string]any{
				"execution_at": result.RequestedExecutionAt,
			})
		}

		order, err = q.ConfirmPaymentOrder(ctx, sqlc.ConfirmPaymentOrderParams{
			Status: PaymentProcessing, VopOverridden: requiresOverride,
			PaymentOrderID: paymentID, OwnerID: ownerID,
		})
		if err != nil {
			return err
		}
		booked, executeErr := s.bookPaymentTx(ctx, q, order)
		if executeErr != nil {
			failed, failErr := markFailed(ctx, q, order, executeErr)
			if failErr != nil {
				return failErr
			}
			result = failed
			businessErr = executeErr
			return s.auditWithQueries(ctx, q, ownerID, paymentID, "PAYMENT_FAILED", map[string]any{
				"reason": publicFailureReason(executeErr),
			})
		}
		result = booked
		return s.auditWithQueries(ctx, q, ownerID, paymentID, "PAYMENT_BOOKED", map[string]any{
			"ledger_transaction_id": booked.LedgerTransactionID.UUID,
		})
	})
	if err != nil {
		return sqlc.PaymentOrder{}, err
	}
	s.hub.Publish(ownerID)
	return result, businessErr
}

func (s *PaymentService) CancelPayment(ctx context.Context, ownerID, paymentID uuid.UUID) (sqlc.PaymentOrder, error) {
	order, err := s.store.CancelPaymentOrder(ctx, sqlc.CancelPaymentOrderParams{PaymentOrderID: paymentID, OwnerID: ownerID})
	if err == sql.ErrNoRows {
		return sqlc.PaymentOrder{}, ErrInvalidPaymentState
	}
	if err != nil {
		return sqlc.PaymentOrder{}, err
	}
	if auditErr := s.audit(ctx, ownerID, paymentID, "PAYMENT_CANCELLED", map[string]any{}); auditErr != nil {
		return sqlc.PaymentOrder{}, auditErr
	}
	s.hub.Publish(ownerID)
	return order, nil
}

func (s *PaymentService) GetPayment(ctx context.Context, ownerID, paymentID uuid.UUID) (sqlc.PaymentOrder, error) {
	order, err := s.store.GetPaymentOrder(ctx, paymentID)
	if err == sql.ErrNoRows || order.OwnerID != ownerID {
		return sqlc.PaymentOrder{}, ErrPaymentNotFound
	}
	return order, err
}

func (s *PaymentService) ListPayments(ctx context.Context, ownerID uuid.UUID, limit, offset int32) ([]sqlc.PaymentOrder, error) {
	return s.store.ListPaymentOrdersByOwner(ctx, sqlc.ListPaymentOrdersByOwnerParams{
		OwnerID: ownerID, ResultLimit: limit, ResultOffset: offset,
	})
}

func (s *PaymentService) bookPaymentTx(ctx context.Context, q *sqlc.Queries, order sqlc.PaymentOrder) (sqlc.PaymentOrder, error) {
	amount, err := decimal.NewFromString(order.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return sqlc.PaymentOrder{}, ErrInvalidAmount
	}
	destinationID := uuid.Nil
	if order.BeneficiaryAccountID.Valid {
		destinationID = order.BeneficiaryAccountID.UUID
	} else {
		settlement, settlementErr := q.GetSettlementAccount(ctx)
		if settlementErr != nil {
			return sqlc.PaymentOrder{}, settlementErr
		}
		destinationID = settlement.ID
	}
	if destinationID == order.SourceAccountID {
		return sqlc.PaymentOrder{}, ErrSameAccountTransfer
	}

	accounts, err := q.ListAccountsForUpdate(ctx, []uuid.UUID{order.SourceAccountID, destinationID})
	if err != nil {
		return sqlc.PaymentOrder{}, err
	}
	if len(accounts) != 2 {
		return sqlc.PaymentOrder{}, ErrAccountNotFound
	}
	accountByID := map[uuid.UUID]sqlc.Account{accounts[0].ID: accounts[0], accounts[1].ID: accounts[1]}
	source := accountByID[order.SourceAccountID]
	destination := accountByID[destinationID]
	if source.IsSystem || !source.OwnerID.Valid || source.OwnerID.UUID != order.OwnerID {
		return sqlc.PaymentOrder{}, ErrPaymentUnauthorized
	}
	if source.Status != "ACTIVE" || destination.Status != "ACTIVE" {
		return sqlc.PaymentOrder{}, ErrAccountBlocked
	}
	if source.Currency != "EUR" || destination.Currency != "EUR" {
		return sqlc.PaymentOrder{}, ErrCurrencyMismatch
	}
	available, err := decimal.NewFromString(source.AvailableBalance)
	if err != nil {
		return sqlc.PaymentOrder{}, errors.New("invalid available balance")
	}
	if available.LessThan(amount) {
		return sqlc.PaymentOrder{}, ErrInsufficientFunds
	}

	txID := uuid.New()
	category := categorizePurpose(order.Purpose.String)
	executionDate := sql.NullTime{Time: s.now().UTC(), Valid: true}
	entryBase := sqlc.CreatePaymentEntryParams{
		TransactionID:  txID,
		PaymentOrderID: uuid.NullUUID{UUID: order.ID, Valid: true},
		Purpose:        order.Purpose,
		Category:       nullString(category),
		ExecutionDate:  executionDate,
	}

	sourceEntry := entryBase
	sourceEntry.AccountID = source.ID
	sourceEntry.Debit = amount.StringFixed(4)
	sourceEntry.Credit = decimal.Zero.StringFixed(4)
	sourceEntry.Description = nullString("SEPA-Demoüberweisung " + order.EndToEndID)
	sourceEntry.CounterpartyName = nullString(order.BeneficiaryName)
	sourceEntry.CounterpartyIban = nullString(sepa.MaskIBAN(order.BeneficiaryIban))
	if _, err = q.CreatePaymentEntry(ctx, sourceEntry); err != nil {
		return sqlc.PaymentOrder{}, err
	}

	sender, senderErr := q.GetUserByID(ctx, order.OwnerID)
	if senderErr != nil {
		return sqlc.PaymentOrder{}, senderErr
	}
	destinationEntry := entryBase
	destinationEntry.AccountID = destination.ID
	destinationEntry.Debit = decimal.Zero.StringFixed(4)
	destinationEntry.Credit = amount.StringFixed(4)
	destinationEntry.Description = nullString("SEPA-Demoeingang " + order.EndToEndID)
	destinationEntry.CounterpartyName = nullString(sender.FullName)
	destinationEntry.CounterpartyIban = nullString(sepa.MaskIBAN(source.Iban))
	if _, err = q.CreatePaymentEntry(ctx, destinationEntry); err != nil {
		return sqlc.PaymentOrder{}, err
	}

	if err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{Balance: amount.Neg().StringFixed(4), ID: source.ID}); err != nil {
		return sqlc.PaymentOrder{}, err
	}
	if err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{Balance: amount.StringFixed(4), ID: destination.ID}); err != nil {
		return sqlc.PaymentOrder{}, err
	}
	return q.MarkPaymentBooked(ctx, sqlc.MarkPaymentBookedParams{
		LedgerTransactionID: uuid.NullUUID{UUID: txID, Valid: true}, PaymentOrderID: order.ID,
	})
}

func validatePaymentInput(input CreatePaymentInput) (decimal.Decimal, error) {
	if input.OwnerID == uuid.Nil || input.SourceAccountID == uuid.Nil || input.BeneficiaryName == "" || utf8.RuneCountInString(input.BeneficiaryName) > 140 {
		return decimal.Zero, ErrInvalidPaymentInput
	}
	if err := sepa.ValidateIBAN(input.BeneficiaryIBAN); err != nil {
		return decimal.Zero, err
	}
	amount, err := decimal.NewFromString(input.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) || amount.Exponent() < -2 {
		return decimal.Zero, ErrInvalidAmount
	}
	if utf8.RuneCountInString(input.Purpose) > 140 {
		return decimal.Zero, ErrInvalidPaymentInput
	}
	if len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 128 {
		return decimal.Zero, ErrInvalidPaymentInput
	}
	if input.TransferType == "" {
		input.TransferType = PaymentStandard
	}
	if input.TransferType != PaymentStandard && input.TransferType != PaymentInstant {
		return decimal.Zero, ErrInvalidPaymentInput
	}
	if input.ScheduleType != ScheduleImmediate && input.ScheduleType != ScheduleScheduled {
		return decimal.Zero, ErrInvalidPaymentInput
	}
	if input.ScheduleType == ScheduleScheduled && input.RequestedExecution.Before(time.Now().Add(-time.Minute)) {
		return decimal.Zero, ErrInvalidPaymentInput
	}
	return amount, nil
}

func samePaymentIntent(order sqlc.PaymentOrder, input CreatePaymentInput, amount decimal.Decimal) bool {
	storedAmount, err := decimal.NewFromString(order.Amount)
	if err != nil {
		return false
	}
	sameExecution := true
	if input.ScheduleType == ScheduleScheduled {
		sameExecution = order.RequestedExecutionAt.Equal(input.RequestedExecution.UTC())
	}
	wantsInstant := input.TransferType == PaymentInstant
	isInstant := order.PaymentKind == "SEPA_INSTANT"
	return order.SourceAccountID == input.SourceAccountID &&
		order.BeneficiaryIban == input.BeneficiaryIBAN &&
		order.BeneficiaryName == input.BeneficiaryName &&
		order.BeneficiaryBic.String == input.BeneficiaryBIC &&
		storedAmount.Equal(amount) &&
		order.ScheduleType == input.ScheduleType &&
		order.Purpose.String == input.Purpose &&
		order.CreditorReference.String == input.CreditorReference &&
		sameExecution &&
		(wantsInstant == isInstant || order.PaymentKind == "INTERNAL" || order.PaymentKind == "UMBUCHUNG")
}

func markFailed(ctx context.Context, q *sqlc.Queries, order sqlc.PaymentOrder, cause error) (sqlc.PaymentOrder, error) {
	return q.MarkPaymentFailed(ctx, sqlc.MarkPaymentFailedParams{
		FailureReason:  nullString(publicFailureReason(cause)),
		RejectCode:     nullString(rejectCode(cause)),
		PaymentOrderID: order.ID,
	})
}

func rejectCode(err error) string {
	switch {
	case errors.Is(err, ErrInsufficientFunds):
		return "AM04"
	case errors.Is(err, ErrAccountBlocked):
		return "AC06"
	case errors.Is(err, ErrCurrencyMismatch):
		return "CURR"
	default:
		return "TECH"
	}
}

func publicFailureReason(err error) string {
	switch {
	case errors.Is(err, ErrInsufficientFunds):
		return "Nicht ausreichende Deckung"
	case errors.Is(err, ErrAccountBlocked):
		return "Konto ist nicht aktiv"
	case errors.Is(err, ErrCurrencyMismatch):
		return "Währung wird nicht unterstützt"
	case errors.Is(err, ErrSameAccountTransfer):
		return "Quell- und Zielkonto sind identisch"
	default:
		return "Technischer Verarbeitungsfehler"
	}
}

func categorizePurpose(purpose string) string {
	value := strings.ToLower(purpose)
	for category, words := range map[string][]string{
		"Wohnen":       {"miete", "wohnung", "strom"},
		"Lebensmittel": {"markt", "supermarkt", "lebensmittel"},
		"Mobilität":    {"bahn", "ticket", "verkehr", "tanken"},
		"Abonnements":  {"abo", "netflix", "spotify", "subscription"},
		"Gehalt":       {"gehalt", "lohn"},
	} {
		for _, word := range words {
			if strings.Contains(value, word) {
				return category
			}
		}
	}
	return "Sonstiges"
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func optionalString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return nullString(*value)
}

func (s *PaymentService) audit(ctx context.Context, ownerID, paymentID uuid.UUID, eventType string, data map[string]any) error {
	return s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		return s.auditWithQueries(ctx, q, ownerID, paymentID, eventType, data)
	})
}

func (s *PaymentService) auditWithQueries(ctx context.Context, q *sqlc.Queries, ownerID, paymentID uuid.UUID, eventType string, data map[string]any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode audit event: %w", err)
	}
	_, err = q.CreateAuditEvent(ctx, sqlc.CreateAuditEventParams{
		OwnerID:        uuid.NullUUID{UUID: ownerID, Valid: true},
		PaymentOrderID: uuid.NullUUID{UUID: paymentID, Valid: paymentID != uuid.Nil},
		EventType:      eventType,
		EventData:      payload,
	})
	return err
}

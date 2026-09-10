package payment

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
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	sepa "github.com/mustafa-oezdemir/banking_go/internal/account"
	"github.com/mustafa-oezdemir/banking_go/internal/ledger"
	ledgerdomain "github.com/mustafa-oezdemir/banking_go/internal/ledger/domain"
	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	paymentdomain "github.com/mustafa-oezdemir/banking_go/internal/payment/domain"
	db "github.com/mustafa-oezdemir/banking_go/internal/platform/database"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

const (
	// PaymentAwaitingConfirmation is the state before explicit demo consent.
	PaymentAwaitingConfirmation = string(paymentdomain.StatusAwaitingConfirmation)
	// PaymentScheduled is the state of a confirmed future payment.
	PaymentScheduled = string(paymentdomain.StatusScheduled)
	// PaymentProcessing is the exclusively claimed booking state.
	PaymentProcessing = string(paymentdomain.StatusProcessing)
	// PaymentBooked is the terminal successful state.
	PaymentBooked = string(paymentdomain.StatusBooked)
	// PaymentFailed is the terminal rejected or failed state.
	PaymentFailed = string(paymentdomain.StatusFailed)

	// ScheduleImmediate requests processing during confirmation.
	ScheduleImmediate = "IMMEDIATE"
	// ScheduleScheduled requests processing at a future execution time.
	ScheduleScheduled = "SCHEDULED"

	// PaymentStandard selects the simulated standard transfer rail.
	PaymentStandard = "STANDARD"
	// PaymentInstant selects the simulated instant transfer rail.
	PaymentInstant = "INSTANT"
)

var (
	// ErrPaymentNotFound indicates an unknown or owner-inaccessible payment.
	ErrPaymentNotFound = errors.New("payment order not found")
	// ErrInvalidPaymentState indicates a disallowed state-machine transition.
	ErrInvalidPaymentState = paymentdomain.ErrInvalidTransition
	// ErrVoPOverrideRequired requires explicit consent for a non-match result.
	ErrVoPOverrideRequired = errors.New("explicit confirmation is required for this payee verification result")
	// ErrIdempotencyConflict indicates reuse of a key with a changed intent.
	ErrIdempotencyConflict = errors.New("idempotency key was already used for a different payment")
	// ErrAccountBlocked indicates that the source or destination is inactive.
	ErrAccountBlocked = ledger.ErrAccountBlocked
	// ErrPaymentUnauthorized indicates that the source is not owned by the caller.
	ErrPaymentUnauthorized = errors.New("source account does not belong to the authenticated user")
	// ErrInvalidPaymentInput indicates malformed payment intent data.
	ErrInvalidPaymentInput = errors.New("invalid payment input")
	// ErrStandingOrderInvalid indicates malformed recurring-payment data.
	ErrStandingOrderInvalid = errors.New("invalid standing order")
	// ErrInvalidAmount preserves the ledger amount error at the payment boundary.
	ErrInvalidAmount = ledger.ErrInvalidAmount
	// ErrCurrencyMismatch preserves the ledger currency error at the payment boundary.
	ErrCurrencyMismatch = ledger.ErrCurrencyMismatch
	// ErrInsufficientFunds preserves the ledger balance error at the payment boundary.
	ErrInsufficientFunds = ledger.ErrInsufficientFunds
	// ErrAccountNotFound preserves the ledger account error at the payment boundary.
	ErrAccountNotFound = ledger.ErrAccountNotFound
	// ErrSameAccountTransfer preserves the ledger same-account error at the payment boundary.
	ErrSameAccountTransfer = ledger.ErrSameAccountTransfer
)

// Service owns payment state transitions and double-entry booking.
type Service struct {
	store    *db.Store
	hub      *EventHub
	now      func() time.Time
	notifier notification.Sender
}

// NewService creates a payment orchestration service for a store.
func NewService(store *db.Store, hub *EventHub) *Service {
	if hub == nil {
		hub = NewEventHub()
	}
	return &Service{store: store, hub: hub, now: time.Now, notifier: notification.NoopSender{}}
}

// SetNotificationSender enables post-commit account activity emails.
func (s *Service) SetNotificationSender(sender notification.Sender) {
	if sender != nil {
		s.notifier = sender
	}
}

// EventHub returns the service's lightweight SSE notification hub.
func (s *Service) EventHub() *EventHub { return s.hub }

// CreatePaymentInput describes one normalized payment intent.
//
//nolint:govet // Group fields by payment semantics instead of memory layout.
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

// CreatePaymentResult contains a new or idempotently replayed order.
type CreatePaymentResult struct {
	Order    sqlc.PaymentOrder
	Replayed bool
}

// CreatePayment creates one awaiting-confirmation order. The idempotency key is
// scoped to the authenticated owner and never books funds by itself.
func (s *Service) CreatePayment(ctx context.Context, input CreatePaymentInput) (CreatePaymentResult, error) {
	input.BeneficiaryName = strings.TrimSpace(input.BeneficiaryName)
	input.BeneficiaryIBAN = sepa.NormalizeIBAN(input.BeneficiaryIBAN)
	input.BeneficiaryBIC = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(input.BeneficiaryBIC), " ", ""))
	input.Purpose = strings.TrimSpace(input.Purpose)
	input.CreditorReference = strings.TrimSpace(input.CreditorReference)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.TransferType = strings.ToUpper(strings.TrimSpace(input.TransferType))
	input.ScheduleType = strings.ToUpper(strings.TrimSpace(input.ScheduleType))

	amount, err := validatePaymentInput(input, s.now())
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
	if !sepa.CustomerCanOperate(input.OwnerID, source.OwnerID.UUID, source.OwnerID.Valid, source.IsSystem) {
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
	} else {
		// PostgreSQL stores timestamps at microsecond precision. Canonicalize the
		// value before insertion so an idempotent replay compares the same value.
		execution = execution.UTC().Round(time.Microsecond)
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
		if db.IsUniqueViolation(err) {
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
func (s *Service) ConfirmPayment(ctx context.Context, ownerID, paymentID uuid.UUID, acceptMismatch bool) (sqlc.PaymentOrder, error) {
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

		targetStatus := PaymentProcessing
		if order.ScheduleType == ScheduleScheduled && order.RequestedExecutionAt.After(s.now()) {
			targetStatus = PaymentScheduled
		}
		if transitionErr := paymentdomain.ValidateTransition(
			paymentdomain.Status(order.Status), paymentdomain.Status(targetStatus),
		); transitionErr != nil {
			return ErrInvalidPaymentState
		}

		if targetStatus == PaymentScheduled {
			result, err = q.ConfirmPaymentOrder(ctx, sqlc.ConfirmPaymentOrderParams{
				Status: targetStatus, VopOverridden: requiresOverride,
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
			Status: targetStatus, VopOverridden: requiresOverride,
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
	if businessErr == nil && result.Status == PaymentBooked {
		s.notifyBookedPayment(ctx, result)
	}
	return result, businessErr
}

// CancelPayment cancels an owner-authorized draft or scheduled order.
func (s *Service) CancelPayment(ctx context.Context, ownerID, paymentID uuid.UUID) (sqlc.PaymentOrder, error) {
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

// GetPayment returns one owner-authorized payment order.
func (s *Service) GetPayment(ctx context.Context, ownerID, paymentID uuid.UUID) (sqlc.PaymentOrder, error) {
	order, err := s.store.GetPaymentOrder(ctx, paymentID)
	if err == sql.ErrNoRows || order.OwnerID != ownerID {
		return sqlc.PaymentOrder{}, ErrPaymentNotFound
	}
	return order, err
}

// ListPayments returns a page of payment orders for an owner.
func (s *Service) ListPayments(ctx context.Context, ownerID uuid.UUID, limit, offset int32) ([]sqlc.PaymentOrder, error) {
	return s.store.ListPaymentOrdersByOwner(ctx, sqlc.ListPaymentOrdersByOwnerParams{
		OwnerID: ownerID, ResultLimit: limit, ResultOffset: offset,
	})
}

func (s *Service) bookPaymentTx(ctx context.Context, q *sqlc.Queries, order sqlc.PaymentOrder) (sqlc.PaymentOrder, error) {
	if err := paymentdomain.ValidateTransition(paymentdomain.Status(order.Status), paymentdomain.StatusBooked); err != nil {
		return sqlc.PaymentOrder{}, ErrInvalidPaymentState
	}
	amount, err := decimal.NewFromString(order.Amount)
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return sqlc.PaymentOrder{}, ErrInvalidAmount
	}
	var destinationID uuid.UUID
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
	available, err := decimal.NewFromString(source.AvailableBalance)
	if err != nil {
		return sqlc.PaymentOrder{}, errors.New("invalid available balance")
	}
	posting, err := ledgerdomain.PlanCustomerTransfer(
		order.OwnerID,
		ledgerdomain.AccountSnapshot{
			ID: source.ID, OwnerID: source.OwnerID.UUID, OwnerAssigned: source.OwnerID.Valid,
			Currency: source.Currency, Status: source.Status, AvailableBalance: available, System: source.IsSystem,
		},
		ledgerdomain.AccountSnapshot{
			ID: destination.ID, OwnerID: destination.OwnerID.UUID, OwnerAssigned: destination.OwnerID.Valid,
			Currency: destination.Currency, Status: destination.Status, System: destination.IsSystem,
		},
		amount,
		ledgerdomain.TransferPolicy{AllowSystemDestination: true},
	)
	if err != nil {
		if errors.Is(err, ledgerdomain.ErrAccountOwnership) || errors.Is(err, ledgerdomain.ErrSystemAccount) {
			return sqlc.PaymentOrder{}, ErrPaymentUnauthorized
		}
		return sqlc.PaymentOrder{}, err
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
	sourceEntry.AccountID = posting.DebitLeg.AccountID
	sourceEntry.Debit = posting.DebitLeg.Debit.StringFixed(4)
	sourceEntry.Credit = posting.DebitLeg.Credit.StringFixed(4)
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
	destinationEntry.AccountID = posting.CreditLeg.AccountID
	destinationEntry.Debit = posting.CreditLeg.Debit.StringFixed(4)
	destinationEntry.Credit = posting.CreditLeg.Credit.StringFixed(4)
	destinationEntry.Description = nullString("SEPA-Demoeingang " + order.EndToEndID)
	destinationEntry.CounterpartyName = nullString(sender.FullName)
	destinationEntry.CounterpartyIban = nullString(sepa.MaskIBAN(source.Iban))
	if _, err = q.CreatePaymentEntry(ctx, destinationEntry); err != nil {
		return sqlc.PaymentOrder{}, err
	}

	if err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
		Balance: posting.DebitLeg.Debit.Neg().StringFixed(4), ID: posting.DebitLeg.AccountID,
	}); err != nil {
		return sqlc.PaymentOrder{}, err
	}
	if err = q.UpdateAccountBalance(ctx, sqlc.UpdateAccountBalanceParams{
		Balance: posting.CreditLeg.Credit.StringFixed(4), ID: posting.CreditLeg.AccountID,
	}); err != nil {
		return sqlc.PaymentOrder{}, err
	}
	return q.MarkPaymentBooked(ctx, sqlc.MarkPaymentBookedParams{
		LedgerTransactionID: uuid.NullUUID{UUID: txID, Valid: true}, PaymentOrderID: order.ID,
	})
}

func (s *Service) notifyBookedPayment(ctx context.Context, order sqlc.PaymentOrder) {
	if err := s.notifier.NotifyActivity(ctx, notification.Activity{
		UserID: order.OwnerID, AccountID: order.SourceAccountID, Kind: "SEPA_PAYMENT_SENT",
		Direction: "DEBIT", Amount: order.Amount, Currency: "EUR",
		Counterparty: order.BeneficiaryName, Reference: order.Purpose.String,
	}); err != nil {
		log.Warn().Err(err).Str("kind", "SEPA_PAYMENT_SENT").Msg("Post-commit notification failed")
	}
	if !order.BeneficiaryAccountID.Valid {
		return
	}
	destination, err := s.store.GetAccount(ctx, order.BeneficiaryAccountID.UUID)
	if err != nil || destination.IsSystem || !destination.OwnerID.Valid {
		return
	}
	if err = s.notifier.NotifyActivity(ctx, notification.Activity{
		UserID: destination.OwnerID.UUID, AccountID: destination.ID, Kind: "SEPA_PAYMENT_RECEIVED",
		Direction: "CREDIT", Amount: order.Amount, Currency: "EUR",
		Reference: order.Purpose.String,
	}); err != nil {
		log.Warn().Err(err).Str("kind", "SEPA_PAYMENT_RECEIVED").Msg("Post-commit notification failed")
	}
}

func validatePaymentInput(input CreatePaymentInput, now time.Time) (decimal.Decimal, error) {
	if input.OwnerID == uuid.Nil || input.SourceAccountID == uuid.Nil || input.BeneficiaryName == "" || utf8.RuneCountInString(input.BeneficiaryName) > 140 {
		return decimal.Zero, ErrInvalidPaymentInput
	}
	if err := sepa.ValidateIBAN(input.BeneficiaryIBAN); err != nil {
		return decimal.Zero, err
	}
	amount, err := ledger.ParseEURAmount(input.Amount)
	if err != nil {
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
	if input.ScheduleType == ScheduleScheduled && input.RequestedExecution.Before(now.Add(-time.Minute)) {
		return decimal.Zero, ErrInvalidPaymentInput
	}
	return amount, nil
}

func samePaymentIntent(order sqlc.PaymentOrder, input CreatePaymentInput, amount decimal.Decimal) bool {
	storedAmount, err := decimal.NewFromString(order.Amount)
	if err != nil {
		return false
	}
	return paymentdomain.EquivalentIntent(
		paymentdomain.Intent{
			SourceAccountID: order.SourceAccountID, BeneficiaryName: order.BeneficiaryName,
			BeneficiaryIBAN: order.BeneficiaryIban, BeneficiaryBIC: order.BeneficiaryBic.String,
			Amount: storedAmount, ScheduleType: order.ScheduleType, Purpose: order.Purpose.String,
			CreditorReference: order.CreditorReference.String, RequestedExecution: order.RequestedExecutionAt,
			Instant:  order.PaymentKind == "SEPA_INSTANT",
			Internal: order.PaymentKind == "INTERNAL" || order.PaymentKind == "UMBUCHUNG",
		},
		paymentdomain.Intent{
			SourceAccountID: input.SourceAccountID, BeneficiaryName: input.BeneficiaryName,
			BeneficiaryIBAN: input.BeneficiaryIBAN, BeneficiaryBIC: input.BeneficiaryBIC,
			Amount: amount, ScheduleType: input.ScheduleType, Purpose: input.Purpose,
			CreditorReference: input.CreditorReference, RequestedExecution: input.RequestedExecution,
			Instant: input.TransferType == PaymentInstant,
		},
	)
}

func markFailed(ctx context.Context, q *sqlc.Queries, order sqlc.PaymentOrder, cause error) (sqlc.PaymentOrder, error) {
	if err := paymentdomain.ValidateTransition(paymentdomain.Status(order.Status), paymentdomain.StatusFailed); err != nil {
		return sqlc.PaymentOrder{}, ErrInvalidPaymentState
	}
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
		"Lebensmittel": {"markt", "supermarkt", "lebensmittel"}, //nolint:misspell // Correct German term.
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

func (s *Service) audit(ctx context.Context, ownerID, paymentID uuid.UUID, eventType string, data map[string]any) error {
	return s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		return s.auditWithQueries(ctx, q, ownerID, paymentID, eventType, data)
	})
}

func (s *Service) auditWithQueries(ctx context.Context, q *sqlc.Queries, ownerID, paymentID uuid.UUID, eventType string, data map[string]any) error {
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

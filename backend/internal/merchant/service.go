// Package merchant owns immutable merchant checkout instructions and delegates
// all financial booking to the existing Payment application service.
package merchant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/ledger"
	"github.com/mustafa-oezdemir/banking_go/internal/payment"
	db "github.com/mustafa-oezdemir/banking_go/internal/platform/database"
)

const (
	StatusAwaitingCustomer = "AWAITING_CUSTOMER"
	StatusProcessing       = "PROCESSING"
	StatusBooked           = "BOOKED"
	StatusFailed           = "FAILED"
	StatusExpired          = "EXPIRED"
)

var (
	ErrIntentNotFound = errors.New("merchant payment intent not found")
	ErrIntentExpired  = errors.New("merchant payment intent has expired")
	ErrIntentState    = errors.New("merchant payment intent is not awaiting approval")
	ErrMerchantInput  = errors.New("invalid merchant payment intent")
)

// CreateIntentInput is accepted only from an authenticated merchant backend.
type CreateIntentInput struct {
	MerchantID        string
	MerchantReference string
	Amount            string
	Currency          string
}

// ApprovalInput lets an authenticated customer select only the source account.
type ApprovalInput struct {
	CustomerID      uuid.UUID
	SourceAccountID uuid.UUID
	IntentID        uuid.UUID
}

// Service keeps checkout pricing immutable and uses payment.Service for booking.
type Service struct {
	store    *db.Store
	payments *payment.Service
	now      func() time.Time
	ttl      time.Duration
}

// NewService creates the merchant application boundary.
func NewService(store *db.Store, payments *payment.Service) *Service {
	return &Service{store: store, payments: payments, now: time.Now, ttl: 30 * time.Minute}
}

// CreateIntent creates or safely replays a merchant checkout instruction.
func (service *Service) CreateIntent(ctx context.Context, input CreateIntentInput) (db.MerchantPaymentIntent, bool, error) {
	input.MerchantID = strings.ToLower(strings.TrimSpace(input.MerchantID))
	input.MerchantReference = strings.TrimSpace(input.MerchantReference)
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.Currency != "EUR" || input.MerchantID == "" {
		return db.MerchantPaymentIntent{}, false, ErrMerchantInput
	}
	reference, err := payment.NormalizePaymentPlainText(input.MerchantReference, 1, 140, true)
	if err != nil {
		return db.MerchantPaymentIntent{}, false, ErrMerchantInput
	}
	amount, err := ledger.ParseEURAmount(input.Amount)
	if err != nil {
		return db.MerchantPaymentIntent{}, false, ErrMerchantInput
	}
	merchant, err := service.store.GetMerchant(ctx, input.MerchantID)
	if errors.Is(err, db.ErrMerchantNotFound) || !merchant.Active {
		return db.MerchantPaymentIntent{}, false, ErrIntentNotFound
	}
	if err != nil {
		return db.MerchantPaymentIntent{}, false, err
	}
	intent, err := service.store.CreateMerchantPaymentIntent(ctx, merchant, reference, amount, service.now().UTC().Add(service.ttl))
	if err == nil {
		return intent, false, nil
	}
	if !db.IsUniqueViolation(err) {
		return db.MerchantPaymentIntent{}, false, err
	}
	existing, lookupErr := service.store.GetMerchantPaymentIntentByMerchantReference(ctx, merchant.ID, reference)
	if lookupErr != nil {
		return db.MerchantPaymentIntent{}, false, lookupErr
	}
	if !existing.Amount.Equal(amount) || existing.Currency != input.Currency {
		return db.MerchantPaymentIntent{}, false, payment.ErrIdempotencyConflict
	}
	return existing, true, nil
}

// GetIntentForCustomer returns a live opaque intent. Once bound to a payment,
// the corresponding payment ownership check prevents cross-customer disclosure.
func (service *Service) GetIntentForCustomer(ctx context.Context, customerID, intentID uuid.UUID) (db.MerchantPaymentIntent, error) {
	intent, err := service.store.GetMerchantPaymentIntent(ctx, intentID)
	if errors.Is(err, db.ErrMerchantNotFound) {
		return db.MerchantPaymentIntent{}, ErrIntentNotFound
	}
	if err != nil {
		return db.MerchantPaymentIntent{}, err
	}
	if intent.Status == StatusAwaitingCustomer && !intent.ExpiresAt.After(service.now()) {
		_ = service.store.ExpireMerchantPaymentIntent(ctx, intent.ID, service.now())
		return db.MerchantPaymentIntent{}, ErrIntentExpired
	}
	if intent.Status == StatusExpired {
		return db.MerchantPaymentIntent{}, ErrIntentExpired
	}
	if intent.PaymentOrderID.Valid {
		if _, paymentErr := service.payments.GetPayment(ctx, customerID, intent.PaymentOrderID.UUID); paymentErr != nil {
			return db.MerchantPaymentIntent{}, ErrIntentNotFound
		}
	}
	return intent, nil
}

// Approve creates a payment using only persisted intent values, then confirms
// it through the existing ownership-checked double-entry payment service.
func (service *Service) Approve(ctx context.Context, input ApprovalInput) (db.MerchantPaymentIntent, error) {
	intent, err := service.GetIntentForCustomer(ctx, input.CustomerID, input.IntentID)
	if err != nil {
		return db.MerchantPaymentIntent{}, err
	}
	if intent.Status != StatusAwaitingCustomer {
		return db.MerchantPaymentIntent{}, ErrIntentState
	}
	created, err := service.payments.CreatePayment(ctx, payment.CreatePaymentInput{
		OwnerID: input.CustomerID, SourceAccountID: input.SourceAccountID,
		BeneficiaryName: intent.MerchantName, BeneficiaryIBAN: intent.BeneficiaryIBAN,
		Amount: intent.Amount.StringFixed(2), TransferType: payment.PaymentStandard,
		ScheduleType: payment.ScheduleImmediate, Purpose: intent.MerchantReference,
		IdempotencyKey: fmt.Sprintf("merchant:%s", intent.ID),
	})
	if err != nil {
		return db.MerchantPaymentIntent{}, err
	}
	if err = service.store.LinkMerchantIntentPayment(ctx, intent.ID, created.Order.ID); err != nil {
		return db.MerchantPaymentIntent{}, err
	}
	order, err := service.payments.ConfirmPayment(ctx, input.CustomerID, created.Order.ID, false)
	if err != nil {
		if order.Status == payment.PaymentFailed {
			_ = service.store.SetMerchantIntentStatus(ctx, intent.ID, StatusFailed)
		}
		return db.MerchantPaymentIntent{}, err
	}
	status := StatusFailed
	if order.Status == payment.PaymentBooked {
		status = StatusBooked
	}
	if err = service.store.SetMerchantIntentStatus(ctx, intent.ID, status); err != nil {
		return db.MerchantPaymentIntent{}, err
	}
	return service.GetIntentForCustomer(ctx, input.CustomerID, intent.ID)
}

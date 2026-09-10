package payment

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	sepa "github.com/mustafa-oezdemir/banking_go/internal/account"
	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	platformoutbox "github.com/mustafa-oezdemir/banking_go/internal/platform/outbox"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// OutboxWriter is Payment's narrow transaction-bound event port.
type OutboxWriter interface {
	Enqueue(context.Context, sqlc.DBTX, notification.EventEnvelope) error
}

func defaultOutboxWriter() OutboxWriter { return platformoutbox.Writer{} }

// SetOutboxWriter replaces the transaction-bound writer in focused tests.
func (service *Service) SetOutboxWriter(writer OutboxWriter) {
	if writer != nil {
		service.outbox = writer
	}
}

func (service *Service) enqueueBookedEvents(
	ctx context.Context,
	queries *sqlc.Queries,
	executor sqlc.DBTX,
	order sqlc.PaymentOrder,
) error {
	correlationID := "payment-" + order.ID.String()
	if err := service.enqueueActivityEvent(ctx, queries, executor, order, order.OwnerID, order.SourceAccountID,
		notification.EventTypePaymentBooked, "SEPA_PAYMENT_SENT", "DEBIT", order.BeneficiaryName, correlationID); err != nil {
		return err
	}
	if !order.BeneficiaryAccountID.Valid {
		return nil
	}
	destination, err := queries.GetAccount(ctx, order.BeneficiaryAccountID.UUID)
	if err != nil || destination.IsSystem || !destination.OwnerID.Valid {
		return err
	}
	return service.enqueueActivityEvent(ctx, queries, executor, order, destination.OwnerID.UUID, destination.ID,
		notification.EventTypePaymentBooked, "SEPA_PAYMENT_RECEIVED", "CREDIT", "", correlationID)
}

func (service *Service) enqueueFailedEvent(
	ctx context.Context,
	queries *sqlc.Queries,
	executor sqlc.DBTX,
	order sqlc.PaymentOrder,
) error {
	return service.enqueueActivityEvent(ctx, queries, executor, order, order.OwnerID, order.SourceAccountID,
		notification.EventTypePaymentFailed, "SEPA_PAYMENT_FAILED", "DEBIT", order.BeneficiaryName,
		"payment-"+order.ID.String())
}

func (service *Service) enqueueActivityEvent(
	ctx context.Context,
	queries *sqlc.Queries,
	executor sqlc.DBTX,
	order sqlc.PaymentOrder,
	ownerID, accountID uuid.UUID,
	eventType, kind, direction, counterparty, correlationID string,
) error {
	if service.outbox == nil {
		return errors.New("payment outbox writer is not configured")
	}
	user, err := queries.GetUserByID(ctx, ownerID)
	if err != nil {
		return fmt.Errorf("load notification recipient: %w", err)
	}
	account, err := queries.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("load notification account: %w", err)
	}
	if account.IsSystem || !account.OwnerID.Valid || account.OwnerID.UUID != ownerID {
		return errors.New("payment notification account ownership check failed")
	}
	event, err := notification.NewPaymentEvent(eventType, order.ID, correlationID, notification.ActivityCommand{
		RecipientEmail: user.Email, RecipientName: user.FullName, AccountName: account.Name,
		MaskedIBAN: sepa.MaskIBAN(account.Iban), Balance: account.Balance, Kind: kind,
		Direction: direction, Amount: order.Amount, Currency: "EUR", Counterparty: counterparty,
		Reference: order.Purpose.String,
	})
	if err != nil {
		return fmt.Errorf("create payment notification event: %w", err)
	}
	if err = service.outbox.Enqueue(ctx, executor, event); err != nil {
		return fmt.Errorf("enqueue payment notification event: %w", err)
	}
	return nil
}

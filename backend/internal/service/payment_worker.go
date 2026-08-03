package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// RunDuePayments recovers stale claims, materializes due standing-order
// occurrences, claims scheduled payments with SKIP LOCKED, and books each once.
func (s *PaymentService) RunDuePayments(ctx context.Context, batchSize int32) (int, error) {
	if batchSize <= 0 || batchSize > 100 {
		batchSize = 25
	}
	if _, err := s.store.RecoverStalePayments(ctx, 300); err != nil {
		return 0, fmt.Errorf("recover stale payments: %w", err)
	}
	if err := s.materializeStandingOrders(ctx, batchSize); err != nil {
		return 0, err
	}

	var claimed []sqlc.PaymentOrder
	if err := s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		var err error
		claimed, err = q.ClaimDuePaymentOrders(ctx, batchSize)
		return err
	}); err != nil {
		return 0, err
	}

	processed := 0
	for _, order := range claimed {
		if err := s.processClaimedPayment(ctx, order.ID); err != nil {
			// Failure state is persisted by processClaimedPayment; keep processing
			// the rest of the batch.
			continue
		}
		processed++
	}
	return processed, nil
}

func (s *PaymentService) processClaimedPayment(ctx context.Context, paymentID uuid.UUID) error {
	var ownerID uuid.UUID
	var businessErr error
	var bookedOrder sqlc.PaymentOrder
	err := s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		order, err := q.GetPaymentOrderForUpdate(ctx, paymentID)
		if err != nil {
			return err
		}
		ownerID = order.OwnerID
		if order.Status != PaymentProcessing || order.LedgerTransactionID.Valid {
			return ErrInvalidPaymentState
		}
		booked, err := s.bookPaymentTx(ctx, q, order)
		if err != nil {
			if _, failErr := markFailed(ctx, q, order, err); failErr != nil {
				return failErr
			}
			businessErr = err
			return s.auditWithQueries(ctx, q, order.OwnerID, order.ID, "PAYMENT_FAILED", map[string]any{"reason": publicFailureReason(err)})
		}
		bookedOrder = booked
		return s.auditWithQueries(ctx, q, order.OwnerID, order.ID, "PAYMENT_BOOKED", map[string]any{"ledger_transaction_id": booked.LedgerTransactionID.UUID})
	})
	if ownerID != uuid.Nil {
		s.hub.Publish(ownerID)
	}
	if err != nil {
		return err
	}
	if businessErr == nil && bookedOrder.ID != uuid.Nil {
		s.notifyBookedPayment(ctx, bookedOrder)
	}
	return businessErr
}

func (s *PaymentService) materializeStandingOrders(ctx context.Context, batchSize int32) error {
	return s.store.ExecTx(ctx, func(q *sqlc.Queries) error {
		orders, err := q.ClaimDueStandingOrders(ctx, batchSize)
		if err != nil {
			return err
		}
		for _, standing := range orders {
			kind := "SEPA"
			vop := VoPOther
			if standing.BeneficiaryAccountID.Valid {
				kind = "INTERNAL"
				vop = VoPMatch
			} else if standing.TransferType == PaymentInstant {
				kind = "SEPA_INSTANT"
			}
			key := fmt.Sprintf("standing:%s:%s", standing.ID, standing.NextExecutionAt.UTC().Format(time.RFC3339))
			_, err = q.CreatePaymentOrder(ctx, sqlc.CreatePaymentOrderParams{
				OwnerID: standing.OwnerID, SourceAccountID: standing.SourceAccountID,
				BeneficiaryAccountID: standing.BeneficiaryAccountID,
				StandingOrderID:      uuid.NullUUID{UUID: standing.ID, Valid: true},
				BeneficiaryName:      standing.BeneficiaryName, BeneficiaryIban: standing.BeneficiaryIban,
				BeneficiaryBic: standing.BeneficiaryBic, Amount: standing.Amount,
				PaymentKind: kind, ScheduleType: "STANDING", Purpose: standing.Purpose,
				CreditorReference: standing.CreditorReference,
				EndToEndID:        "DEMO-" + stringsNoHyphen(uuid.NewString())[:27], IdempotencyKey: key,
				RequestedExecutionAt: standing.NextExecutionAt, VopResult: vop,
				Status: PaymentScheduled,
			})
			if err != nil {
				return err
			}
			next := nextStandingExecution(standing.NextExecutionAt, standing.Frequency)
			status := "ACTIVE"
			if shouldCompleteStanding(standing, next) {
				status = "COMPLETED"
			}
			if _, err = q.AdvanceStandingOrder(ctx, sqlc.AdvanceStandingOrderParams{
				NextExecutionAt: next, Status: status, StandingOrderID: standing.ID,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func stringsNoHyphen(value string) string {
	result := make([]byte, 0, len(value))
	for i := range len(value) {
		if value[i] != '-' {
			result = append(result, value[i])
		}
	}
	return string(result)
}

var _ = sql.ErrNoRows

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
	"github.com/mustafa-oezdemir/banking_go/internal/service"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// VerifyPayee runs the simulation-only Verification of Payee check.
func (h *Handler) VerifyPayee(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input struct {
		Name string `json:"name"`
		IBAN string `json:"iban"`
	}
	if err = decodeStrictJSON(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	result, err := h.payments.VerifyPayee(r.Context(), ownerID, input.Name, input.IBAN)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid IBAN")
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// CreatePayment creates an idempotent payment draft awaiting explicit consent.
func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		respondError(w, http.StatusBadRequest, "Idempotency-Key header is required")
		return
	}
	var input struct {
		SourceAccountID    string `json:"source_account_id"`
		BeneficiaryName    string `json:"beneficiary_name"`
		BeneficiaryIBAN    string `json:"beneficiary_iban"`
		BeneficiaryBIC     string `json:"beneficiary_bic"`
		Amount             string `json:"amount"`
		TransferType       string `json:"transfer_type"`
		ScheduleType       string `json:"schedule_type"`
		Purpose            string `json:"purpose"`
		CreditorReference  string `json:"creditor_reference"`
		RequestedExecution string `json:"requested_execution_at"`
	}
	if err = decodeStrictJSON(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input; money values must be JSON strings")
		return
	}
	sourceID, err := uuid.Parse(input.SourceAccountID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid source_account_id")
		return
	}
	scheduleType := strings.ToUpper(strings.TrimSpace(input.ScheduleType))
	if scheduleType == "" {
		scheduleType = service.ScheduleImmediate
	}
	transferType := strings.ToUpper(strings.TrimSpace(input.TransferType))
	if transferType == "" {
		transferType = service.PaymentStandard
	}
	execution := time.Now().UTC()
	if scheduleType == service.ScheduleScheduled {
		execution, err = time.Parse(time.RFC3339, input.RequestedExecution)
		if err != nil {
			respondError(w, http.StatusBadRequest, "requested_execution_at must be RFC3339")
			return
		}
	}
	result, err := h.payments.CreatePayment(r.Context(), service.CreatePaymentInput{
		OwnerID: ownerID, SourceAccountID: sourceID, BeneficiaryName: input.BeneficiaryName,
		BeneficiaryIBAN: input.BeneficiaryIBAN, BeneficiaryBIC: input.BeneficiaryBIC,
		Amount: input.Amount, TransferType: transferType, ScheduleType: scheduleType,
		Purpose: input.Purpose, CreditorReference: input.CreditorReference,
		RequestedExecution: execution, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		respondPaymentError(w, err)
		return
	}
	if result.Replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	respondJSON(w, http.StatusCreated, toPaymentResponse(result.Order, true))
}

func (h *Handler) ListPayments(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	limit, offset := pagination(r)
	orders, err := h.payments.ListPayments(r.Context(), ownerID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list payments")
		return
	}
	response := make([]PaymentResponse, len(orders))
	for i, order := range orders {
		response[i] = toPaymentResponse(order, false)
	}
	respondJSON(w, http.StatusOK, response)
}

func (h *Handler) GetPayment(w http.ResponseWriter, r *http.Request) {
	ownerID, paymentID, ok := ownerAndPathID(w, r)
	if !ok {
		return
	}
	order, err := h.payments.GetPayment(r.Context(), ownerID, paymentID)
	if err != nil {
		respondPaymentError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, toPaymentResponse(order, true))
}

func (h *Handler) ConfirmPayment(w http.ResponseWriter, r *http.Request) {
	ownerID, paymentID, ok := ownerAndPathID(w, r)
	if !ok {
		return
	}
	var input struct {
		AcceptVoPMismatch bool `json:"accept_vop_mismatch"`
		ConfirmDemo       bool `json:"confirm_demo"`
	}
	if err := decodeStrictJSON(r, &input); err != nil || !input.ConfirmDemo {
		respondError(w, http.StatusBadRequest, "explicit demo confirmation is required")
		return
	}
	order, err := h.payments.ConfirmPayment(r.Context(), ownerID, paymentID, input.AcceptVoPMismatch)
	if err != nil {
		if order.ID != uuid.Nil && order.Status == service.PaymentFailed {
			respondJSON(w, http.StatusUnprocessableEntity, toPaymentResponse(order, true))
			return
		}
		respondPaymentError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, toPaymentResponse(order, true))
}

func (h *Handler) CancelPayment(w http.ResponseWriter, r *http.Request) {
	ownerID, paymentID, ok := ownerAndPathID(w, r)
	if !ok {
		return
	}
	order, err := h.payments.CancelPayment(r.Context(), ownerID, paymentID)
	if err != nil {
		respondPaymentError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, toPaymentResponse(order, true))
}

func (h *Handler) CreateStandingOrder(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input StandingOrderRequest
	if err = decodeStrictJSON(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	sourceID, err := uuid.Parse(input.SourceAccountID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid source_account_id")
		return
	}
	start, err := parseDate(input.StartDate)
	if err != nil {
		respondError(w, http.StatusBadRequest, "start_date must be YYYY-MM-DD")
		return
	}
	end, err := parseOptionalDate(input.EndDate)
	if err != nil {
		respondError(w, http.StatusBadRequest, "end_date must be YYYY-MM-DD")
		return
	}
	order, err := h.payments.CreateStandingOrder(r.Context(), service.CreateStandingOrderInput{
		OwnerID: ownerID, SourceAccountID: sourceID, BeneficiaryName: input.BeneficiaryName,
		BeneficiaryIBAN: input.BeneficiaryIBAN, BeneficiaryBIC: input.BeneficiaryBIC,
		Amount: input.Amount, Purpose: input.Purpose, CreditorReference: input.CreditorReference,
		TransferType: input.TransferType, Frequency: input.Frequency,
		StartDate: start, EndDate: end, MaxOccurrences: input.MaxOccurrences,
	})
	if err != nil {
		respondPaymentError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, toStandingOrderResponse(order))
}

func (h *Handler) ListStandingOrders(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	orders, err := h.store.ListStandingOrdersByOwner(r.Context(), ownerID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list standing orders")
		return
	}
	response := make([]StandingOrderResponse, len(orders))
	for i, order := range orders {
		response[i] = toStandingOrderResponse(order)
	}
	respondJSON(w, http.StatusOK, response)
}

func (h *Handler) UpdateStandingOrder(w http.ResponseWriter, r *http.Request) {
	ownerID, orderID, ok := ownerAndPathID(w, r)
	if !ok {
		return
	}
	var input struct {
		Amount         string `json:"amount"`
		Purpose        string `json:"purpose"`
		Status         string `json:"status"`
		EndDate        string `json:"end_date"`
		MaxOccurrences *int32 `json:"max_occurrences"`
	}
	if err := decodeStrictJSON(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	end, err := parseOptionalDate(input.EndDate)
	if err != nil {
		respondError(w, http.StatusBadRequest, "end_date must be YYYY-MM-DD")
		return
	}
	order, err := h.payments.UpdateStandingOrder(r.Context(), ownerID, orderID, input.Amount, input.Purpose, input.Status, end, input.MaxOccurrences)
	if err != nil {
		respondPaymentError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, toStandingOrderResponse(order))
}

func (h *Handler) DeleteStandingOrder(w http.ResponseWriter, r *http.Request) {
	ownerID, orderID, ok := ownerAndPathID(w, r)
	if !ok {
		return
	}
	order, err := h.payments.CancelStandingOrder(r.Context(), ownerID, orderID)
	if err != nil {
		respondPaymentError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, toStandingOrderResponse(order))
}

func (h *Handler) ListAccountTransactions(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid account ID")
		return
	}
	account, err := h.store.GetAccount(r.Context(), accountID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil || !account.OwnerID.Valid || account.OwnerID.UUID != ownerID || account.IsSystem {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}
	limit, offset := pagination(r)
	params := sqlc.ListAccountTransactionsParams{
		AccountID: accountID, Direction: queryNullString(r, "direction"), Status: queryNullString(r, "status"),
		Category: queryNullString(r, "category"), MinAmount: queryNullString(r, "min_amount"),
		MaxAmount: queryNullString(r, "max_amount"), ResultLimit: limit, ResultOffset: offset,
	}
	if params.DateFrom, err = queryNullTime(r, "date_from"); err != nil {
		respondError(w, http.StatusBadRequest, "invalid date_from")
		return
	}
	if params.DateTo, err = queryNullTime(r, "date_to"); err != nil {
		respondError(w, http.StatusBadRequest, "invalid date_to")
		return
	}
	entries, err := h.store.ListAccountTransactions(r.Context(), params)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list transactions")
		return
	}
	response := make([]EntryResponse, len(entries))
	for i, entry := range entries {
		response[i] = toEntryResponse(entry)
	}
	respondJSON(w, http.StatusOK, response)
}

func (h *Handler) ListBeneficiaries(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	items, err := h.store.ListBeneficiaries(r.Context(), ownerID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list beneficiaries")
		return
	}
	for i := range items {
		items[i].Iban = sepa.MaskIBAN(items[i].Iban)
	}
	respondJSON(w, http.StatusOK, items)
}

func (h *Handler) CreateBeneficiary(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input struct {
		Name     string `json:"name"`
		IBAN     string `json:"iban"`
		BIC      string `json:"bic"`
		Category string `json:"category"`
	}
	if err = decodeStrictJSON(r, &input); err != nil || strings.TrimSpace(input.Name) == "" {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	iban := sepa.NormalizeIBAN(input.IBAN)
	if err = sepa.ValidateIBAN(iban); err != nil {
		respondError(w, http.StatusBadRequest, "invalid IBAN")
		return
	}
	item, err := h.store.CreateBeneficiary(r.Context(), sqlc.CreateBeneficiaryParams{
		OwnerID: ownerID, Name: strings.TrimSpace(input.Name), Iban: iban,
		Bic:      sql.NullString{String: strings.TrimSpace(input.BIC), Valid: strings.TrimSpace(input.BIC) != ""},
		Category: sql.NullString{String: strings.TrimSpace(input.Category), Valid: strings.TrimSpace(input.Category) != ""},
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save beneficiary")
		return
	}
	respondJSON(w, http.StatusCreated, item)
}

// Events streams durable audit events and works with EventSource cookies.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	afterID, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	updates, unsubscribe := h.payments.EventHub().Subscribe(ownerID)
	defer unsubscribe()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	writeEvents := func() error {
		events, listErr := h.store.ListAuditEventsAfter(r.Context(), sqlc.ListAuditEventsAfterParams{
			OwnerID: uuid.NullUUID{UUID: ownerID, Valid: true}, AfterID: afterID, ResultLimit: 100,
		})
		if listErr != nil {
			return listErr
		}
		for _, event := range events {
			payload, marshalErr := json.Marshal(event)
			if marshalErr != nil {
				return marshalErr
			}
			if _, writeErr := fmt.Fprintf(w, "id: %d\nevent: payment\ndata: %s\n\n", event.ID, payload); writeErr != nil {
				return writeErr
			}
			afterID = event.ID
		}
		flusher.Flush()
		return nil
	}
	if err = writeEvents(); err != nil {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			if writeEvents() != nil {
				return
			}
		case <-ticker.C:
			if _, err = fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func decodeStrictJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func ownerAndPathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid ID")
		return uuid.Nil, uuid.Nil, false
	}
	return ownerID, id, true
}

func respondPaymentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrPaymentNotFound), errors.Is(err, service.ErrAccountNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrPaymentUnauthorized):
		respondError(w, http.StatusForbidden, "access denied")
	case errors.Is(err, service.ErrIdempotencyConflict), errors.Is(err, service.ErrInvalidPaymentState):
		respondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrVoPOverrideRequired), errors.Is(err, service.ErrInvalidPaymentInput),
		errors.Is(err, service.ErrInvalidAmount), errors.Is(err, service.ErrStandingOrderInvalid),
		errors.Is(err, service.ErrSameAccountTransfer), errors.Is(err, sepa.ErrInvalidIBAN):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		respondError(w, http.StatusInternalServerError, "payment processing failed")
	}
}

func pagination(r *http.Request) (int32, int32) {
	limitValue, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offsetValue, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limitValue <= 0 || limitValue > 100 {
		limitValue = 50
	}
	if offsetValue < 0 {
		offsetValue = 0
	}
	return int32(limitValue), int32(offsetValue)
}

func queryNullString(r *http.Request, key string) sql.NullString {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	return sql.NullString{String: value, Valid: value != ""}
}

func queryNullTime(r *http.Request, key string) (sql.NullTime, error) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return sql.NullTime{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	return sql.NullTime{Time: parsed, Valid: err == nil}, err
}

func parseDate(value string) (time.Time, error) {
	return time.Parse("2006-01-02", value)
}

func parseOptionalDate(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := parseDate(value)
	return &parsed, err
}

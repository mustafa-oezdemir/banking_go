package api

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/merchant"
	"github.com/mustafa-oezdemir/banking_go/internal/payment"
	db "github.com/mustafa-oezdemir/banking_go/internal/platform/database"
)

// MerchantPaymentIntentResponse deliberately exposes the persisted checkout
// values, never browser-submitted recipient or pricing values.
type MerchantPaymentIntentResponse struct {
	ID                string  `json:"payment_intent_id"`
	MerchantID        string  `json:"merchant_id"`
	MerchantName      string  `json:"merchant_name"`
	BeneficiaryIBAN   string  `json:"beneficiary_iban"`
	MerchantReference string  `json:"merchant_reference"`
	Amount            string  `json:"amount"`
	Currency          string  `json:"currency"`
	Status            string  `json:"status"`
	PaymentID         *string `json:"payment_id,omitempty"`
	ReturnURL         string  `json:"return_url"`
	ExpiresAt         string  `json:"expires_at"`
}

func toMerchantIntentResponse(intent merchantIntent) MerchantPaymentIntentResponse {
	response := MerchantPaymentIntentResponse{
		ID: intent.ID.String(), MerchantID: intent.MerchantID, MerchantName: intent.MerchantName,
		BeneficiaryIBAN: intent.BeneficiaryIBAN, MerchantReference: intent.MerchantReference,
		Amount: intent.Amount.StringFixed(2), Currency: intent.Currency, Status: intent.Status,
		ReturnURL: intent.ReturnURL, ExpiresAt: intent.ExpiresAt.UTC().Format(time.RFC3339),
	}
	if intent.PaymentOrderID.Valid {
		value := intent.PaymentOrderID.UUID.String()
		response.PaymentID = &value
	}
	return response
}

// merchantIntent keeps the database record shape private to the HTTP adapter.
type merchantIntent = db.MerchantPaymentIntent

// CreateMerchantPaymentIntent is the server-to-server checkout entry point.
func (h *Handler) CreateMerchantPaymentIntent(w http.ResponseWriter, r *http.Request) {
	if !validMerchantAPIToken(r) {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input struct {
		MerchantID     string `json:"merchant_id"`
		OrderReference string `json:"order_reference"`
		Amount         string `json:"amount"`
		Currency       string `json:"currency"`
	}
	if err := decodeStrictJSON(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input; amount must be a JSON string")
		return
	}
	intent, replayed, err := h.merchants.CreateIntent(r.Context(), merchant.CreateIntentInput{
		MerchantID: input.MerchantID, MerchantReference: input.OrderReference, Amount: input.Amount, Currency: input.Currency,
	})
	if err != nil {
		respondMerchantError(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	response := toMerchantIntentResponse(intent)
	respondJSON(w, http.StatusCreated, struct {
		MerchantPaymentIntentResponse
		ApprovalURL string `json:"approval_url"`
	}{response, merchantApprovalURL(r, intent.ID)})
}

// GetMerchantPaymentIntent returns an intent to a logged-in customer.
func (h *Handler) GetMerchantPaymentIntent(w http.ResponseWriter, r *http.Request) {
	customerID, intentID, ok := merchantCustomerAndIntentID(w, r)
	if !ok {
		return
	}
	intent, err := h.merchants.GetIntentForCustomer(r.Context(), customerID, intentID)
	if err != nil {
		respondMerchantError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, toMerchantIntentResponse(intent))
}

// ApproveMerchantPaymentIntent lets a customer choose a source account and
// consent; every destination and amount field remains database-derived.
func (h *Handler) ApproveMerchantPaymentIntent(w http.ResponseWriter, r *http.Request) {
	customerID, intentID, ok := merchantCustomerAndIntentID(w, r)
	if !ok {
		return
	}
	var input struct {
		SourceAccountID string `json:"source_account_id"`
		ConfirmDemo     bool   `json:"confirm_demo"`
	}
	if err := decodeStrictJSON(r, &input); err != nil || !input.ConfirmDemo {
		respondError(w, http.StatusBadRequest, "explicit demo confirmation is required")
		return
	}
	sourceAccountID, err := uuid.Parse(strings.TrimSpace(input.SourceAccountID))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid source_account_id")
		return
	}
	intent, err := h.merchants.Approve(r.Context(), merchant.ApprovalInput{
		CustomerID: customerID, SourceAccountID: sourceAccountID, IntentID: intentID,
	})
	if err != nil {
		respondMerchantError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, toMerchantIntentResponse(intent))
}

func merchantCustomerAndIntentID(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	customerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return uuid.Nil, uuid.Nil, false
	}
	intentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid payment intent id")
		return uuid.Nil, uuid.Nil, false
	}
	return customerID, intentID, true
}

func validMerchantAPIToken(r *http.Request) bool {
	expected := strings.TrimSpace(os.Getenv("MERCHANT_API_TOKEN"))
	provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return len(expected) >= 32 && len(provided) == len(expected) &&
		subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func merchantApprovalURL(_ *http.Request, intentID uuid.UUID) string {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("BANKING_PUBLIC_URL")), "/")
	if baseURL == "" {
		baseURL = "http://localhost:3003"
	}
	return baseURL + "/pay/" + intentID.String()
}

func respondMerchantError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, merchant.ErrIntentNotFound):
		respondError(w, http.StatusNotFound, "merchant payment intent not found")
	case errors.Is(err, merchant.ErrIntentExpired):
		respondError(w, http.StatusGone, "merchant payment intent has expired")
	case errors.Is(err, merchant.ErrIntentState), errors.Is(err, payment.ErrIdempotencyConflict):
		respondError(w, http.StatusConflict, "merchant payment intent cannot be approved")
	case errors.Is(err, merchant.ErrMerchantInput), errors.Is(err, payment.ErrInvalidPaymentInput):
		respondError(w, http.StatusBadRequest, "invalid merchant payment intent")
	default:
		respondPaymentError(w, err)
	}
}

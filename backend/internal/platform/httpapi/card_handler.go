package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/card"
	"github.com/mustafa-oezdemir/banking_go/internal/merchant"
)

func (h *Handler) IssueCard(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input struct {
		AccountID string `json:"account_id"`
	}
	if decodeStrictJSON(r, &input) != nil {
		respondError(w, http.StatusBadRequest, "invalid card request")
		return
	}
	accountID, err := uuid.Parse(strings.TrimSpace(input.AccountID))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid account_id")
		return
	}
	issued, err := h.cards.Issue(r.Context(), ownerID, accountID)
	if err != nil {
		respondCardError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, http.StatusCreated, issued)
}

func (h *Handler) ListCards(w http.ResponseWriter, r *http.Request) {
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	cards, err := h.cards.List(r.Context(), ownerID)
	if err != nil {
		respondCardError(w, err)
		return
	}
	type responseCard struct {
		ID        string `json:"id"`
		AccountID string `json:"account_id"`
		Brand     string `json:"brand"`
		Last4     string `json:"last4"`
		ExpMonth  int    `json:"exp_month"`
		ExpYear   int    `json:"exp_year"`
		Status    string `json:"status"`
	}
	result := make([]responseCard, 0, len(cards))
	for _, item := range cards {
		result = append(result, responseCard{item.ID.String(), item.AccountID.String(), item.Brand, item.LastFour, item.ExpMonth, item.ExpYear, item.Status})
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, http.StatusOK, result)
}

// GetCardCredentials reveals a versioned demo card only to its authenticated owner.
func (h *Handler) GetCardCredentials(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	ownerID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	cardID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusNotFound, "card credentials are unavailable")
		return
	}
	revealed, err := h.cards.Reveal(r.Context(), ownerID, cardID)
	if err != nil {
		respondCardError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, revealed)
}

func (h *Handler) CancelCard(w http.ResponseWriter, r *http.Request) {
	ownerID, cardID, ok := ownerAndPathID(w, r)
	if !ok {
		return
	}
	if err := h.cards.Cancel(r.Context(), ownerID, cardID); err != nil {
		respondCardError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	ownerID, cardID, ok := ownerAndPathID(w, r)
	if !ok {
		return
	}
	if err := h.cards.Delete(r.Context(), ownerID, cardID); err != nil {
		respondCardError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) TokenizeMerchantCard(w http.ResponseWriter, r *http.Request) {
	if !validMerchantAPIToken(r) {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input struct {
		MerchantID string `json:"merchant_id"`
		CardNumber string `json:"card_number"`
		ExpMonth   int    `json:"exp_month"`
		ExpYear    int    `json:"exp_year"`
		CVC        string `json:"cvc"`
	}
	if decodeStrictJSON(r, &input) != nil {
		respondError(w, http.StatusBadRequest, "invalid card credentials")
		return
	}
	tokenized, err := h.cards.Tokenize(r.Context(), input.MerchantID, input.CardNumber, input.CVC, input.ExpMonth, input.ExpYear)
	if err != nil {
		respondCardError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, http.StatusCreated, tokenized)
}

func (h *Handler) ApproveMerchantCardPayment(w http.ResponseWriter, r *http.Request) {
	if !validMerchantAPIToken(r) {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	intentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid payment intent id")
		return
	}
	var input struct {
		MerchantID string `json:"merchant_id"`
		CardToken  string `json:"card_token"`
		CVC        string `json:"cvc"`
	}
	if decodeStrictJSON(r, &input) != nil {
		respondError(w, http.StatusBadRequest, "invalid card credentials")
		return
	}
	authorized, err := h.cards.Authorize(r.Context(), input.MerchantID, input.CardToken, input.CVC)
	if err != nil {
		respondCardError(w, err)
		return
	}
	intent, err := h.merchants.Approve(r.Context(), merchant.ApprovalInput{CustomerID: authorized.OwnerID, SourceAccountID: authorized.AccountID, IntentID: intentID})
	if err != nil {
		respondMerchantError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, http.StatusOK, struct {
		MerchantPaymentIntentResponse
		Brand string `json:"brand"`
		Last4 string `json:"last4"`
	}{toMerchantIntentResponse(intent), authorized.Brand, authorized.Last4})
}

func respondCardError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, card.ErrInvalidCard):
		respondError(w, http.StatusUnauthorized, "invalid card credentials")
	case errors.Is(err, card.ErrCardUnavailable):
		respondError(w, http.StatusConflict, "card is unavailable")
	case errors.Is(err, card.ErrCardCredentialsUnavailable):
		respondError(w, http.StatusNotFound, "card credentials are unavailable")
	default:
		respondError(w, http.StatusInternalServerError, "card operation failed")
	}
}

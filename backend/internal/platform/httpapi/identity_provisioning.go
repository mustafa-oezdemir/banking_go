package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/ledger"
)

// ProvisionCustomerInternal receives the only Identity-to-Banking command. It
// is not browser-routable and remains idempotent for retry after a lost reply.
func (h *Handler) ProvisionCustomerInternal(w http.ResponseWriter, r *http.Request) {
	if !validInternalServiceToken(r) {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input struct {
		IdentityID string `json:"identity_id"`
		Email      string `json:"email"`
		FullName   string `json:"full_name"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	identifier, err := uuid.Parse(strings.TrimSpace(input.IdentityID))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid identity id")
		return
	}
	if _, err = h.store.GetUserByID(r.Context(), identifier); err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, err = h.ledger.CreateFundedCustomer(r.Context(), ledger.NewCustomer{IdentityID: identifier, Email: strings.TrimSpace(input.Email), FullName: defaultFullName(input.FullName, input.Email), HashedPassword: "identity-managed"})
	if err != nil {
		respondError(w, http.StatusConflict, "customer provisioning failed")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// RevokeCustomerSessionsInternal applies an Identity-owned credential change
// to Banking's local session generation without exposing Banking persistence.
func (h *Handler) RevokeCustomerSessionsInternal(w http.ResponseWriter, r *http.Request) {
	if !validInternalServiceToken(r) {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "id")))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid customer id")
		return
	}
	if err = h.store.RevokeUserSessions(r.Context(), userID); err != nil {
		respondError(w, http.StatusNotFound, "customer not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validInternalServiceToken(r *http.Request) bool {
	expected := strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN"))
	provided := strings.TrimSpace(r.Header.Get("X-Internal-Service-Token"))
	return len(expected) >= 32 && len(provided) == len(expected) && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

package api

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/identity"
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
	if err := decodeStrictJSON(r, &input); err != nil {
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
	email, err := identity.NormalizeEmail(input.Email)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid email")
		return
	}
	fullName := defaultFullName(input.FullName, email)
	if fullName, err = identity.NormalizeFullName(fullName); err != nil {
		respondError(w, http.StatusBadRequest, "invalid full name")
		return
	}
	_, err = h.ledger.CreateFundedCustomer(r.Context(), ledger.NewCustomer{IdentityID: identifier, Email: email, FullName: fullName, HashedPassword: "identity-managed"})
	if err != nil {
		respondError(w, http.StatusConflict, "customer provisioning failed")
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// SyncCustomerSessionVersionInternal applies the authoritative Identity session
// generation idempotently, preventing retry-induced version drift.
func (h *Handler) SyncCustomerSessionVersionInternal(w http.ResponseWriter, r *http.Request) {
	if !validInternalServiceToken(r) {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	userID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "id")))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid customer id")
		return
	}
	var input struct {
		SessionVersion int64 `json:"session_version"`
	}
	if err = decodeStrictJSON(r, &input); err != nil || input.SessionVersion < 0 {
		respondError(w, http.StatusBadRequest, "invalid session version")
		return
	}
	if err = h.store.SetUserSessionVersion(r.Context(), userID, input.SessionVersion); err != nil {
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

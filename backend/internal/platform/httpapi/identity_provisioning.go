package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/ledger"
)

// ProvisionCustomerInternal receives the only Identity-to-Banking command. It
// is not browser-routable and remains idempotent for retry after a lost reply.
func (h *Handler) ProvisionCustomerInternal(w http.ResponseWriter, r *http.Request) {
	expected := strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN"))
	provided := strings.TrimSpace(r.Header.Get("X-Internal-Service-Token"))
	if len(expected) < 32 || len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
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

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
)

type adminUserResponse struct {
	CreatedAt    time.Time `json:"created_at"`
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	FullName     string    `json:"full_name"`
	Role         string    `json:"role"`
	TotalBalance string    `json:"total_balance"`
	AccountCount int64     `json:"account_count"`
}

type adminAccountResponse struct {
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	ID               string    `json:"id"`
	OwnerID          string    `json:"owner_id"`
	OwnerEmail       string    `json:"owner_email"`
	OwnerName        string    `json:"owner_name"`
	Name             string    `json:"name"`
	MaskedIBAN       string    `json:"masked_iban"`
	AccountType      string    `json:"account_type"`
	Status           string    `json:"status"`
	Balance          string    `json:"balance"`
	AvailableBalance string    `json:"available_balance"`
}

type adminOverviewResponse struct {
	Users        []adminUserResponse    `json:"users"`
	Accounts     []adminAccountResponse `json:"accounts"`
	PaymentCount int64                  `json:"payment_count"`
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return uuid.Nil, false
	}
	role, err := h.store.GetUserRole(r.Context(), userID)
	if err != nil || role != "ADMIN" {
		respondError(w, http.StatusForbidden, "administrator access required")
		return uuid.Nil, false
	}
	return userID, true
}

// AdminOverview lists users and accounts for the management dashboard.
func (h *Handler) AdminOverview(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	users, err := h.store.ListAdminUsers(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	accounts, err := h.store.ListAdminAccounts(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list accounts")
		return
	}
	paymentCount, err := h.store.CountPaymentOrders(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count payments")
		return
	}

	userResponses := make([]adminUserResponse, 0, len(users))
	for _, user := range users {
		userResponses = append(userResponses, adminUserResponse{
			ID: user.ID.String(), Email: user.Email, FullName: user.FullName, Role: user.Role,
			CreatedAt: user.CreatedAt, AccountCount: user.AccountCount, TotalBalance: user.TotalBalance,
		})
	}
	accountResponses := make([]adminAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		accountResponses = append(accountResponses, adminAccountResponse{
			ID: account.ID.String(), OwnerID: account.OwnerID.String(), OwnerEmail: account.OwnerEmail,
			OwnerName: account.OwnerName, Name: account.Name, MaskedIBAN: sepa.MaskIBAN(account.IBAN),
			AccountType: account.AccountType, Status: account.Status, Balance: account.Balance,
			AvailableBalance: account.AvailableBalance, CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt,
		})
	}
	respondJSON(w, http.StatusOK, adminOverviewResponse{Users: userResponses, Accounts: accountResponses, PaymentCount: paymentCount})
}

// AdminUpdateUserRole promotes or demotes a user.
func (h *Handler) AdminUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	adminID, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var input struct {
		Role string `json:"role"`
	}
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	input.Role = strings.ToUpper(strings.TrimSpace(input.Role))
	if input.Role != "CUSTOMER" && input.Role != "ADMIN" {
		respondError(w, http.StatusBadRequest, "role must be CUSTOMER or ADMIN")
		return
	}
	if userID == adminID && input.Role != "ADMIN" {
		respondError(w, http.StatusBadRequest, "you cannot remove your own administrator role")
		return
	}
	if err = h.store.UpdateUserRole(r.Context(), adminID, userID, input.Role, chimiddleware.GetReqID(r.Context())); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		respondError(w, status, "failed to update user role")
		return
	}
	respondJSON(w, http.StatusOK, MessageResponse{Message: "User role updated"})
}

// AdminUpdateAccountStatus activates or blocks any customer account.
func (h *Handler) AdminUpdateAccountStatus(w http.ResponseWriter, r *http.Request) {
	adminID, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status != "ACTIVE" && input.Status != "BLOCKED" {
		respondError(w, http.StatusBadRequest, "status must be ACTIVE or BLOCKED")
		return
	}
	if err = h.store.UpdateAdminAccountStatus(r.Context(), adminID, accountID, input.Status, chimiddleware.GetReqID(r.Context())); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		respondError(w, status, "failed to update account status")
		return
	}
	respondJSON(w, http.StatusOK, MessageResponse{Message: "Account status updated"})
}

// AdminAdjustAccountBalance posts an audited double-entry deposit or withdrawal.
func (h *Handler) AdminAdjustAccountBalance(w http.ResponseWriter, r *http.Request) {
	adminID, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	var input struct {
		Operation string `json:"operation"`
		Amount    string `json:"amount"`
	}
	if err = json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(input.Amount))
	if err != nil || !amount.IsPositive() || amount.Exponent() < -2 {
		respondError(w, http.StatusBadRequest, "amount must be a positive EUR value with at most two decimals")
		return
	}
	operation := strings.ToUpper(strings.TrimSpace(input.Operation))
	switch operation {
	case "DEPOSIT":
	case "WITHDRAW":
	default:
		respondError(w, http.StatusBadRequest, "operation must be DEPOSIT or WITHDRAW")
		return
	}
	err = h.ledger.AdjustBalanceAsAdmin(
		r.Context(), adminID, accountID, operation, amount.StringFixed(2), chimiddleware.GetReqID(r.Context()),
	)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	account, lookupErr := h.store.GetAccount(r.Context(), accountID)
	if lookupErr == nil && account.OwnerID.Valid {
		direction := "DEBIT"
		if operation == "DEPOSIT" {
			direction = "CREDIT"
		}
		h.notifier.NotifyActivity(notification.Activity{
			UserID: account.OwnerID.UUID, AccountID: account.ID, Kind: "ADMIN_" + operation,
			Direction: direction, Amount: amount.StringFixed(2), Currency: account.Currency,
			Reference: "Administrative Kontobuchung",
		})
	}
	respondJSON(w, http.StatusOK, MessageResponse{Message: "Account balance adjusted"})
}

package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mustafa-oezdemir/banking_go/internal/identity"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

const (
	passwordResetTokenBytes = 32
	passwordResetLifetime   = 15 * time.Minute
	passwordResetJobTimeout = 20 * time.Second
	passwordResetBodyLimit  = 4 << 10
	signupOpeningBalance    = "500.00"
)

var passwordResetAccepted = MessageResponse{
	Message: "If the address is registered, a password reset email has been sent.",
}

// passwordResetStore contains precisely the persistence operations used by
// this security-sensitive flow. The raw reset token never reaches the store.
type passwordResetStore interface {
	GetUserByEmail(context.Context, string) (sqlc.User, error)
	CreatePasswordResetToken(context.Context, uuid.UUID, []byte, time.Time) error
	ResetPasswordWithToken(context.Context, []byte, string, time.Time) (uuid.UUID, error)
}

// passwordResetDispatcher makes the HTTP path independent of lookup, storage,
// provider availability, and the associated user-existence timing signal.
type passwordResetDispatcher func(func())

func dispatchPasswordReset(job func()) { go job() }

// ForgotPassword creates and emails a short-lived token without revealing
// whether the submitted address belongs to an account.
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, passwordResetBodyLimit+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		// Keep malformed requests indistinguishable from unknown accounts. The
		// route-level rate limit still protects this public endpoint.
		input.Email = ""
	}
	email, err := identity.NormalizeEmail(input.Email)
	if err != nil {
		email = ""
	}
	dispatch := h.dispatchReset
	if dispatch == nil {
		dispatch = dispatchPasswordReset
	}
	dispatch(func() {
		ctx, cancel := context.WithTimeout(context.Background(), passwordResetJobTimeout)
		defer cancel()
		h.processPasswordReset(ctx, email)
	})
	respondJSON(w, http.StatusAccepted, passwordResetAccepted)
}

// processPasswordReset does the expensive/sensitive work after the generic
// HTTP response. It always generates and hashes entropy before the lookup so a
// known and unknown normalized address follow comparable local work.
func (h *Handler) processPasswordReset(ctx context.Context, email string) {
	tokenBytes := make([]byte, passwordResetTokenBytes)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		// Do not attach errors here: provider/database error strings must never
		// be allowed to expose a raw token or recipient address through logs.
		log.Error().Msg("Password reset token generation failed")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	// Hash the canonical URL-safe token text. This preserves the ability to
	// consume a still-valid token issued immediately before a rolling deploy.
	tokenHash := sha256.Sum256([]byte(token))

	lookupEmail := email
	if lookupEmail == "" {
		lookupEmail = "invalid-password-reset@invalid.example"
	}
	user, err := h.passwordResets.GetUserByEmail(ctx, lookupEmail)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		log.Error().Msg("Password reset user lookup failed")
		return
	}
	if err = h.passwordResets.CreatePasswordResetToken(
		ctx, user.ID, tokenHash[:], time.Now().UTC().Add(passwordResetLifetime),
	); err != nil {
		log.Error().Msg("Password reset token persistence failed")
		return
	}
	mailCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	if err = h.notifier.SendPasswordReset(mailCtx, user.Email, user.FullName, token); err != nil {
		log.Error().Msg("Password reset email delivery failed")
	}
}

// ResetPassword validates and consumes a token before updating the password.
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	input.Token = strings.TrimSpace(input.Token)
	rawToken, err := base64.RawURLEncoding.DecodeString(input.Token)
	if err != nil || len(rawToken) != passwordResetTokenBytes {
		respondError(w, http.StatusBadRequest, "reset link is invalid or expired")
		return
	}
	if err = identity.ValidatePassword(input.NewPassword); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	hashed, err := identity.HashPassword(input.NewPassword)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	tokenHash := sha256.Sum256([]byte(input.Token))
	if _, err = h.passwordResets.ResetPasswordWithToken(r.Context(), tokenHash[:], hashed, time.Now().UTC()); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Msg("Password reset failed")
		}
		respondError(w, http.StatusBadRequest, "reset link is invalid or expired")
		return
	}
	ClearSessionCookie(w, r)
	respondJSON(w, http.StatusOK, MessageResponse{Message: "Password updated successfully."})
}

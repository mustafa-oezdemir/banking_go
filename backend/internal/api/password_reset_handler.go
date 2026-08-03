package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

const (
	passwordResetTokenBytes = 32
	passwordResetLifetime   = 15 * time.Minute
	signupOpeningBalance    = "500.00"
)

var passwordResetAccepted = MessageResponse{
	Message: "If the address is registered, a password reset email has been sent.",
}

// ForgotPassword creates and emails a short-lived token without revealing
// whether the submitted address belongs to an account.
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		respondJSON(w, http.StatusAccepted, passwordResetAccepted)
		return
	}
	user, err := h.store.GetUserByEmail(r.Context(), email)
	if errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusAccepted, passwordResetAccepted)
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("Password reset user lookup failed")
		respondJSON(w, http.StatusAccepted, passwordResetAccepted)
		return
	}
	tokenBytes := make([]byte, passwordResetTokenBytes)
	if _, err = rand.Read(tokenBytes); err != nil {
		log.Error().Err(err).Msg("Password reset token generation failed")
		respondJSON(w, http.StatusAccepted, passwordResetAccepted)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := sha256.Sum256([]byte(token))
	if err = h.store.CreatePasswordResetToken(
		r.Context(), user.ID, tokenHash[:], time.Now().UTC().Add(passwordResetLifetime),
	); err != nil {
		log.Error().Err(err).Msg("Password reset token persistence failed")
		respondJSON(w, http.StatusAccepted, passwordResetAccepted)
		return
	}
	mailCtx, cancel := contextWithEmailTimeout(r)
	defer cancel()
	if err = h.notifier.SendPasswordReset(mailCtx, user.Email, user.FullName, token); err != nil {
		log.Error().Err(err).Msg("Password reset email delivery failed")
	}
	respondJSON(w, http.StatusAccepted, passwordResetAccepted)
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
	if err = validateRegistrationPassword(input.NewPassword); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	tokenHash := sha256.Sum256([]byte(input.Token))
	if _, err = h.store.ResetPasswordWithToken(r.Context(), tokenHash[:], string(hashed), time.Now().UTC()); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Password reset failed")
		}
		respondError(w, http.StatusBadRequest, "reset link is invalid or expired")
		return
	}
	ClearSessionCookie(w, r)
	respondJSON(w, http.StatusOK, MessageResponse{Message: "Password updated successfully."})
}

func contextWithEmailTimeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 12*time.Second)
}

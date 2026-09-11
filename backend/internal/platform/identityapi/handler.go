// Package identityapi exposes the standalone Identity service HTTP contract.
package identityapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/rs/zerolog/log"

	"github.com/mustafa-oezdemir/banking_go/internal/identity"
)

const (
	issuer          = "pehlione-identity"
	audience        = "pehlione-banking-api"
	sessionCookie   = "jwt"
	sessionLifetime = 15 * time.Minute
	resetLifetime   = 15 * time.Minute
	resetJobTimeout = 20 * time.Second
)

// CustomerProvisioner creates the Banking Customer after a successful identity registration.
type CustomerProvisioner interface {
	ProvisionCustomer(context.Context, uuid.UUID, string, string) error
	SyncCustomerSessionVersion(context.Context, uuid.UUID, int64) error
}

// PasswordResetSender delivers an already-generated password-reset command.
type PasswordResetSender interface {
	SendPasswordReset(context.Context, string, string, string) error
}

// Store is the Identity persistence port used by HTTP workflows.
type Store interface {
	identity.AuthenticationRepository
	Register(context.Context, string, string, string) (uuid.UUID, error)
	Delete(context.Context, uuid.UUID) error
	FindLoginAccountByID(context.Context, uuid.UUID) (identity.LoginAccount, bool, error)
	ChangePassword(context.Context, uuid.UUID, string, string) (int64, bool, error)
	RevokeSessions(context.Context, uuid.UUID) (int64, error)
	GetByEmail(context.Context, string) (uuid.UUID, string, bool, error)
	CreatePasswordReset(context.Context, uuid.UUID, []byte, time.Time) error
	ResetPassword(context.Context, []byte, string, time.Time) (uuid.UUID, int64, error)
}

// Handler owns identity HTTP workflows and never queries Banking persistence.
type Handler struct {
	store       Store
	auth        *identity.AuthenticationService
	tokens      *jwtauth.JWTAuth
	provisioner CustomerProvisioner
	notifier    PasswordResetSender
	dispatch    func(func())
}

// New creates the identity HTTP handler.
func New(store Store, secret string, provisioner CustomerProvisioner, notifier PasswordResetSender) (*Handler, error) {
	if store == nil || provisioner == nil || notifier == nil {
		return nil, errors.New("identity dependencies are required")
	}
	if len(strings.TrimSpace(secret)) < 32 {
		return nil, errors.New("JWT_SECRET must be at least 32 characters")
	}
	tokens := jwtauth.New("HS256", []byte(secret), nil, jwt.WithIssuer(issuer), jwt.WithAudience(audience))
	return &Handler{store: store, auth: identity.NewAuthenticationService(store), tokens: tokens, provisioner: provisioner, notifier: notifier, dispatch: func(job func()) { go job() }}, nil
}

// Routes returns public Identity endpoints and its independent health check.
func (handler *Handler) Routes() http.Handler {
	router := chi.NewRouter()
	router.Use(limitRequestBody)
	router.Use(requireJSON)
	router.Use(noStore)
	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.With(newIPRateLimiter(5, time.Minute)).Post("/register", handler.register)
	router.With(newIPRateLimiter(10, time.Minute)).Post("/login", handler.login)
	router.With(newIPRateLimiter(30, time.Minute)).Post("/forgot-password", handler.forgotPassword)
	router.With(newIPRateLimiter(60, time.Minute)).Post("/reset-password", handler.resetPassword)
	router.Group(func(protected chi.Router) {
		protected.Use(jwtauth.Verifier(handler.tokens))
		protected.Use(jwtauth.Authenticator(handler.tokens))
		protected.Use(handler.requireActiveSession)
		protected.Use(requireCSRF)
		protected.With(newIPRateLimiter(10, time.Minute)).Post("/change-password", handler.changePassword)
		protected.Post("/logout", handler.logout)
	})
	return router
}

func (handler *Handler) register(w http.ResponseWriter, request *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		FullName string `json:"full_name"`
	}
	if err := decode(request, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid input")
		return
	}
	email, err := identity.NormalizeEmail(input.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err = identity.ValidatePassword(input.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	fullName := strings.TrimSpace(input.FullName)
	if fullName == "" {
		fullName = strings.Split(email, "@")[0]
	}
	if fullName, err = identity.NormalizeFullName(fullName); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := identity.HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create identity")
		return
	}
	userID, err := handler.store.Register(request.Context(), email, fullName, hash)
	if err != nil {
		writeError(w, http.StatusConflict, "identity already exists")
		return
	}
	if err = handler.provisioner.ProvisionCustomer(request.Context(), userID, email, fullName); err != nil {
		if rollbackErr := handler.store.Delete(request.Context(), userID); rollbackErr != nil {
			log.Error().Err(rollbackErr).Msg("Identity compensation failed")
		}
		writeError(w, http.StatusServiceUnavailable, "customer provisioning unavailable")
		return
	}
	token, err := handler.issueToken(userID, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	setCookie(w, request, token)
	writeJSON(w, http.StatusCreated, map[string]string{"user_id": userID.String(), "email": email})
}

func (handler *Handler) login(w http.ResponseWriter, request *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(request, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid input")
		return
	}
	authenticated, err := handler.auth.Authenticate(request.Context(), input.Email, input.Password)
	if errors.Is(err, identity.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "identity unavailable")
		return
	}
	syncCtx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	if err = handler.provisioner.SyncCustomerSessionVersion(syncCtx, authenticated.UserID, authenticated.SessionVersion); err != nil {
		writeError(w, http.StatusServiceUnavailable, "session synchronization temporarily unavailable")
		return
	}
	token, err := handler.issueToken(authenticated.UserID, authenticated.SessionVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	setCookie(w, request, token)
	writeJSON(w, http.StatusOK, map[string]string{"message": "login successful"})
}

func (handler *Handler) logout(w http.ResponseWriter, request *http.Request) {
	userID, err := authenticatedUserID(request)
	if err != nil {
		clearCookie(w, request)
		writeError(w, http.StatusUnauthorized, "invalid session")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	version, err := handler.store.RevokeSessions(ctx, userID)
	if err != nil {
		clearCookie(w, request)
		writeError(w, http.StatusServiceUnavailable, "session revocation temporarily unavailable")
		return
	}
	if err = handler.provisioner.SyncCustomerSessionVersion(ctx, userID, version); err != nil {
		clearCookie(w, request)
		writeError(w, http.StatusServiceUnavailable, "session synchronization temporarily unavailable")
		return
	}
	clearCookie(w, request)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logout successful"})
}

func (handler *Handler) changePassword(w http.ResponseWriter, request *http.Request) {
	userID, err := authenticatedUserID(request)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid session")
		return
	}
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err = decode(request, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid input")
		return
	}
	account, found, err := handler.store.FindLoginAccountByID(request.Context(), userID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "identity unavailable")
		return
	}
	if !found || !identity.VerifyPassword(account.HashedPassword, input.CurrentPassword) {
		identity.VerifyDummyPassword(input.CurrentPassword)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err = identity.ValidatePassword(input.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if identity.VerifyPassword(account.HashedPassword, input.NewPassword) {
		writeError(w, http.StatusBadRequest, "new password must be different")
		return
	}
	hash, err := identity.HashPassword(input.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	version, changed, err := handler.store.ChangePassword(ctx, userID, account.HashedPassword, hash)
	if err != nil {
		clearCookie(w, request)
		writeError(w, http.StatusServiceUnavailable, "password update temporarily unavailable")
		return
	}
	if !changed {
		clearCookie(w, request)
		writeError(w, http.StatusConflict, "credentials changed; sign in again")
		return
	}
	if err = handler.provisioner.SyncCustomerSessionVersion(ctx, userID, version); err != nil {
		clearCookie(w, request)
		writeError(w, http.StatusServiceUnavailable, "session synchronization temporarily unavailable")
		return
	}
	clearCookie(w, request)
	writeJSON(w, http.StatusOK, map[string]string{"message": "password updated; sign in again"})
}

func (handler *Handler) forgotPassword(w http.ResponseWriter, request *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	_ = decode(request, &input)
	email, err := identity.NormalizeEmail(input.Email)
	if err != nil {
		email = ""
	}
	dispatch := handler.dispatch
	if dispatch == nil {
		dispatch = func(job func()) { go job() }
	}
	dispatch(func() { handler.processPasswordReset(email) })
	writeJSON(w, http.StatusAccepted, accepted())
}

func (handler *Handler) processPasswordReset(email string) {
	ctx, cancel := context.WithTimeout(context.Background(), resetJobTimeout)
	defer cancel()
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		log.Warn().Msg("Password reset entropy generation failed")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	lookupEmail := email
	if lookupEmail == "" {
		lookupEmail = "invalid-password-reset@invalid.example"
	}
	userID, name, found, err := handler.store.GetByEmail(ctx, lookupEmail)
	if err != nil || !found {
		return
	}
	if err = handler.store.CreatePasswordReset(ctx, userID, hash[:], time.Now().UTC().Add(resetLifetime)); err != nil {
		log.Warn().Msg("Password reset persistence failed")
		return
	}
	if err = handler.notifier.SendPasswordReset(ctx, email, name, token); err != nil {
		// Adapter error strings are intentionally omitted because they may contain
		// a recipient or provider request details.
		log.Warn().Msg("Password reset delivery failed")
	}
}

func (handler *Handler) resetPassword(w http.ResponseWriter, request *http.Request) {
	var input struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := decode(request, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid input")
		return
	}
	input.Token = strings.TrimSpace(input.Token)
	raw, err := base64.RawURLEncoding.DecodeString(input.Token)
	if err != nil || len(raw) != 32 {
		writeError(w, http.StatusBadRequest, "reset link is invalid or expired")
		return
	}
	if err = identity.ValidatePassword(input.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := identity.HashPassword(input.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	tokenHash := sha256.Sum256([]byte(input.Token))
	userID, version, err := handler.store.ResetPassword(request.Context(), tokenHash[:], hash, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "reset link is invalid or expired")
		return
	}
	revokeCtx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()
	if err = handler.provisioner.SyncCustomerSessionVersion(revokeCtx, userID, version); err != nil {
		log.Error().Msg("Banking session synchronization failed after password reset")
		writeError(w, http.StatusServiceUnavailable, "password updated; session synchronization is temporarily unavailable")
		return
	}
	clearCookie(w, request)
	writeJSON(w, http.StatusOK, map[string]string{"message": "password updated successfully"})
}

func (handler *Handler) issueToken(userID uuid.UUID, version int64) (string, error) {
	_, token, err := handler.tokens.Encode(map[string]any{"user_id": userID.String(), "session_version": version, "iss": issuer, "aud": audience, "jti": uuid.NewString(), "iat": time.Now().Unix(), "nbf": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(sessionLifetime).Unix()})
	return token, err
}

func (handler *Handler) requireActiveSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		userID, err := authenticatedUserID(request)
		if err != nil {
			clearCookie(w, request)
			writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}
		version, err := sessionVersion(request)
		if err != nil {
			clearCookie(w, request)
			writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}
		current, err := handler.store.SessionVersion(request.Context(), userID)
		if err != nil || current != version {
			clearCookie(w, request)
			writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}
		next.ServeHTTP(w, request)
	})
}

func authenticatedUserID(request *http.Request) (uuid.UUID, error) {
	_, claims, err := jwtauth.FromContext(request.Context())
	if err != nil {
		return uuid.Nil, err
	}
	raw, ok := claims["user_id"].(string)
	if !ok {
		return uuid.Nil, errors.New("user_id claim missing")
	}
	return uuid.Parse(raw)
}

func sessionVersion(request *http.Request) (int64, error) {
	_, claims, err := jwtauth.FromContext(request.Context())
	if err != nil {
		return 0, err
	}
	switch value := claims["session_version"].(type) {
	case float64:
		return int64(value), nil
	case int64:
		return value, nil
	case json.Number:
		return value.Int64()
	default:
		return 0, errors.New("session_version claim missing")
	}
}

func accepted() map[string]string {
	return map[string]string{"message": "If the address is registered, a password reset email has been sent."}
}
func decode(r *http.Request, target any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 64<<10+1))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}

func limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const maxBodyBytes int64 = 64 << 10
		if r.ContentLength > maxBodyBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && r.ContentLength != 0 {
			contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
			if contentType != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		next.ServeHTTP(w, r)
	})
}
func requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			next.ServeHTTP(w, r)
			return
		}
		fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
		if fetchSite == "cross-site" || fetchSite == "same-site" || r.Header.Get("X-CSRF-Protection") != "1" {
			writeError(w, http.StatusForbidden, "cross-site request blocked")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func secure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
func setCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int(sessionLifetime.Seconds()), Expires: time.Now().Add(sessionLifetime), HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode})
}
func clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode})
}

type rateWindow struct {
	started time.Time
	count   int
}

func newIPRateLimiter(limit int, window time.Duration) func(http.Handler) http.Handler {
	var mutex sync.Mutex
	clients := make(map[string]rateWindow)
	lastCleanup := time.Now()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			mutex.Lock()
			if now.Sub(lastCleanup) >= window {
				for key, candidate := range clients {
					if now.Sub(candidate.started) >= window {
						delete(clients, key)
					}
				}
				lastCleanup = now
			}
			entry := clients[host]
			if entry.started.IsZero() || now.Sub(entry.started) >= window {
				entry = rateWindow{started: now}
			}
			entry.count++
			clients[host] = entry
			limited := entry.count > limit
			retryAfter := max(1, int(time.Until(entry.started.Add(window)).Seconds()))
			mutex.Unlock()
			if limited {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				writeError(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

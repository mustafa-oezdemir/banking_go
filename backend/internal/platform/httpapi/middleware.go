package api

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

var (
	// TokenAuth holds the JWT authenticator used by the API package.
	TokenAuth *jwtauth.JWTAuth
)

const (
	sessionCookieName = "jwt"
	sessionDuration   = 2 * time.Hour
	tokenIssuer       = "pehlione-banking"
	tokenAudience     = "pehlione-banking-web"
)

// InitTokenAuthFromEnv initializes JWT auth using the JWT_SECRET environment variable.
func InitTokenAuthFromEnv() error {
	// Keep bootstrap simple: this function is called once from main().
	secret := os.Getenv("JWT_SECRET")
	return InitTokenAuth(secret)
}

// InitTokenAuth initializes JWT auth with the provided secret.
func InitTokenAuth(secret string) error {
	// Fail fast if JWT configuration is insecure or missing.
	if secret == "" {
		return errors.New("JWT_SECRET environment variable is required")
	}

	if len(secret) < 32 {
		return errors.New("JWT_SECRET must be at least 32 characters")
	}

	TokenAuth = jwtauth.New(
		"HS256",
		[]byte(secret),
		nil,
		jwt.WithIssuer(tokenIssuer),
		jwt.WithAudience(tokenAudience),
	)
	return nil
}

// GenerateToken creates a signed JWT for the given user ID.
func GenerateToken(userID uuid.UUID) (string, error) {
	return GenerateTokenForVersion(userID, 0)
}

// GenerateTokenForVersion creates a token bound to the user's current server-side session generation.
func GenerateTokenForVersion(userID uuid.UUID, sessionVersion int64) (string, error) {
	if TokenAuth == nil {
		return "", errors.New("token auth is not initialized")
	}

	// Include user identity and expiry in signed JWT claims.
	claims := map[string]interface{}{
		"user_id":         userID.String(),
		"session_version": sessionVersion,
		"iss":             tokenIssuer,
		"aud":             tokenAudience,
		"jti":             uuid.NewString(),
		"iat":             time.Now().Unix(),
		"nbf":             time.Now().Add(-time.Minute).Unix(),
		"exp":             time.Now().Add(sessionDuration).Unix(),
	}
	_, tokenString, err := TokenAuth.Encode(claims)
	return tokenString, err
}

// SetSessionCookie stores the JWT in an HttpOnly, same-site session cookie.
func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := r.TLS != nil ||
		strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") ||
		strings.EqualFold(os.Getenv("RENDER"), "true")
	// Secure is disabled only for explicit local HTTP development.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Production sets Secure via TLS/proxy/Render detection.
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		Expires:  time.Now().Add(sessionDuration),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// ClearSessionCookie expires the browser session cookie.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil ||
		strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") ||
		strings.EqualFold(os.Getenv("RENDER"), "true")
	// Match the original cookie attributes so browsers reliably expire it.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Production sets Secure via TLS/proxy/Render detection.
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

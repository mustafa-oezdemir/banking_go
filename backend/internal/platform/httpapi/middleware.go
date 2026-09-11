package api

import (
	"errors"
	"os"

	"github.com/go-chi/jwtauth/v5"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

var (
	// TokenAuth holds the JWT authenticator used by the API package.
	TokenAuth *jwtauth.JWTAuth
)

const (
	sessionCookieName = "jwt"
	// Tokens are issued by the Identity service. Banking only validates them
	// before applying its own customer/account authorization rules.
	tokenIssuer   = "pehlione-identity"
	tokenAudience = "pehlione-banking-api"
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

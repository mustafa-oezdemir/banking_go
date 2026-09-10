package identity

import (
	"errors"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 15
	// MaxPasswordBytes is bcrypt's safe input boundary.
	MaxPasswordBytes = 72
	maxPasswordBytes = MaxPasswordBytes
	maxEmailLength   = 254
)

var commonPasswords = map[string]struct{}{
	"123456789012345": {},
	"letmeinletmein":  {},
	"password123456":  {},
	"qwertyqwerty123": {},
	"welcome12345678": {},
}

var dummyPasswordHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("timing-defense-password"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return hash
}()

// HashPassword validates and hashes a password at the identity boundary.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// VerifyPassword compares a stored bcrypt hash with a candidate password.
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// VerifyDummyPassword performs the same expensive comparison for unknown users.
func VerifyDummyPassword(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
}

// NormalizeEmail validates and canonicalizes an identity email address.
func NormalizeEmail(rawEmail string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(rawEmail))
	if email == "" || len(email) > maxEmailLength {
		return "", errors.New("a valid email address is required")
	}

	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return "", errors.New("a valid email address is required")
	}
	return email, nil
}

// ValidatePassword applies the registration and reset password policy.
func ValidatePassword(password string) error {
	if len([]rune(password)) < minPasswordLength {
		return errors.New("password must be at least 15 characters")
	}
	if len([]byte(password)) > maxPasswordBytes {
		return errors.New("password must be at most 72 bytes")
	}
	if _, blocked := commonPasswords[strings.ToLower(password)]; blocked {
		return errors.New("password is too common")
	}
	return nil
}

package api

import (
	"errors"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 15
	maxPasswordBytes  = 72
	maxEmailLength    = 254
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

func passwordMatches(hash []byte, password string) bool {
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}

func normalizeEmail(rawEmail string) (string, error) {
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

func validateRegistrationPassword(password string) error {
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

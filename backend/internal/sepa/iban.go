// Package sepa contains simulation-safe SEPA primitives. It never performs
// network calls and cannot route real payments.
package sepa

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

const (
	// DemoBankCode is intentionally reserved only inside this application.
	// It is not evidence of reachability and must not be used for real routing.
	DemoBankCode     = "99999999"
	GermanIBANLength = 22
)

var ErrInvalidIBAN = errors.New("invalid IBAN")

// NormalizeIBAN strips spaces and uppercases an IBAN for validation/storage.
func NormalizeIBAN(value string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value)))
}

// ValidateIBAN validates length, character shape and the ISO 13616 MOD-97
// checksum. German demo accounts are exactly 22 characters long.
func ValidateIBAN(value string) error {
	iban := NormalizeIBAN(value)
	if len(iban) < 15 || len(iban) > 34 {
		return ErrInvalidIBAN
	}
	if iban[:2] == "DE" && len(iban) != GermanIBANLength {
		return ErrInvalidIBAN
	}
	for i, r := range iban {
		if i < 2 {
			if r < 'A' || r > 'Z' {
				return ErrInvalidIBAN
			}
			continue
		}
		if i < 4 {
			if r < '0' || r > '9' {
				return ErrInvalidIBAN
			}
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z')) {
			return ErrInvalidIBAN
		}
	}
	if mod97(iban) != 1 {
		return ErrInvalidIBAN
	}
	return nil
}

// GenerateGermanDemoIBAN creates a valid 22-character German-format demo
// IBAN. Random account numbers and a database UNIQUE constraint provide two
// independent uniqueness layers.
func GenerateGermanDemoIBAN() (string, error) {
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(10), nil)
	accountNumber, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", fmt.Errorf("generate demo account number: %w", err)
	}
	bban := DemoBankCode + fmt.Sprintf("%010d", accountNumber)
	checkInput := bban + "131400" // D=13, E=14, temporary checksum 00.
	value := new(big.Int)
	if _, ok := value.SetString(checkInput, 10); !ok {
		return "", errors.New("build demo IBAN checksum")
	}
	checkDigits := 98 - new(big.Int).Mod(value, big.NewInt(97)).Int64()
	iban := fmt.Sprintf("DE%02d%s", checkDigits, bban)
	if err := ValidateIBAN(iban); err != nil {
		return "", fmt.Errorf("generated IBAN failed validation: %w", err)
	}
	return iban, nil
}

// MaskIBAN keeps enough context for users while avoiding exposure in lists or
// logs. Account-owner detail responses may deliberately return the full IBAN.
func MaskIBAN(value string) string {
	iban := NormalizeIBAN(value)
	if len(iban) <= 8 {
		return "****"
	}
	return iban[:4] + strings.Repeat("*", len(iban)-8) + iban[len(iban)-4:]
}

func mod97(iban string) int {
	rearranged := iban[4:] + iban[:4]
	remainder := 0
	for _, r := range rearranged {
		if r >= '0' && r <= '9' {
			remainder = (remainder*10 + int(r-'0')) % 97
			continue
		}
		value := int(r-'A') + 10
		remainder = (remainder*100 + value) % 97
	}
	return remainder
}

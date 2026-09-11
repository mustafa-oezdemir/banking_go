// Package account owns customer account identifiers and profile rules.
package account

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxAccountNameLength = 100

var (
	phonePattern  = regexp.MustCompile(`^[+0-9() /-]+$`)
	markupPattern = regexp.MustCompile(`(?i)</?[a-z][^>]*>`)
)

// ProfileInput is the customer-maintained profile data accepted by the account boundary.
type ProfileInput struct {
	FullName     string
	Phone        string
	BirthDate    string
	AddressLine1 string
	AddressLine2 string
	PostalCode   string
	City         string
	CountryCode  string
}

// ValidateName normalizes and validates a customer-visible account name.
func ValidateName(raw string) (string, error) {
	return normalizePlainText(raw, "account name", 1, maxAccountNameLength, true)
}

// NormalizeProfile canonicalizes and validates customer-maintained profile data.
func NormalizeProfile(input ProfileInput, now time.Time) (ProfileInput, time.Time, error) {
	input.FullName = strings.TrimSpace(input.FullName)
	input.Phone = strings.TrimSpace(input.Phone)
	input.BirthDate = strings.TrimSpace(input.BirthDate)
	input.AddressLine1 = strings.TrimSpace(input.AddressLine1)
	input.AddressLine2 = strings.TrimSpace(input.AddressLine2)
	input.PostalCode = strings.TrimSpace(input.PostalCode)
	input.City = strings.TrimSpace(input.City)
	input.CountryCode = strings.ToUpper(strings.TrimSpace(input.CountryCode))

	var err error
	if input.FullName, err = normalizePlainText(input.FullName, "full_name", 2, 100, true); err != nil {
		return ProfileInput{}, time.Time{}, err
	}
	if len(input.Phone) < 7 || len(input.Phone) > 32 || !phonePattern.MatchString(input.Phone) {
		return ProfileInput{}, time.Time{}, errors.New("phone format is invalid")
	}
	if input.AddressLine1, err = normalizePlainText(input.AddressLine1, "address_line1", 3, 120, true); err != nil {
		return ProfileInput{}, time.Time{}, err
	}
	if input.AddressLine2, err = normalizePlainText(input.AddressLine2, "address_line2", 0, 120, false); err != nil {
		return ProfileInput{}, time.Time{}, err
	}
	if input.PostalCode, err = normalizePlainText(input.PostalCode, "postal_code", 3, 12, true); err != nil {
		return ProfileInput{}, time.Time{}, err
	}
	if input.City, err = normalizePlainText(input.City, "city", 2, 80, true); err != nil {
		return ProfileInput{}, time.Time{}, err
	}
	if len(input.CountryCode) != 2 || input.CountryCode[0] < 'A' || input.CountryCode[0] > 'Z' || input.CountryCode[1] < 'A' || input.CountryCode[1] > 'Z' {
		return ProfileInput{}, time.Time{}, errors.New("country_code must be a two-letter ISO code")
	}
	birthDate, err := time.Parse("2006-01-02", input.BirthDate)
	if err != nil || birthDate.After(now) || birthDate.Before(now.AddDate(-120, 0, 0)) {
		return ProfileInput{}, time.Time{}, errors.New("birth_date is invalid")
	}
	return input, birthDate, nil
}

func normalizePlainText(raw, field string, min, max int, required bool) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" && !required {
		return "", nil
	}
	length := utf8.RuneCountInString(value)
	if length < min || length > max {
		return "", errors.New(field + " has an invalid length")
	}
	if markupPattern.MatchString(value) {
		return "", errors.New(field + " must not contain HTML markup")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", errors.New(field + " must not contain control characters")
		}
	}
	return value, nil
}

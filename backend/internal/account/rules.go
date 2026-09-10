// Package account owns customer account identifiers and profile rules.
package account

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const maxAccountNameLength = 100

var phonePattern = regexp.MustCompile(`^[+0-9() /-]+$`)

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
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("account name is required")
	}
	if utf8.RuneCountInString(name) > maxAccountNameLength {
		return "", errors.New("account name must be 100 characters or fewer")
	}
	return name, nil
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

	if utf8.RuneCountInString(input.FullName) < 2 || utf8.RuneCountInString(input.FullName) > 100 {
		return ProfileInput{}, time.Time{}, errors.New("full_name must contain between 2 and 100 characters")
	}
	if len(input.Phone) < 7 || len(input.Phone) > 32 || !phonePattern.MatchString(input.Phone) {
		return ProfileInput{}, time.Time{}, errors.New("phone format is invalid")
	}
	if utf8.RuneCountInString(input.AddressLine1) < 3 || utf8.RuneCountInString(input.AddressLine1) > 120 ||
		utf8.RuneCountInString(input.AddressLine2) > 120 {
		return ProfileInput{}, time.Time{}, errors.New("address format is invalid")
	}
	if len(input.PostalCode) < 3 || len(input.PostalCode) > 12 {
		return ProfileInput{}, time.Time{}, errors.New("postal_code must contain between 3 and 12 characters")
	}
	if utf8.RuneCountInString(input.City) < 2 || utf8.RuneCountInString(input.City) > 80 {
		return ProfileInput{}, time.Time{}, errors.New("city must contain between 2 and 80 characters")
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

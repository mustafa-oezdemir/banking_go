package card

import (
	"testing"

	"github.com/google/uuid"
)

func TestGeneratePANCreatesValidVisaNumber(t *testing.T) {
	pan, err := generatePAN()
	if err != nil {
		t.Fatalf("generatePAN() error = %v", err)
	}
	if len(pan) != 16 || pan[0] != '4' || !validPAN(pan) {
		t.Fatalf("generatePAN() = %q, want a valid 16-digit Visa PAN", pan)
	}
}

func TestValidPAN(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "known valid visa", value: "4242424242424242", valid: true},
		{name: "bad checksum", value: "4242424242424243", valid: false},
		{name: "non visa", value: "5242424242424242", valid: false},
		{name: "wrong length", value: "424242424242424", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validPAN(test.value); got != test.valid {
				t.Fatalf("validPAN(%q) = %t, want %t", test.value, got, test.valid)
			}
		})
	}
}

func TestNormalizePAN(t *testing.T) {
	if got := normalizePAN(" 4242-4242 4242-4242 "); got != "4242424242424242" {
		t.Fatalf("normalizePAN() = %q", got)
	}
}

func TestDerivedCredentialsAreDeterministicAndValid(t *testing.T) {
	service := NewService(nil, "0123456789abcdef0123456789abcdef")
	cardID := uuid.MustParse("c0a80101-0000-4000-8000-000000000001")
	pan, cvc := service.deriveCredentials(cardID)
	secondPAN, secondCVC := service.deriveCredentials(cardID)
	if pan != secondPAN || cvc != secondCVC {
		t.Fatal("derived credentials must be deterministic")
	}
	if !validPAN(pan) {
		t.Fatalf("derived PAN %q is not Luhn-valid", pan)
	}
	if !validCVC(cvc) {
		t.Fatalf("derived CVC %q is not a 3-digit value", cvc)
	}
}

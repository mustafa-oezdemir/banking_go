package card

import "testing"

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

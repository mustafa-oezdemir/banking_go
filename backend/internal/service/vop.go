package service

import (
	"context"
	"database/sql"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
)

const (
	VoPMatch      = "MATCH"
	VoPCloseMatch = "CLOSE_MATCH"
	VoPNoMatch    = "NO_MATCH"
	VoPOther      = "OTHER"
)

// VoPResult is the simulation-only Verification of Payee response. ActualName
// is populated only for CLOSE_MATCH; NO_MATCH never discloses account-holder
// data.
type VoPResult struct {
	Result        string  `json:"result"`
	SuggestedName *string `json:"suggested_name,omitempty"`
	DemoNotice    string  `json:"demo_notice"`
}

// VerifyPayee resolves an internal demo account or a user's saved demo payee.
// Unknown external IBANs return OTHER because no real banking network is used.
func (s *PaymentService) VerifyPayee(ctx context.Context, ownerID uuid.UUID, providedName, rawIBAN string) (VoPResult, error) {
	iban := sepa.NormalizeIBAN(rawIBAN)
	if err := sepa.ValidateIBAN(iban); err != nil {
		return VoPResult{}, err
	}

	actualName := ""
	payee, err := s.store.LookupPayeeByIBAN(ctx, iban)
	if err == nil && payee.Status == "ACTIVE" {
		actualName = payee.FullName
	} else if err != nil && err != sql.ErrNoRows {
		return VoPResult{}, err
	}

	if actualName == "" {
		beneficiary, beneficiaryErr := s.store.GetBeneficiaryByIBAN(ctx, sqlcGetBeneficiaryParams(ownerID, iban))
		if beneficiaryErr == nil {
			actualName = beneficiary.Name
		} else if beneficiaryErr != sql.ErrNoRows {
			return VoPResult{}, beneficiaryErr
		}
	}

	return EvaluateVoP(providedName, actualName, actualName != ""), nil
}

// EvaluateVoP deterministically maps a provided and known account-holder name
// to the four result categories described by the EU VoP scheme.
func EvaluateVoP(providedName, actualName string, available bool) VoPResult {
	notice := "Demo-Prüfung – keine Verbindung zu einem echten Banknetz"
	if !available || normalizePersonName(providedName) == "" {
		return VoPResult{Result: VoPOther, DemoNotice: notice}
	}
	provided := normalizePersonName(providedName)
	actual := normalizePersonName(actualName)
	if provided == actual {
		return VoPResult{Result: VoPMatch, DemoNotice: notice}
	}
	distance := levenshtein([]rune(provided), []rune(actual))
	threshold := max(2, len([]rune(actual))/5)
	if distance <= threshold || sameNameTokens(provided, actual) {
		suggestion := strings.TrimSpace(actualName)
		return VoPResult{Result: VoPCloseMatch, SuggestedName: &suggestion, DemoNotice: notice}
	}
	return VoPResult{Result: VoPNoMatch, DemoNotice: notice}
}

func normalizePersonName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	space := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space && b.Len() > 0 {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func sameNameTokens(a, b string) bool {
	aTokens := strings.Fields(a)
	bTokens := strings.Fields(b)
	if len(aTokens) != len(bTokens) || len(aTokens) < 2 {
		return false
	}
	counts := make(map[string]int, len(aTokens))
	for _, token := range aTokens {
		counts[token]++
	}
	for _, token := range bTokens {
		counts[token]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i, aRune := range a {
		current := make([]int, len(b)+1)
		current[0] = i + 1
		for j, bRune := range b {
			cost := 0
			if aRune != bRune {
				cost = 1
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

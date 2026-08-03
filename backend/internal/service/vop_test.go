package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvaluateVoPScenarios(t *testing.T) {
	tests := []struct {
		name       string
		provided   string
		actual     string
		want       string
		available  bool
		suggestion bool
	}{
		{name: "match", provided: "Anna Müller", actual: "Anna Müller", available: true, want: VoPMatch},
		{name: "normalized match", provided: "  ANNA   MÜLLER ", actual: "Anna Müller", available: true, want: VoPMatch},
		{name: "close typo", provided: "Ana Müller", actual: "Anna Müller", available: true, want: VoPCloseMatch, suggestion: true},
		{name: "close token order", provided: "Müller Anna", actual: "Anna Müller", available: true, want: VoPCloseMatch, suggestion: true},
		{name: "no match", provided: "Max Mustermann", actual: "Anna Müller", available: true, want: VoPNoMatch},
		{name: "other", provided: "Anna Müller", actual: "", available: false, want: VoPOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateVoP(tt.provided, tt.actual, tt.available)
			assert.Equal(t, tt.want, got.Result)
			assert.Equal(t, tt.suggestion, got.SuggestedName != nil)
			if tt.want == VoPNoMatch {
				assert.Nil(t, got.SuggestedName, "NO_MATCH must not disclose the real name")
			}
		})
	}
}

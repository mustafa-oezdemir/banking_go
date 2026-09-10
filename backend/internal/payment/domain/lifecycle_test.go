package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentLifecycleAllowsDeclaredTransitions(t *testing.T) {
	for _, transition := range [][2]Status{
		{StatusAwaitingConfirmation, StatusScheduled},
		{StatusAwaitingConfirmation, StatusProcessing},
		{StatusScheduled, StatusProcessing},
		{StatusProcessing, StatusBooked},
		{StatusProcessing, StatusFailed},
		{StatusProcessing, StatusScheduled},
	} {
		require.NoError(t, ValidateTransition(transition[0], transition[1]))
	}
}

func TestPaymentLifecycleRejectsInvalidAndTerminalTransitions(t *testing.T) {
	for _, transition := range [][2]Status{
		{StatusAwaitingConfirmation, StatusBooked},
		{StatusScheduled, StatusBooked},
		{StatusBooked, StatusProcessing},
		{StatusFailed, StatusScheduled},
		{StatusCancelled, StatusProcessing},
	} {
		assert.ErrorIs(t, ValidateTransition(transition[0], transition[1]), ErrInvalidTransition)
	}
}

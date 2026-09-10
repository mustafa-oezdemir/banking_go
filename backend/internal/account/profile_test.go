package account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type profileRepositoryStub struct {
	profile      Profile
	update       ProfileUpdate
	updateCalled bool
}

func (repository *profileRepositoryStub) GetCustomerProfile(_ context.Context, userID uuid.UUID) (Profile, error) {
	if repository.profile.ID != userID {
		return Profile{}, errors.New("profile not found")
	}
	return repository.profile, nil
}

func (repository *profileRepositoryStub) UpdateCustomerProfile(_ context.Context, update ProfileUpdate) (Profile, error) {
	repository.updateCalled = true
	repository.update = update
	repository.profile = Profile{
		ID: update.UserID, FullName: update.FullName, Phone: update.Phone,
		BirthDate: update.BirthDate.Format("2006-01-02"), AddressLine1: update.AddressLine1,
		AddressLine2: update.AddressLine2, PostalCode: update.PostalCode,
		City: update.City, CountryCode: update.CountryCode,
	}
	return repository.profile, nil
}

func TestProfileServiceUpdatesThroughOwnerScopedPort(t *testing.T) {
	userID := uuid.New()
	repository := &profileRepositoryStub{}
	service := NewProfileService(repository, func() time.Time {
		return time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	})

	profile, err := service.Update(t.Context(), userID, ProfileInput{
		FullName: "  Anna Beispiel ", Phone: "+49 170 1234567", BirthDate: "1990-05-12",
		AddressLine1: " Musterstraße 12 ", PostalCode: "10115", City: " Berlin ", CountryCode: "de",
	})
	require.NoError(t, err)
	assert.True(t, repository.updateCalled)
	assert.Equal(t, userID, repository.update.UserID)
	assert.Equal(t, "Anna Beispiel", repository.update.FullName)
	assert.Equal(t, "DE", repository.update.CountryCode)
	assert.Equal(t, "1990-05-12", repository.update.BirthDate.Format("2006-01-02"))
	assert.True(t, profile.Complete())
}

func TestProfileServiceRejectsInvalidInputBeforePersistence(t *testing.T) {
	repository := &profileRepositoryStub{}
	service := NewProfileService(repository, func() time.Time {
		return time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	})

	_, err := service.Update(t.Context(), uuid.New(), ProfileInput{FullName: "x"})
	require.Error(t, err)
	assert.False(t, repository.updateCalled)
}

package account

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidProfile classifies customer-supplied profile validation failures.
var ErrInvalidProfile = errors.New("invalid customer profile")

type profileValidationError struct {
	cause error
}

func (validationError profileValidationError) Error() string { return validationError.cause.Error() }
func (profileValidationError) Unwrap() error                 { return ErrInvalidProfile }

// Profile contains customer-maintained personal and address data. Email is
// read-only because changing it requires a separate verified identity flow.
type Profile struct {
	UpdatedAt    time.Time
	Email        string
	FullName     string
	Phone        string
	BirthDate    string
	AddressLine1 string
	AddressLine2 string
	PostalCode   string
	City         string
	CountryCode  string
	ID           uuid.UUID
}

// Complete reports whether every mandatory customer profile field is present.
func (profile Profile) Complete() bool {
	return profile.FullName != "" && profile.Phone != "" && profile.BirthDate != "" &&
		profile.AddressLine1 != "" && profile.PostalCode != "" && profile.City != "" && profile.CountryCode != ""
}

// ProfileUpdate is the normalized persistence command emitted by ProfileService.
type ProfileUpdate struct {
	BirthDate    time.Time
	FullName     string
	Phone        string
	AddressLine1 string
	AddressLine2 string
	PostalCode   string
	City         string
	CountryCode  string
	UserID       uuid.UUID
}

// ProfileRepository is the narrow persistence port required by the profile use case.
type ProfileRepository interface {
	GetCustomerProfile(context.Context, uuid.UUID) (Profile, error)
	UpdateCustomerProfile(context.Context, ProfileUpdate) (Profile, error)
}

// ProfileService coordinates customer profile reads and validated updates.
type ProfileService struct {
	repository ProfileRepository
	now        func() time.Time
}

// NewProfileService constructs the profile application service. The optional
// clock keeps validation deterministic in unit tests.
func NewProfileService(repository ProfileRepository, clocks ...func() time.Time) *ProfileService {
	now := time.Now
	if len(clocks) > 0 && clocks[0] != nil {
		now = clocks[0]
	}
	return &ProfileService{repository: repository, now: now}
}

// Get returns the authenticated customer's own profile.
func (service *ProfileService) Get(ctx context.Context, userID uuid.UUID) (Profile, error) {
	return service.repository.GetCustomerProfile(ctx, userID)
}

// Update validates and normalizes profile data before invoking persistence.
func (service *ProfileService) Update(ctx context.Context, userID uuid.UUID, input ProfileInput) (Profile, error) {
	normalized, birthDate, err := NormalizeProfile(input, service.now().UTC())
	if err != nil {
		return Profile{}, profileValidationError{cause: err}
	}
	return service.repository.UpdateCustomerProfile(ctx, ProfileUpdate{
		UserID: userID, FullName: normalized.FullName, Phone: normalized.Phone, BirthDate: birthDate,
		AddressLine1: normalized.AddressLine1, AddressLine2: normalized.AddressLine2,
		PostalCode: normalized.PostalCode, City: normalized.City, CountryCode: normalized.CountryCode,
	})
}

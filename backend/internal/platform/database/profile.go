package db

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

// CustomerProfile contains customer-maintained personal and address data.
// Email is intentionally read-only in this flow because changing it requires
// a separate verified security process.
type CustomerProfile struct {
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

// UpdateCustomerProfileParams contains validated profile fields.
type UpdateCustomerProfileParams struct {
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

const customerProfileColumns = `
	id, email, full_name, phone, COALESCE(birth_date::TEXT, ''),
	address_line1, address_line2, postal_code, city, country_code, updated_at`

func scanCustomerProfile(row interface{ Scan(...any) error }) (CustomerProfile, error) {
	var profile CustomerProfile
	err := row.Scan(
		&profile.ID, &profile.Email, &profile.FullName, &profile.Phone, &profile.BirthDate,
		&profile.AddressLine1, &profile.AddressLine2, &profile.PostalCode, &profile.City,
		&profile.CountryCode, &profile.UpdatedAt,
	)
	return profile, err
}

// GetCustomerProfile returns only the authenticated user's profile.
func (store *Store) GetCustomerProfile(ctx context.Context, userID uuid.UUID) (CustomerProfile, error) {
	return scanCustomerProfile(store.db.QueryRowContext(ctx, `SELECT `+customerProfileColumns+` FROM users WHERE id = $1`, userID))
}

// UpdateCustomerProfile atomically updates the profile and records a
// non-sensitive audit marker. Personal values are deliberately excluded from
// event_data to avoid duplicating PII in the audit stream.
func (store *Store) UpdateCustomerProfile(ctx context.Context, input UpdateCustomerProfileParams) (CustomerProfile, error) {
	var profile CustomerProfile
	err := store.ExecTxWithHandle(ctx, func(_ *sqlc.Queries, executor sqlc.DBTX) error {
		var updateErr error
		profile, updateErr = scanCustomerProfile(executor.QueryRowContext(ctx, `
			UPDATE users
			SET full_name = $2, phone = $3, birth_date = $4, address_line1 = $5,
			    address_line2 = $6, postal_code = $7, city = $8, country_code = $9,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
			RETURNING `+customerProfileColumns,
			input.UserID, input.FullName, input.Phone, input.BirthDate, input.AddressLine1,
			input.AddressLine2, input.PostalCode, input.City, input.CountryCode,
		))
		if updateErr != nil {
			return updateErr
		}
		_, updateErr = executor.ExecContext(ctx, `
			INSERT INTO audit_events (owner_id, event_type, event_data)
			VALUES ($1, 'PROFILE_UPDATED', '{"scope":"customer_profile"}'::JSONB)`, input.UserID)
		return updateErr
	})
	return profile, err
}

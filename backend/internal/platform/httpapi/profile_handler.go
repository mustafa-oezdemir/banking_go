package api

import (
	"net/http"
	"time"

	"github.com/mustafa-oezdemir/banking_go/internal/account"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/database"
)

type profileResponse struct {
	UpdatedAt       time.Time `json:"updated_at"`
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	FullName        string    `json:"full_name"`
	Phone           string    `json:"phone"`
	BirthDate       string    `json:"birth_date"`
	AddressLine1    string    `json:"address_line1"`
	AddressLine2    string    `json:"address_line2"`
	PostalCode      string    `json:"postal_code"`
	City            string    `json:"city"`
	CountryCode     string    `json:"country_code"`
	ProfileComplete bool      `json:"profile_complete"`
}

type updateProfileRequest struct {
	FullName     string `json:"full_name"`
	Phone        string `json:"phone"`
	BirthDate    string `json:"birth_date"`
	AddressLine1 string `json:"address_line1"`
	AddressLine2 string `json:"address_line2"`
	PostalCode   string `json:"postal_code"`
	City         string `json:"city"`
	CountryCode  string `json:"country_code"`
}

func mapProfile(profile db.CustomerProfile) profileResponse {
	return profileResponse{
		ID: profile.ID.String(), Email: profile.Email, FullName: profile.FullName,
		Phone: profile.Phone, BirthDate: profile.BirthDate, AddressLine1: profile.AddressLine1,
		AddressLine2: profile.AddressLine2, PostalCode: profile.PostalCode, City: profile.City,
		CountryCode: profile.CountryCode, UpdatedAt: profile.UpdatedAt,
		ProfileComplete: profile.FullName != "" && profile.Phone != "" && profile.BirthDate != "" &&
			profile.AddressLine1 != "" && profile.PostalCode != "" && profile.City != "" && profile.CountryCode != "",
	}
}

// GetProfile godoc
// @Summary Get the authenticated customer's profile
// @Tags profile
// @Produce json
// @Success 200 {object} profileResponse
// @Failure 401 {object} ErrorResponse
// @Security Bearer
// @Router /profile [get]
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	profile, err := h.store.GetCustomerProfile(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	respondJSON(w, http.StatusOK, mapProfile(profile))
}

// UpdateProfile godoc
// @Summary Update the authenticated customer's profile
// @Description Updates personal and postal data. Email changes require a separate verified flow.
// @Tags profile
// @Accept json
// @Produce json
// @Param body body updateProfileRequest true "Profile data"
// @Success 200 {object} profileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security Bearer
// @Router /profile [patch]
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, err := authenticatedUserID(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var input updateProfileRequest
	if err = decodeStrictJSON(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid input")
		return
	}
	birthDate, err := validateProfileInput(&input, time.Now().UTC())
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	profile, err := h.store.UpdateCustomerProfile(r.Context(), db.UpdateCustomerProfileParams{
		UserID: userID, FullName: input.FullName, Phone: input.Phone, BirthDate: birthDate,
		AddressLine1: input.AddressLine1, AddressLine2: input.AddressLine2,
		PostalCode: input.PostalCode, City: input.City, CountryCode: input.CountryCode,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	respondJSON(w, http.StatusOK, mapProfile(profile))
}

func validateProfileInput(input *updateProfileRequest, now time.Time) (time.Time, error) {
	normalized, birthDate, err := account.NormalizeProfile(account.ProfileInput{
		FullName: input.FullName, Phone: input.Phone, BirthDate: input.BirthDate,
		AddressLine1: input.AddressLine1, AddressLine2: input.AddressLine2,
		PostalCode: input.PostalCode, City: input.City, CountryCode: input.CountryCode,
	}, now)
	if err != nil {
		return time.Time{}, err
	}
	input.FullName = normalized.FullName
	input.Phone = normalized.Phone
	input.BirthDate = normalized.BirthDate
	input.AddressLine1 = normalized.AddressLine1
	input.AddressLine2 = normalized.AddressLine2
	input.PostalCode = normalized.PostalCode
	input.City = normalized.City
	input.CountryCode = normalized.CountryCode
	return birthDate, nil
}

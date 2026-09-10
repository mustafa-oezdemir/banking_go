package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mustafa-oezdemir/banking_go/internal/db"
)

var phonePattern = regexp.MustCompile(`^[+0-9() /-]+$`)

type profileResponse struct {
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
	UpdatedAt       time.Time `json:"updated_at"`
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
	input.FullName = strings.TrimSpace(input.FullName)
	input.Phone = strings.TrimSpace(input.Phone)
	input.BirthDate = strings.TrimSpace(input.BirthDate)
	input.AddressLine1 = strings.TrimSpace(input.AddressLine1)
	input.AddressLine2 = strings.TrimSpace(input.AddressLine2)
	input.PostalCode = strings.TrimSpace(input.PostalCode)
	input.City = strings.TrimSpace(input.City)
	input.CountryCode = strings.ToUpper(strings.TrimSpace(input.CountryCode))

	if utf8.RuneCountInString(input.FullName) < 2 || utf8.RuneCountInString(input.FullName) > 100 {
		return time.Time{}, errors.New("full_name must contain between 2 and 100 characters")
	}
	if len(input.Phone) < 7 || len(input.Phone) > 32 || !phonePattern.MatchString(input.Phone) {
		return time.Time{}, errors.New("phone format is invalid")
	}
	if utf8.RuneCountInString(input.AddressLine1) < 3 || utf8.RuneCountInString(input.AddressLine1) > 120 ||
		utf8.RuneCountInString(input.AddressLine2) > 120 {
		return time.Time{}, errors.New("address format is invalid")
	}
	if len(input.PostalCode) < 3 || len(input.PostalCode) > 12 {
		return time.Time{}, errors.New("postal_code must contain between 3 and 12 characters")
	}
	if utf8.RuneCountInString(input.City) < 2 || utf8.RuneCountInString(input.City) > 80 {
		return time.Time{}, errors.New("city must contain between 2 and 80 characters")
	}
	if len(input.CountryCode) != 2 || input.CountryCode[0] < 'A' || input.CountryCode[0] > 'Z' || input.CountryCode[1] < 'A' || input.CountryCode[1] > 'Z' {
		return time.Time{}, errors.New("country_code must be a two-letter ISO code")
	}
	birthDate, err := time.Parse("2006-01-02", input.BirthDate)
	if err != nil || birthDate.After(now) || birthDate.Before(now.AddDate(-120, 0, 0)) {
		return time.Time{}, errors.New("birth_date is invalid")
	}
	return birthDate, nil
}

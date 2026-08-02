package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, "nosniff", rw.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rw.Header().Get("X-Frame-Options"))
	assert.Contains(t, rw.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
	assert.Contains(t, rw.Header().Get("Strict-Transport-Security"), "max-age=31536000")
}

func TestIPRateLimiter(t *testing.T) {
	t.Setenv("RENDER", "false")
	t.Setenv("TRUST_PROXY_HEADERS", "false")
	handler := NewIPRateLimiter(2, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for attempt := 1; attempt <= 3; attempt++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(attempt))
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)
		if attempt <= 2 {
			assert.Equal(t, http.StatusNoContent, rw.Code)
		} else {
			assert.Equal(t, http.StatusTooManyRequests, rw.Code)
			assert.NotEmpty(t, rw.Header().Get("Retry-After"))
		}
	}
}

func TestCSRFProtection(t *testing.T) {
	const allowedOrigin = "https://pehlione-banking.com"
	handler := CSRFProtection([]string{allowedOrigin})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{"name":"Savings"}`))
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "signed-session"})
		return req
	}

	missingHeader := httptest.NewRecorder()
	handler.ServeHTTP(missingHeader, newRequest())
	assert.Equal(t, http.StatusForbidden, missingHeader.Code)

	validRequest := newRequest()
	validRequest.Header.Set(csrfHeaderName, "1")
	validRequest.Header.Set("Origin", allowedOrigin)
	validRequest.Header.Set("Sec-Fetch-Site", "same-origin")
	validResponse := httptest.NewRecorder()
	handler.ServeHTTP(validResponse, validRequest)
	assert.Equal(t, http.StatusNoContent, validResponse.Code)

	crossSiteRequest := newRequest()
	crossSiteRequest.Header.Set(csrfHeaderName, "1")
	crossSiteRequest.Header.Set("Origin", "https://attacker.example")
	crossSiteRequest.Header.Set("Sec-Fetch-Site", "cross-site")
	crossSiteResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossSiteResponse, crossSiteRequest)
	assert.Equal(t, http.StatusForbidden, crossSiteResponse.Code)

	bearerRequest := newRequest()
	bearerRequest.Header.Set("Authorization", "Bearer API-client-token")
	bearerResponse := httptest.NewRecorder()
	handler.ServeHTTP(bearerResponse, bearerRequest)
	require.Equal(t, http.StatusNoContent, bearerResponse.Code)
}

func TestRequireJSON(t *testing.T) {
	handler := RequireJSON(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("email=user@example.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rw.Code)
}

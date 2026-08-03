package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/db"
)

// RequireActiveSession rejects cryptographically valid JWTs revoked by logout or role changes.
func RequireActiveSession(store *db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := authenticatedUserID(r)
			if err != nil {
				respondError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			version, err := tokenSessionVersion(r)
			if err != nil {
				respondError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			persisted, err := store.GetUserSessionVersion(r.Context(), userID)
			if err != nil || persisted != version {
				respondError(w, http.StatusUnauthorized, "session expired")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func tokenSessionVersion(r *http.Request) (int64, error) {
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		return 0, err
	}
	switch value := claims["session_version"].(type) {
	case float64:
		return int64(value), nil
	case int64:
		return value, nil
	case json.Number:
		return value.Int64()
	default:
		return 0, errors.New("session_version claim missing")
	}
}

func tokenExpiry(r *http.Request) (time.Time, error) {
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		return time.Time{}, err
	}
	var seconds int64
	switch value := claims["exp"].(type) {
	case float64:
		seconds = int64(value)
	case int64:
		seconds = value
	case json.Number:
		seconds, err = value.Int64()
	default:
		err = errors.New("exp claim missing")
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(seconds, 0), nil
}

func revokeAuthenticatedSession(store *db.Store, r *http.Request) {
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		return
	}
	raw, ok := claims["user_id"].(string)
	if !ok {
		return
	}
	userID, err := uuid.Parse(raw)
	if err == nil {
		if revokeErr := store.RevokeUserSessions(r.Context(), userID); revokeErr != nil {
			return
		}
	}
}

const csrfHeaderName = "X-CSRF-Protection"

type rateLimitWindow struct {
	startedAt time.Time
	count     int
}

// NewIPRateLimiter creates a fixed-window per-client limiter with bounded cleanup.
func NewIPRateLimiter(limit int, window time.Duration) func(http.Handler) http.Handler {
	var mutex sync.Mutex
	clients := make(map[string]rateLimitWindow)
	lastCleanup := time.Now()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			client := clientIP(r)

			mutex.Lock()
			if now.Sub(lastCleanup) >= window {
				for key, entry := range clients {
					if now.Sub(entry.startedAt) >= window {
						delete(clients, key)
					}
				}
				lastCleanup = now
			}

			entry := clients[client]
			if entry.startedAt.IsZero() || now.Sub(entry.startedAt) >= window {
				entry = rateLimitWindow{startedAt: now}
			}
			entry.count++
			clients[client] = entry
			limited := entry.count > limit
			retryAfter := max(1, int(time.Until(entry.startedAt.Add(window)).Seconds()))
			mutex.Unlock()

			if limited {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				respondError(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	trustProxyHeaders := strings.EqualFold(os.Getenv("RENDER"), "true") ||
		strings.EqualFold(os.Getenv("TRUST_PROXY_HEADERS"), "true")
	if trustProxyHeaders {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(forwarded) != nil {
			return forwarded
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(realIP) != nil {
			return realIP
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// SecurityHeaders adds browser hardening headers to every API response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		if !strings.HasPrefix(r.URL.Path, "/swagger/") {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		}
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// LimitRequestBody prevents oversized JSON payloads from exhausting resources.
func LimitRequestBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				respondError(w, http.StatusRequestEntityTooLarge, "request body is too large")
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireJSON rejects unsafe requests with an unexpected body content type.
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnsafeMethod(r.Method) && r.ContentLength != 0 {
			contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
			if contentType != "application/json" {
				respondError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// CSRFProtection protects cookie-authenticated unsafe requests with Fetch Metadata,
// an explicit custom header, and an origin allowlist.
func CSRFProtection(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.TrimSuffix(origin, "/")] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isUnsafeMethod(r.Method) || r.Header.Get("Authorization") != "" {
				next.ServeHTTP(w, r)
				return
			}
			if _, err := r.Cookie(sessionCookieName); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			fetchSite := strings.ToLower(r.Header.Get("Sec-Fetch-Site"))
			if fetchSite == "cross-site" || fetchSite == "same-site" {
				respondError(w, http.StatusForbidden, "cross-site request blocked")
				return
			}
			if r.Header.Get(csrfHeaderName) != "1" {
				respondError(w, http.StatusForbidden, "CSRF protection header required")
				return
			}

			origin := strings.TrimSuffix(r.Header.Get("Origin"), "/")
			if origin != "" {
				if _, ok := allowed[origin]; !ok {
					respondError(w, http.StatusForbidden, "request origin is not allowed")
					return
				}
			} else if fetchSite != "same-origin" {
				respondError(w, http.StatusForbidden, "request origin could not be verified")
				return
			}

			w.Header().Add("Vary", "Origin")
			w.Header().Add("Vary", "Sec-Fetch-Site")
			next.ServeHTTP(w, r)
		})
	}
}

func isUnsafeMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

// Package gateway implements the deliberately small public edge router.
package gateway

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const maxRequestBytes = 1 << 20

// Config names the two public upstreams. Notification remains private.
type Config struct{ IdentityURL, BankingURL string }

// Gateway routes auth paths to Identity and all remaining API paths to Banking.
type Gateway struct {
	identity, banking  *httputil.ReverseProxy
	requests, failures atomic.Uint64
}

// New validates upstream URLs and creates a gateway without business logic.
func New(config Config) (*Gateway, error) {
	identity, err := parseUpstream(config.IdentityURL)
	if err != nil {
		return nil, err
	}
	banking, err := parseUpstream(config.BankingURL)
	if err != nil {
		return nil, err
	}
	result := &Gateway{identity: proxy(identity), banking: proxy(banking)}
	for _, upstream := range []*httputil.ReverseProxy{result.identity, result.banking} {
		upstream.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			result.failures.Add(1)
			log.Warn().Err(err).Str("request_id", r.Header.Get("X-Request-ID")).Msg("Gateway upstream unavailable")
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		}
	}
	return result, nil
}

func parseUpstream(raw string) (*url.URL, error) {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, errors.New("gateway upstream must be an absolute HTTP(S) URL")
	}
	return target, nil
}
func proxy(target *url.URL) *httputil.ReverseProxy {
	result := httputil.NewSingleHostReverseProxy(target)
	original := result.Director
	result.Director = func(r *http.Request) {
		original(r)
		r.Host = target.Host
		r.Header.Del("X-Internal-Service-Token")
		otel.GetTextMapPropagator().Inject(r.Context(), propagation.HeaderCarrier(r.Header))
	}
	return result
}

// Handler applies correlation, edge headers, a request limit, and deterministic routing.
func (gateway *Gateway) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", requestID)
		r.Header.Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		if r.ContentLength > maxRequestBytes {
			http.Error(w, "request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		gateway.requests.Add(1)
		if isIdentityPath(r.URL.Path) {
			gateway.identity.ServeHTTP(w, r)
		} else {
			gateway.banking.ServeHTTP(w, r)
		}
		log.Info().Str("correlation_id", requestID).Str("path", r.URL.Path).Dur("duration", time.Since(start)).Msg("Gateway request completed")
	})
}

func isIdentityPath(path string) bool {
	switch path {
	case "/register", "/login", "/logout", "/change-password", "/forgot-password", "/reset-password":
		return true
	}
	return false
}

// Metrics exposes minimal Prometheus-compatible counters for edge routing.
func (gateway *Gateway) Metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte("# TYPE gateway_http_requests_total counter\ngateway_http_requests_total " + formatUint(gateway.requests.Load()) + "\n# TYPE gateway_upstream_failures_total counter\ngateway_upstream_failures_total " + formatUint(gateway.failures.Load()) + "\n"))
}
func formatUint(value uint64) string { return strconv.FormatUint(value, 10) }

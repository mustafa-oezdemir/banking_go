// Package notificationapi exposes the private HTTP contract of the Notification service.
package notificationapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mustafa-oezdemir/banking_go/internal/notification"
)

const (
	maxCommandBytes       = 64 << 10
	defaultDeliveryWindow = 12 * time.Second
)

type contextKey string

const requestIDKey contextKey = "notification_request_id"

// Config contains private API authentication and delivery limits.
type Config struct {
	Token           string
	DeliveryTimeout time.Duration
}

// Handler validates commands and delegates provider delivery.
type Handler struct {
	delivery        notification.Delivery
	token           string
	deliveryTimeout time.Duration
}

// New constructs the private Notification HTTP adapter.
func New(delivery notification.Delivery, config Config) (*Handler, error) {
	config.Token = strings.TrimSpace(config.Token)
	if delivery == nil || !delivery.Enabled() {
		return nil, errors.New("notification delivery provider is not configured")
	}
	if len(config.Token) < 32 {
		return nil, errors.New("notification service token must contain at least 32 characters")
	}
	if config.DeliveryTimeout <= 0 {
		config.DeliveryTimeout = defaultDeliveryWindow
	}
	return &Handler{delivery: delivery, token: config.Token, deliveryTimeout: config.DeliveryTimeout}, nil
}

// Routes returns the health and authenticated command endpoints.
func (handler *Handler) Routes() http.Handler {
	router := chi.NewRouter()
	router.Use(handler.requestID)
	router.Use(handler.accessLog)
	router.Use(chimiddleware.Recoverer)
	router.Get("/health", handler.health)
	router.Group(func(private chi.Router) {
		private.Use(handler.authenticate)
		private.Post("/v1/notifications/password-reset", handler.passwordReset)
		private.Post("/v1/notifications/account-activity", handler.accountActivity)
	})
	return router
}

func (handler *Handler) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (handler *Handler) passwordReset(writer http.ResponseWriter, request *http.Request) {
	var command notification.PasswordResetCommand
	if err := decodeCommand(writer, request, &command); err != nil || command.Validate() != nil {
		writeError(writer, http.StatusBadRequest, "invalid notification command")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.deliveryTimeout)
	defer cancel()
	if err := handler.delivery.DeliverPasswordReset(ctx, command); err != nil {
		log.Error().Str("request_id", requestID(request.Context())).Msg("Password reset provider delivery failed")
		writeError(writer, http.StatusBadGateway, "notification delivery failed")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) accountActivity(writer http.ResponseWriter, request *http.Request) {
	var command notification.ActivityCommand
	if err := decodeCommand(writer, request, &command); err != nil || command.Validate() != nil {
		writeError(writer, http.StatusBadRequest, "invalid notification command")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.deliveryTimeout)
	defer cancel()
	if err := handler.delivery.DeliverActivity(ctx, command); err != nil {
		log.Error().Str("request_id", requestID(request.Context())).Str("kind", command.Kind).
			Msg("Account activity delivery failed")
		writeError(writer, http.StatusBadGateway, "notification delivery failed")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(handler.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(handler.token)) != 1 {
			writeError(writer, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (handler *Handler) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		identifier := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if len(identifier) == 0 || len(identifier) > 128 || strings.ContainsAny(identifier, "\r\n") {
			identifier = uuid.NewString()
		}
		writer.Header().Set("X-Request-ID", identifier)
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), requestIDKey, identifier)))
	})
}

func (handler *Handler) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		log.Info().Str("request_id", requestID(request.Context())).Str("method", request.Method).
			Str("path", request.URL.Path).Int("status", recorder.status).Dur("duration", time.Since(started)).
			Msg("Notification request completed")
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func requestID(ctx context.Context) string {
	identifier, _ := ctx.Value(requestIDKey).(string)
	return identifier
}

func decodeCommand(writer http.ResponseWriter, request *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("content type must be application/json")
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxCommandBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		log.Error().Err(err).Msg("Notification response encoding failed")
	}
}

// Command notification-service runs the independently deployable email service.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	emailservice "github.com/mustafa-oezdemir/banking_go/internal/platform/email"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/notificationapi"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Str("service", "notification").Logger()
	if err := godotenv.Load(".env", "../.env"); err != nil {
		log.Debug().Err(err).Msg("Notification .env file not loaded; using process environment")
	}

	delivery := emailservice.NewFromEnvironment()
	deliveryTimeout := durationFromEnvironment("NOTIFICATION_DELIVERY_TIMEOUT", 12*time.Second)
	handler, err := notificationapi.New(delivery, notificationapi.Config{
		Token: os.Getenv("NOTIFICATION_SERVICE_TOKEN"), DeliveryTimeout: deliveryTimeout,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Notification service configuration is invalid")
	}

	port := strings.TrimSpace(os.Getenv("NOTIFICATION_PORT"))
	if port == "" {
		port = "8090"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Info().Str("port", port).Str("provider", delivery.Provider()).Msg("Notification service started")
		serverErrors <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			log.Error().Err(shutdownErr).Msg("Notification service shutdown failed")
		}
		cancel()
	case serveErr := <-serverErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error().Err(serveErr).Msg("Notification service failed")
			stop()
			return
		}
	}
	stop()
	log.Info().Msg("Notification service stopped")
}

func durationFromEnvironment(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		log.Warn().Str("setting", name).Msg("Ignoring invalid duration setting")
		return fallback
	}
	return value
}

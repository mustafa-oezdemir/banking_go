// Command notification-service runs the independently deployable email service.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	emailservice "github.com/mustafa-oezdemir/banking_go/internal/platform/email"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/notificationapi"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/notificationstore"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/observability"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/rabbitmq"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Str("service", "notification").Logger()
	shutdownTelemetry := observability.Init(context.Background(), "notification-service")
	defer func() { _ = shutdownTelemetry(context.Background()) }()
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
	notificationDBURL, err := notificationDatabaseURL()
	if err != nil {
		log.Fatal().Err(err).Msg("Notification idempotency database configuration is invalid")
	}
	database, err := sql.Open("postgres", notificationDBURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Notification idempotency database open failed")
	}
	defer database.Close()
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err = database.PingContext(pingCtx); err != nil {
		pingCancel()
		log.Fatal().Err(err).Msg("Notification idempotency database unavailable")
	}
	pingCancel()
	eventStore, err := notificationstore.New(database)
	if err != nil {
		log.Fatal().Err(err).Msg("Notification idempotency store initialization failed")
	}
	rabbitConfig, err := rabbitmq.ConfigFromEnvironment()
	if err != nil {
		log.Fatal().Err(err).Msg("Notification RabbitMQ configuration is invalid")
	}
	processor, err := rabbitmq.NewProcessor(eventStore, delivery, rabbitConfig.DeliveryWindow)
	if err != nil {
		log.Fatal().Err(err).Msg("Notification event processor initialization failed")
	}
	consumer, err := rabbitmq.NewConsumer(rabbitConfig, processor)
	if err != nil {
		log.Fatal().Err(err).Msg("Notification RabbitMQ consumer initialization failed")
	}
	consumerCtx, stopConsumer := context.WithCancel(context.Background())
	defer stopConsumer()
	go consumer.Run(consumerCtx)

	port := strings.TrimSpace(os.Getenv("NOTIFICATION_PORT"))
	if port == "" {
		port = "8090"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           observability.HTTP("notification-service", handler.Routes()),
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

func notificationDatabaseURL() (string, error) {
	if value := strings.TrimSpace(os.Getenv("NOTIFICATION_DB_URL")); value != "" {
		return value, nil
	}
	host := strings.TrimSpace(os.Getenv("NOTIFICATION_DB_HOST"))
	if host == "" {
		host = strings.TrimSpace(os.Getenv("DB_HOST"))
	}
	if host == "" {
		return "", fmt.Errorf("NOTIFICATION_DB_URL or NOTIFICATION_DB_HOST is required")
	}
	port := firstEnvironment("NOTIFICATION_DB_PORT", "DB_PORT", "5432")
	databaseName := firstEnvironment("NOTIFICATION_DB_NAME", "DB_NAME", "simple_ledger")
	user := firstEnvironment("NOTIFICATION_DB_USER", "DB_USER", "root")
	password := os.Getenv("NOTIFICATION_DB_PASSWORD")
	if password == "" {
		password = os.Getenv("DB_PASSWORD")
	}
	sslMode := firstEnvironment("NOTIFICATION_DB_SSLMODE", "DB_SSLMODE", "disable")
	value := &url.URL{
		Scheme: "postgresql", User: url.UserPassword(user, password),
		Host: net.JoinHostPort(host, port), Path: "/" + databaseName,
	}
	query := value.Query()
	query.Set("sslmode", sslMode)
	value.RawQuery = query.Encode()
	return value.String(), nil
}

func firstEnvironment(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

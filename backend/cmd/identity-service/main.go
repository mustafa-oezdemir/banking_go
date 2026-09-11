// Command identity-service owns credential and session responsibilities.
package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/mustafa-oezdemir/banking_go/internal/platform/bankingclient"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/identityapi"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/identitystore"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/notificationclient"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/observability"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Str("service", "identity").Logger()
	shutdownTelemetry := observability.Init(context.Background(), "identity-service")
	defer func() { _ = shutdownTelemetry(context.Background()) }()
	_ = godotenv.Load(".env", "../.env")
	conn, err := sql.Open("postgres", databaseURL())
	if err != nil {
		log.Fatal().Err(err).Msg("Identity database configuration is invalid")
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	err = conn.PingContext(ctx)
	cancel()
	if err != nil {
		log.Fatal().Err(err).Msg("Identity database unavailable")
	}
	provisioner, err := bankingclient.New(os.Getenv("BANKING_INTERNAL_URL"), os.Getenv("INTERNAL_SERVICE_TOKEN"))
	if err != nil {
		log.Fatal().Err(err).Msg("Identity Banking client configuration is invalid")
	}
	notifier, err := notificationclient.New(notificationclient.Config{BaseURL: os.Getenv("NOTIFICATION_SERVICE_URL"), Token: os.Getenv("NOTIFICATION_SERVICE_TOKEN"), Timeout: 4 * time.Second}, nil, nil)
	if err != nil {
		log.Fatal().Err(err).Msg("Identity notification client configuration is invalid")
	}
	handler, err := identityapi.New(identitystore.New(conn), os.Getenv("JWT_SECRET"), provisioner, notifier)
	if err != nil {
		log.Fatal().Err(err).Msg("Identity service configuration is invalid")
	}
	port := strings.TrimSpace(os.Getenv("IDENTITY_PORT"))
	if port == "" {
		port = "8081"
	}
	server := &http.Server{Addr: ":" + port, Handler: observability.HTTP("identity-service", handler.Routes()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	errs := make(chan error, 1)
	go func() { log.Info().Str("port", port).Msg("Identity service started"); errs <- server.ListenAndServe() }()
	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-stopCtx.Done():
		shutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		_ = server.Shutdown(shutdown)
	case err = <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("Identity service failed")
		}
	}
}

func databaseURL() string {
	host := strings.TrimSpace(os.Getenv("IDENTITY_DB_HOST"))
	if host == "" {
		host = strings.TrimSpace(os.Getenv("DB_HOST"))
	}
	port := strings.TrimSpace(os.Getenv("IDENTITY_DB_PORT"))
	if port == "" {
		port = "5432"
	}
	name := strings.TrimSpace(os.Getenv("IDENTITY_DB_NAME"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("DB_NAME"))
	}
	if name == "" {
		name = "simple_ledger"
	}
	user := strings.TrimSpace(os.Getenv("IDENTITY_DB_USER"))
	if user == "" {
		user = strings.TrimSpace(os.Getenv("DB_USER"))
	}
	if user == "" {
		user = "root"
	}
	password := os.Getenv("IDENTITY_DB_PASSWORD")
	if password == "" {
		password = os.Getenv("DB_PASSWORD")
	}
	return "postgresql://" + user + ":" + password + "@" + host + ":" + port + "/" + name + "?sslmode=" + first(os.Getenv("IDENTITY_DB_SSLMODE"), os.Getenv("DB_SSLMODE"), "disable")
}
func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

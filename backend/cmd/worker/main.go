// Command worker runs scheduled and recurring payment processing independently
// from the HTTP service. It is intended for an opt-in paid Render worker.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"

	"github.com/mustafa-oezdemir/banking_go/internal/db"
	"github.com/mustafa-oezdemir/banking_go/internal/service"
)

func main() {
	if err := godotenv.Load(".env", "../.env"); err != nil {
		log.Debug().Err(err).Msg("Worker .env file not loaded; using process environment")
	}
	connectionURL, err := databaseURL()
	if err != nil {
		log.Fatal().Err(err).Msg("Worker database configuration is invalid")
	}
	connection, err := sql.Open("postgres", connectionURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Worker failed to open database")
	}
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 10*time.Second)
	err = connection.PingContext(pingCtx)
	cancelPing()
	if err != nil {
		if closeErr := connection.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("Worker database close failed after connection error")
		}
		log.Fatal().Err(err).Msg("Worker failed to connect to database")
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("Worker database close failed")
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	paymentService := service.NewPaymentService(db.NewStore(connection), nil)
	run := func() {
		processed, runErr := paymentService.RunDuePayments(ctx, 50)
		if runErr != nil {
			log.Error().Err(runErr).Msg("Scheduled payment cycle failed")
			return
		}
		if processed > 0 {
			log.Info().Int("processed", processed).Msg("Scheduled payment cycle completed")
		}
	}
	run()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Payment worker stopped")
			return
		case <-ticker.C:
			run()
		}
	}
}

func databaseURL() (string, error) {
	for _, key := range []string{"DB_URL", "INTERNAL_DATABASE_URL", "RENDER_DATABASE_URL", "DATABASE_URL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value, nil
		}
	}
	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	if host == "" {
		return "", fmt.Errorf("DB_URL or DB_HOST is required")
	}
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	if user == "" {
		user = "root"
	}
	database := strings.TrimSpace(os.Getenv("DB_NAME"))
	if database == "" {
		database = "simple_ledger"
	}
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	if port == "" {
		port = "5432"
	}
	sslMode := strings.TrimSpace(os.Getenv("DB_SSLMODE"))
	if sslMode == "" {
		sslMode = "disable"
	}
	value := &url.URL{Scheme: "postgresql", User: url.UserPassword(user, os.Getenv("DB_PASSWORD")), Host: net.JoinHostPort(host, port), Path: "/" + database}
	query := value.Query()
	query.Set("sslmode", sslMode)
	value.RawQuery = query.Encode()
	return value.String(), nil
}

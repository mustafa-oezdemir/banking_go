// Command gateway runs the small public edge router for Banking and Identity.
package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/gateway"
	"github.com/mustafa-oezdemir/banking_go/internal/platform/observability"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Str("service", "gateway").Logger()
	shutdownTelemetry := observability.Init(context.Background(), "gateway")
	defer func() { _ = shutdownTelemetry(context.Background()) }()
	_ = godotenv.Load(".env", "../.env")
	handler, err := gateway.New(gateway.Config{IdentityURL: os.Getenv("IDENTITY_SERVICE_URL"), BankingURL: os.Getenv("BANKING_API_URL")})
	if err != nil {
		log.Fatal().Err(err).Msg("Gateway configuration is invalid")
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", http.HandlerFunc(handler.Metrics))
	mux.Handle("/", observability.HTTP("gateway", handler.Handler()))
	port := strings.TrimSpace(os.Getenv("GATEWAY_PORT"))
	if port == "" {
		port = "8082"
	}
	server := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Info().Str("port", port).Msg("Gateway started")
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("Gateway failed")
	}
}

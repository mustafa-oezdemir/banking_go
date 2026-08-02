// Package main wires together the HTTP server, database store, and middleware.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/jwtauth/v5"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	httpSwagger "github.com/swaggo/http-swagger"

	_ "github.com/mustafa-oezdemir/banking_go/docs"
	"github.com/mustafa-oezdemir/banking_go/internal/api"
	"github.com/mustafa-oezdemir/banking_go/internal/db"
	"github.com/mustafa-oezdemir/banking_go/internal/service"
)

func initLogger() {
	// Use millisecond precision in logs so request timing is easy to follow in demos.
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	zlog.Logger = zlog.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Caller().Logger()
	zlog.Info().Msg("Logger initialized")
}

// @title           Double-Entry Bank Ledger API
// @version         1.0
// @description     Production-grade double-entry accounting ledger
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token

func parseAllowedOrigins() []string {
	// Allow explicit runtime configuration; defaults are safe for hosted frontend + local dev.
	origins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if strings.TrimSpace(origins) == "" {
		return []string{
			"https://pehlione-banking.com",
			"https://www.pehlione-banking.com",
			"https://pehlione-banking-frontend.onrender.com",
			"https://banking-go.onrender.com",
			"http://localhost:8080",
			"http://127.0.0.1:8080",
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		}
	}

	parts := strings.Split(origins, ",")
	allowed := make([]string, 0, len(parts))
	for _, origin := range parts {
		// Normalize each origin to avoid accidental whitespace mismatches.
		trimmed := strings.TrimSpace(origin)
		if trimmed != "" {
			allowed = append(allowed, trimmed)
		}
	}

	if len(allowed) == 0 {
		return []string{
			"https://pehlione-banking.com",
			"https://www.pehlione-banking.com",
			"https://pehlione-banking-frontend.onrender.com",
			"https://banking-go.onrender.com",
			"http://localhost:8080",
			"http://127.0.0.1:8080",
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		}
	}

	return allowed
}

func resolveDBURL() string {
	// Prefer explicit, composable settings for local and container deployments.
	// DB_URL remains supported for managed platforms that provide one connection string.
	if connStr := resolveDBURLFromParts(); connStr != "" {
		return connStr
	}

	connStr := strings.TrimSpace(os.Getenv("DB_URL"))

	fallbackVars := []string{"INTERNAL_DATABASE_URL", "RENDER_DATABASE_URL", "DATABASE_URL"}

	if connStr == "" {
		// If DB_URL is absent, try common provider-specific environment variables.
		for _, envVar := range fallbackVars {
			if value := strings.TrimSpace(os.Getenv(envVar)); value != "" {
				return value
			}
		}

		if os.Getenv("RENDER") == "true" {
			zlog.Fatal().Msg(
				"DB_URL is not configured. " +
					"Fix: Render dashboard → your web service → Environment → add DB_URL " +
					"set to the Internal Connection String from your PostgreSQL service.",
			)
		}

		// Default connection string for local development only.
		return "postgresql://root:secret@localhost:5432/simple_ledger?sslmode=disable" // #nosec G101 - Local development default
	}

	lower := strings.ToLower(connStr)
	// Localhost DB URLs are invalid in cloud runtimes; attempt safe fallback automatically.
	isLocalHostURL := strings.Contains(lower, "@localhost:") || strings.Contains(lower, "@127.0.0.1:") || strings.Contains(lower, "@[::1]:")
	if isLocalHostURL {
		for _, envVar := range fallbackVars {
			if value := strings.TrimSpace(os.Getenv(envVar)); value != "" {
				return value
			}
		}
		if os.Getenv("RENDER") == "true" {
			zlog.Fatal().Msg(
				"DB_URL resolves to localhost, which is not valid on Render. " +
					"Fix: Render dashboard → your web service → Environment → update DB_URL " +
					"to the Internal Connection String from your PostgreSQL service.",
			)
		}
	}

	return connStr
}

func resolveDBURLFromParts() string {
	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	if host == "" {
		return ""
	}

	port := envOrDefault("DB_PORT", "5432")
	databaseName := envOrDefault("DB_NAME", "simple_ledger")
	user := envOrDefault("DB_USER", "root")
	password := os.Getenv("DB_PASSWORD")
	sslMode := envOrDefault("DB_SSLMODE", "disable")

	query := url.Values{}
	query.Set("sslmode", sslMode)

	connectionURL := &url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(user, password),
		Host:     net.JoinHostPort(host, port),
		Path:     "/" + databaseName,
		RawQuery: query.Encode(),
	}

	return connectionURL.String()
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func loadEnvironment() error {
	// Render injects configuration as environment variables. A .env file is a
	// local-development convenience and is intentionally absent in production.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("RENDER")), "true") {
		return nil
	}

	var lastErr error
	for _, envPath := range []string{".env", "../.env"} {
		err := godotenv.Load(envPath)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
}

func main() {
	// Capture startup time so health endpoint can report uptime.
	startTime := time.Now()

	initLogger()

	if err := loadEnvironment(); err != nil {
		zlog.Info().Err(err).Msg("No .env file found; using system environment")
	}

	if err := api.InitTokenAuthFromEnv(); err != nil {
		zlog.Fatal().Err(err).Msg("Failed to initialize JWT auth")
	}

	// Build DB connection string and validate connectivity before serving traffic.
	connStr := resolveDBURL()
	if strings.Contains(connStr, "@localhost:") || strings.Contains(connStr, "@127.0.0.1:") || strings.Contains(connStr, "@[::1]:") {
		zlog.Warn().Msg("Using localhost DB_URL; this is only valid for local development")
	}
	dbConn, err := sql.Open("postgres", connStr)
	if err != nil {
		zlog.Fatal().Err(err).Msg("Failed to open DB connection")
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer pingCancel()
	if err := dbConn.PingContext(pingCtx); err != nil {
		zlog.Fatal().Err(err).Msg("Failed to connect to DB")
	}
	zlog.Info().Msg("Database connectivity verified")

	defer func() {
		if closeErr := dbConn.Close(); closeErr != nil {
			zlog.Error().Err(closeErr).Msg("Failed to close DB connection")
		}
	}()

	store := db.NewStore(dbConn)
	ledgerSvc := service.NewLedgerService(store)
	eventHub := service.NewEventHub()
	paymentSvc := service.NewPaymentService(store, eventHub)

	// Wire HTTP handlers with service and persistence dependencies.
	h := api.NewHandlerWithPayments(ledgerSvc, paymentSvc, store)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(api.SecurityHeaders)
	r.Use(api.NewIPRateLimiter(300, time.Minute))
	r.Use(api.LimitRequestBody(1 << 20))
	r.Use(api.RequireJSON)
	allowedOrigins := parseAllowedOrigins()
	r.Use(api.CSRFProtection(allowedOrigins))

	// CORS middleware for separate frontend deployments and local development.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Protection", "Idempotency-Key", "Last-Event-ID"},
		ExposedHeaders:   []string{"Link", "Idempotent-Replayed"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Attach request metadata to logs for traceability during debugging.
			reqID := middleware.GetReqID(r.Context())
			zlog.Info().Str("request_id", reqID).Str("path", r.URL.Path).Msg("Request received")
			next.ServeHTTP(w, r)
		})
	})

	// Public routes
	authRateLimit := api.NewIPRateLimiter(10, time.Minute)
	r.With(authRateLimit).Post("/register", h.Register)
	r.With(authRateLimit).Post("/login", h.Login)
	r.Post("/logout", h.Logout)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		// Health returns service liveness plus lightweight runtime metadata.
		zlog.Info().Msg("Health check requested")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"version": "0.1.0",
			"uptime":  time.Since(startTime).String(),
		}); err != nil {
			zlog.Error().Err(err).Msg("Failed to encode health check response")
		}
	})

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DeepLinking(true),
	))
	// Protected routes
	r.Group(func(r chi.Router) {
		// Apply JWT verification only to protected business endpoints.
		r.Use(jwtauth.Verifier(api.TokenAuth))
		r.Use(jwtauth.Authenticator(api.TokenAuth))

		r.Get("/session", h.Session)
		r.Post("/accounts", h.CreateAccount)
		r.Get("/accounts", h.ListAccounts)
		r.Get("/accounts/{id}", h.GetAccount)
		r.Put("/accounts/{id}", h.UpdateAccount)
		r.Delete("/accounts/{id}", h.DeleteAccount)
		r.Post("/accounts/{id}/deposit", h.Deposit)
		r.Post("/accounts/{id}/withdraw", h.Withdraw)
		r.Post("/transfers", h.Transfer)
		r.Get("/accounts/{id}/entries", h.GetEntries)
		r.Get("/accounts/{id}/transactions", h.ListAccountTransactions)
		r.Get("/accounts/{id}/reconcile", h.ReconcileAccount)
		r.Get("/transactions/{id}", h.GetTransactions)

		vopRateLimit := api.NewIPRateLimiter(30, time.Minute)
		paymentRateLimit := api.NewIPRateLimiter(20, time.Minute)
		r.With(vopRateLimit).Post("/payees/verify", h.VerifyPayee)
		r.With(paymentRateLimit).Post("/payments", h.CreatePayment)
		r.Get("/payments", h.ListPayments)
		r.Get("/payments/{id}", h.GetPayment)
		r.With(paymentRateLimit).Post("/payments/{id}/confirm", h.ConfirmPayment)
		r.Post("/payments/{id}/cancel", h.CancelPayment)
		r.Post("/standing-orders", h.CreateStandingOrder)
		r.Get("/standing-orders", h.ListStandingOrders)
		r.Patch("/standing-orders/{id}", h.UpdateStandingOrder)
		r.Delete("/standing-orders/{id}", h.DeleteStandingOrder)
		r.Get("/beneficiaries", h.ListBeneficiaries)
		r.Post("/beneficiaries", h.CreateBeneficiary)
		r.Get("/events", h.Events)
	})

	// Free-demo profile: process due orders only while the web service is awake.
	// Set ENABLE_IN_PROCESS_SCHEDULER=false when running the paid background
	// worker profile to avoid unnecessary polling (SKIP LOCKED remains safe).
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("ENABLE_IN_PROCESS_SCHEDULER")), "false") {
		go func() {
			ctx := context.Background()
			if processed, workerErr := paymentSvc.RunDuePayments(ctx, 25); workerErr != nil {
				zlog.Error().Err(workerErr).Msg("Initial scheduled-payment scan failed")
			} else if processed > 0 {
				zlog.Info().Int("processed", processed).Msg("Initial scheduled payments processed")
			}
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if processed, workerErr := paymentSvc.RunDuePayments(ctx, 25); workerErr != nil {
					zlog.Error().Err(workerErr).Msg("Scheduled-payment scan failed")
				} else if processed > 0 {
					zlog.Info().Int("processed", processed).Msg("Scheduled payments processed")
				}
			}
		}()
	}

	port := os.Getenv("PORT")
	if port == "" {
		// Default port for local development when PORT is not injected.
		port = "8080"
	}

	// Configure HTTP server with timeouts for security
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	zlog.Info().Str("port", port).Msg("Starting server")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		zlog.Fatal().Err(err).Msg("Server failed to start")
	}
}

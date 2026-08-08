// Command server is the StageX participant API entrypoint. It wires the
// platform (config, logger, database, auth) to every domain module and starts
// an HTTP server with graceful shutdown.
//
// The wiring here is deliberately explicit — you can read this one file and see
// exactly how a request flows: middleware → route → controller → service →
// repository → Postgres.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iig/stagex/backend/internal/auth"
	"github.com/iig/stagex/backend/internal/catalog"
	"github.com/iig/stagex/backend/internal/certificates"
	"github.com/iig/stagex/backend/internal/config"
	"github.com/iig/stagex/backend/internal/coupons"
	"github.com/iig/stagex/backend/internal/events"
	"github.com/iig/stagex/backend/internal/feedback"
	"github.com/iig/stagex/backend/internal/myevents"
	"github.com/iig/stagex/backend/internal/notifications"
	"github.com/iig/stagex/backend/internal/payments"
	"github.com/iig/stagex/backend/internal/people"
	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/crypto"
	"github.com/iig/stagex/backend/internal/platform/database"
	"github.com/iig/stagex/backend/internal/platform/logger"
	"github.com/iig/stagex/backend/internal/platform/middleware"
	"github.com/iig/stagex/backend/internal/registrations"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.LogLevel)

	if err := cfg.Validate(); err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// --- Platform: database ---
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		log.Error("migrations failed", "error", err)
		os.Exit(1)
	}
	log.Info("database ready — migrations applied")

	// --- Platform: auth + crypto ---
	tokens := platauth.NewManager(cfg.JWTSecret, cfg.JWTTTL)
	cipher, err := crypto.NewCipher(cfg.AadhaarKey)
	if err != nil {
		log.Error("cipher init failed", "error", err)
		os.Exit(1)
	}
	protect := tokens.Middleware // shorthand for the protected-route middleware

	// --- Domain wiring: repository → service → controller ---
	mux := http.NewServeMux()

	// Auth
	authSvc := auth.NewService(
		auth.NewPgFamilyRepository(pool),
		auth.NewPgOTPRepository(pool),
		tokens, cfg.OTPTTL, cfg.AppEnv != "production",
	)
	auth.RegisterRoutes(mux, auth.NewController(authSvc))

	// Catalog (admin master data, read-only)
	catalog.RegisterRoutes(mux, catalog.NewController(catalog.NewService(pool)))

	// Events
	events.RegisterRoutes(mux, events.NewController(events.NewService(events.NewPgRepository(pool))))

	// Coupons
	couponSvc := coupons.NewService(coupons.NewPgRepository(pool))
	coupons.RegisterRoutes(mux, coupons.NewController(couponSvc))

	// People (protected)
	peopleSvc := people.NewService(people.NewPgRepository(pool), cipher)
	people.RegisterRoutes(mux, people.NewController(peopleSvc), protect)

	// Registrations (protected) — reuses the coupon service for pricing
	regSvc := registrations.NewService(registrations.NewPgRepository(pool), couponSvc)
	registrations.RegisterRoutes(mux, registrations.NewController(regSvc), protect)

	// Payments (protected) — mock provider locally
	paySvc := payments.NewService(payments.NewPgRepository(pool), payments.MockProvider{})
	payments.RegisterRoutes(mux, payments.NewController(paySvc), protect)

	// My Events / Certificates / Feedback (protected)
	myevents.RegisterRoutes(mux, myevents.NewController(myevents.NewService(pool)), protect)
	certificates.RegisterRoutes(mux, certificates.NewController(certificates.NewService(pool)), protect)
	feedback.RegisterRoutes(mux, feedback.NewController(feedback.NewService(pool)), protect)
	notifications.RegisterRoutes(mux, notifications.NewController(notifications.NewService(pool)), protect)

	// Health check (public) for docker-compose and load balancers.
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"degraded"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// --- Middleware chain (outermost first) ---
	handler := middleware.Chain(mux,
		middleware.Recover(log),
		middleware.RequestLogger(log),
		middleware.CORS(cfg.CORSAllowOrigin),
	)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// --- Start + graceful shutdown ---
	go func() {
		log.Info("stagex api listening", "port", cfg.HTTPPort, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
}

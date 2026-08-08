// Command server is the StageX Admin API entrypoint. It is a separate service
// from the participant API (different binary and port) but connects to the SAME
// Postgres database. It exposes two role-gated, non-overlapping route trees:
//   /admin/ops/*    — Operational Admin (master data, oversight, participants)
//   /admin/event/*  — Event Admin (own events, categories, participants)
// Super Admin is intentionally out of scope.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iig/stagex/adminbackend/internal/config"
	"github.com/iig/stagex/adminbackend/internal/eventadmin"
	"github.com/iig/stagex/adminbackend/internal/identity"
	"github.com/iig/stagex/adminbackend/internal/operationaladmin"
	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/database"
	"github.com/iig/stagex/adminbackend/internal/platform/logger"
	"github.com/iig/stagex/adminbackend/internal/platform/middleware"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.LogLevel)
	if err := cfg.Validate(); err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

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

	tokens := auth.NewManager(cfg.JWTSecret, cfg.JWTTTL)

	// Identity + mock-admin bootstrap so the console is testable immediately.
	idSvc := identity.NewService(pool, tokens)
	if err := idSvc.Bootstrap(ctx); err != nil {
		log.Error("admin bootstrap failed", "error", err)
		os.Exit(1)
	}
	log.Info("database ready — admin migrations applied, mock admins seeded")

	// Role guards keep the two route trees disjoint.
	opsGuard := func(h http.Handler) http.Handler { return tokens.RequireRole(auth.RoleOps, h) }
	eventGuard := func(h http.Handler) http.Handler { return tokens.RequireRole(auth.RoleEvent, h) }

	mux := http.NewServeMux()
	identity.NewController(idSvc, tokens).RegisterRoutes(mux)
	operationaladmin.RegisterRoutes(mux, pool, opsGuard)
	eventadmin.RegisterRoutes(mux, pool, eventGuard, cfg.MediaRoot, cfg.MediaPublicBaseURL)

	// Serve uploaded event media publicly so participants can view it.
	if err := os.MkdirAll(cfg.MediaRoot, 0o755); err != nil {
		log.Error("could not create media root", "error", err)
		os.Exit(1)
	}
	mux.Handle("GET /media/", http.StripPrefix("/media/", http.FileServer(http.Dir(cfg.MediaRoot))))

	mux.HandleFunc("GET /admin/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"degraded"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

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

	go func() {
		log.Info("stagex admin api listening", "port", cfg.HTTPPort, "env", cfg.AppEnv)
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

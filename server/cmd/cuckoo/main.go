// Command cuckoo runs the Cuckoo hub: the chat server that agent backends and
// the mobile app both connect to.
//
// It is a single binary with its migrations embedded, so deploying it — or
// self-hosting a private hub — is copying one file and setting environment
// variables.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/api"
	"github.com/Saieshwar5/cuckoo/server/internal/api/middleware"
	"github.com/Saieshwar5/cuckoo/server/internal/auth"
	"github.com/Saieshwar5/cuckoo/server/internal/config"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// A .env file is a convenience for local development; every deployment sets
	// real environment variables, so its absence is not an error.
	_ = godotenv.Load(".env")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg)
	slog.SetDefault(log)
	log.Info("starting cuckoo", "env", cfg.Env, "hub_domain", cfg.HubDomain, "addr", cfg.HTTPAddr)

	authenticator, err := newAuthenticator(cfg)
	if err != nil {
		return err
	}

	// Signals cancel this context, which unwinds startup and then triggers a
	// graceful shutdown of the running server.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := store.Migrate(ctx, cfg.DatabaseURL, log); err != nil {
		return err
	}

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	redisClient, err := newRedis(cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()

	agentService := agents.New(db)
	conversationService := conversations.New(db,
		conversations.WithLimiter(ratelimit.NewRedis(redisClient, "cuckoo:")))
	deliveryService := delivery.New(db, conversationService)

	router := api.NewRouter(api.Deps{
		Logger:        log,
		UserAuth:      authenticator,
		AgentAuth:     auth.NewBinding(agentService),
		Users:         users.New(db),
		Agents:        agentService,
		Conversations: conversationService,
		Delivery:      deliveryService,
		Health: map[string]api.HealthCheck{
			"postgres": db.Ping,
			"redis":    func(ctx context.Context) error { return redisClient.Ping(ctx).Err() },
		},
	})

	// The delivery worker runs beside the HTTP server in the same process:
	// one binary is the whole hub. It stops with the same signal and is
	// waited for, so an attempt in flight is recorded before exit.
	worker := delivery.NewWorker(db, deliveryService, agentService, delivery.Options{
		AllowLoopback: cfg.IsDev(),
		Logger:        log,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Run(ctx)
	}()

	err = serve(ctx, cfg, log, router)
	stop()
	wg.Wait()
	return err
}

// serve runs the HTTP server until the context is cancelled, then lets
// in-flight requests finish before returning.
func serve(ctx context.Context, cfg config.Config, log *slog.Logger, handler http.Handler) error {
	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: handler,

		// Long-lived connections carry the agent protocol and the app's live
		// updates, so there is deliberately no write timeout here; per-request
		// deadlines belong to the handlers that need them.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("listen: %w", err)
			return
		}
		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		log.Info("shutting down", "timeout", cfg.ShutdownTimeout)
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Info("stopped cleanly")
	return nil
}

// newAuthenticator selects how callers are identified.
//
// Production has no authenticator yet, and startup fails rather than falling
// back to the development header. A security bypass that a misconfiguration can
// reach is not a bypass, it is a vulnerability with a scheduled release date.
func newAuthenticator(cfg config.Config) (middleware.Authenticator, error) {
	if !cfg.IsDev() {
		return nil, fmt.Errorf(
			"no authenticator available for CUCKOO_ENV=%s: real authentication is not implemented yet, "+
				"so only CUCKOO_ENV=dev can start", cfg.Env)
	}
	return auth.NewDev(), nil
}

func newRedis(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return redis.NewClient(opts), nil
}

// newLogger writes JSON in every environment: the same lines a developer reads
// locally are the ones a log search reads in production.
func newLogger(cfg config.Config) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
}

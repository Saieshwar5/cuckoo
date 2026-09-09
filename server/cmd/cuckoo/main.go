// Command cuckoo runs the Cuckoo hub: the chat server that agent backends and
// the mobile app both connect to.
//
// It is a single binary with its migrations embedded, so deploying it is
// copying one file and setting environment variables.
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
	"github.com/Saieshwar5/cuckoo/server/internal/apikeys"
	"github.com/Saieshwar5/cuckoo/server/internal/auth"
	"github.com/Saieshwar5/cuckoo/server/internal/blobs"
	"github.com/Saieshwar5/cuckoo/server/internal/config"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/mail"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/retention"
	"github.com/Saieshwar5/cuckoo/server/internal/signin"
	"github.com/Saieshwar5/cuckoo/server/internal/signing"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// version is stamped at build time (-ldflags "-X main.version=…") and shown on
// /healthz, so what is running on a machine is never a guess. "dev" means a
// plain `go build` or `go run`.
var version = "dev"

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
	log.Info("starting cuckoo", "version", version, "env", cfg.Env, "hub_domain", cfg.HubDomain, "addr", cfg.HTTPAddr, "mail", cfg.Mail)

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

	// Live updates travel over one Redis channel so a socket on this
	// instance hears what happened on another. The hub must be listening
	// before anything can be announced.
	bus := realtime.NewRedisBus(redisClient, "cuckoo:client")
	hub := realtime.NewHub(log)
	if err := hub.Start(ctx, bus); err != nil {
		return err
	}

	limits := ratelimit.NewRedis(redisClient, "cuckoo:")

	// Every finished message carries the hub's mark; see the signing
	// package for why. The development key proves nothing and says so.
	signer, err := signing.New(cfg.SigningKey)
	if err != nil {
		return err
	}
	if cfg.DevSigningKey {
		log.Warn("messages are signed with the development key; set CUCKOO_SIGNING_KEY")
	}

	agentService := agents.New(db, agents.WithPublisher(bus))
	conversationService := conversations.New(db,
		conversations.WithLimiter(limits),
		conversations.WithPublisher(bus),
		conversations.WithStreams(conversations.NewStreamStore(redisClient, "cuckoo:")),
		conversations.WithSigner(signer))
	deliveryService := delivery.New(db, conversationService)
	userService := users.New(db)
	pairingService := pairing.New(db, agentService, conversationService, userService, cfg.PublicURL)
	// Sign-in is built after pairing, whose welcomer it calls for every new
	// account; the session authenticator is built after sign-in, which it
	// resolves tokens with. Order matters here and nothing checks it but
	// the first request.
	mailer, err := newMailer(cfg, log)
	if err != nil {
		return err
	}
	signinService := signin.New(db, mailer, signin.WithLimiter(limits),
		signin.WithWelcome(pairingService.Welcomer(cfg.WelcomeHandle)))
	userAuth := newUserAuthenticator(cfg, signinService)
	apiKeyService := apikeys.New(db)

	blobStore, err := newBlobStore(ctx, cfg, log)
	if err != nil {
		return err
	}
	mediaService := media.New(db, blobStore, media.WithLimiter(limits), media.WithLogger(log))
	retentionService := retention.New(db, blobStore, retention.Policy{
		MessageAge:       cfg.Retention.MessageAge,
		UserMediaBudget:  cfg.Retention.UserMediaBudget,
		AgentMediaBudget: cfg.Retention.AgentMediaBudget,
		DryRun:           cfg.Retention.DryRun,
	}, retention.WithLogger(log))
	log.Info("retention", "message_age", cfg.Retention.MessageAge,
		"user_media_budget", cfg.Retention.UserMediaBudget, "agent_media_budget", cfg.Retention.AgentMediaBudget,
		"dry_run", cfg.Retention.DryRun)

	router := api.NewRouter(api.Deps{
		Logger:        log,
		UserAuth:      userAuth,
		AgentAuth:     auth.NewBinding(agentService),
		MgmtAuth:      auth.NewKeyOrUser(apiKeyService, userAuth),
		Users:         userService,
		SignIn:        signinService,
		Agents:        agentService,
		Conversations: conversationService,
		Delivery:      deliveryService,
		Pairing:       pairingService,
		APIKeys:       apiKeyService,
		Media:         mediaService,
		PublicURL:     cfg.PublicURL,
		Hub:           hub,
		Bus:           bus,
		CORSOrigins:   cfg.CORSOrigins,
		Retention:     retentionService,
		Version:       version,
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
	wg.Add(4)
	go func() {
		defer wg.Done()
		worker.Run(ctx)
	}()
	// Streams an agent stops writing to are finished for it, so a bubble
	// never spins forever.
	go func() {
		defer wg.Done()
		conversationService.RunStreamSweeper(ctx, 5*time.Second, log)
	}()
	// Files picked and never sent do not stay on the disk forever.
	go func() {
		defer wg.Done()
		mediaService.RunSweeper(ctx, time.Hour, 24*time.Hour)
	}()
	// The hub is a window, not an archive: what is past it goes.
	go func() {
		defer wg.Done()
		retentionService.Run(ctx, 6*time.Hour)
	}()

	err = serve(ctx, cfg, log, router)
	stop()
	wg.Wait()
	// Sockets are hijacked connections the HTTP shutdown does not wait for.
	hub.Close()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer waitCancel()
	if werr := hub.Wait(waitCtx); werr != nil {
		log.Warn("some socket connections did not finish before shutdown", "error", werr)
	}
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

// newUserAuthenticator selects how people are identified.
//
// Everywhere, a session token. In development, also the header that names
// a user outright, so curl works without signing in. The header is never
// wired outside development: a bypass a misconfiguration can reach is not a
// bypass, it is a vulnerability with a scheduled release date.
func newUserAuthenticator(cfg config.Config, s *signin.Service) middleware.Authenticator {
	session := auth.NewSession(s)
	if cfg.IsDev() {
		return auth.NewDevOrSession(auth.NewDev(), session)
	}
	return session
}

// newMailer picks where sign-in codes go. Console prints them to the log,
// which is right in development and on a hub whose only user reads its log;
// smtp sends them through a provider.
// newBlobStore opens the store uploaded files live in. Both backends use the
// same keys, so which one is underneath is a setting and a copy rather than a
// migration; the log line says which, because "where are the files" should
// never need a look at .env.
func newBlobStore(ctx context.Context, cfg config.Config, log *slog.Logger) (blobs.Store, error) {
	if cfg.Blobs == config.BlobsS3 {
		store, err := blobs.NewS3(ctx, cfg.S3.Bucket, cfg.S3.Region)
		if err != nil {
			return nil, err
		}
		log.Info("media storage ready", "backend", config.BlobsS3, "bucket", store.Bucket())
		return store, nil
	}
	store, err := blobs.NewDisk(cfg.MediaDir)
	if err != nil {
		return nil, err
	}
	log.Info("media storage ready", "backend", config.BlobsDisk, "dir", store.Root())
	return store, nil
}

func newMailer(cfg config.Config, log *slog.Logger) (mail.Mailer, error) {
	if cfg.Mail != config.MailSMTP {
		log.Info("sign-in codes go to the log", "mail", cfg.Mail)
		return mail.NewConsole(log), nil
	}
	m, err := mail.NewSMTP(mail.SMTPConfig{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
		FromName: cfg.SMTP.FromName,
	})
	if err != nil {
		return nil, err
	}
	log.Info("sending mail", "host", cfg.SMTP.Host, "port", cfg.SMTP.Port, "from", cfg.SMTP.From)
	return m, nil
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

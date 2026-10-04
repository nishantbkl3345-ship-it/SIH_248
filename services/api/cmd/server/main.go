// Command server runs the FOGLINE HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"fogline/api/internal/auth"
	"fogline/api/internal/config"
	"fogline/api/internal/platform/database"
	"fogline/api/internal/platform/logger"
	"fogline/api/internal/repository/gormrepo"
	"fogline/api/internal/runtime"
	"fogline/api/internal/service"
	"fogline/api/internal/transport/httpapi"
)

const shutdownTimeout = 15 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	// A missing .env is normal outside local development.
	_ = godotenv.Load()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	log := logger.New(os.Stdout, cfg.LogLevel, cfg.IsProduction())
	slog.SetDefault(log)
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.DBAutoMigrate {
		if err := database.MigrateUp(cfg.DatabaseURL); err != nil {
			return err
		}
		log.Info("migrations applied")
	}

	db, err := database.Open(ctx, cfg.DatabaseURL, !cfg.IsProduction())
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	users := gormrepo.NewUserRepository(db)
	hasher := auth.NewPasswordHasher(auth.DefaultBcryptCost)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTTTL)

	authSvc, err := service.NewAuthService(users, hasher, tokens, cfg.AllowRegistration)
	if err != nil {
		return err
	}
	userSvc := service.NewUserService(users, hasher)
	scenarioRepo := gormrepo.NewScenarioRepository(db)
	scenarioSvc := service.NewScenarioService(scenarioRepo)

	sessionRepo := gormrepo.NewSessionRepository(db)
	live := runtime.NewManager(runtime.Options{
		Store:     service.NewSessionStore(sessionRepo),
		Logger:    log,
		TickEvery: 250 * time.Millisecond,
	})
	defer live.Shutdown()
	sessionSvc := service.NewSessionService(sessionRepo, scenarioRepo, live)

	// Engines live in memory, so an exercise that was running when the
	// process last stopped cannot be continued.
	if n, err := sessionSvc.RecoverInterrupted(ctx); err != nil {
		return fmt.Errorf("recover interrupted sessions: %w", err)
	} else if n > 0 {
		log.Warn("closed sessions interrupted by a restart", "count", n)
	}

	if cfg.BootstrapAdminEmail != "" {
		created, err := userSvc.EnsureAdmin(ctx, cfg.BootstrapAdminEmail, cfg.BootstrapAdminPassword)
		if err != nil {
			return fmt.Errorf("bootstrap admin: %w", err)
		}
		if created {
			log.Info("bootstrap admin created", "email", cfg.BootstrapAdminEmail)
		}
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Logger:         log,
			Auth:           authSvc,
			Users:          userSvc,
			Scenarios:      scenarioSvc,
			Sessions:       sessionSvc,
			Ping:           sqlDB.PingContext,
			CookieSecure:   cfg.CookieSecure,
			AllowedOrigins: cfg.CORSAllowedOrigins,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		stop() // a second signal now terminates immediately
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	// Streams never finish on their own; close them so Shutdown can.
	live.Shutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Info("server stopped")
	return nil
}

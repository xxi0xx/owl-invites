package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"

	"github.com/yannkr/openrsvp/internal/buildinfo"
	"github.com/yannkr/openrsvp/internal/config"
	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/server"
)

func main() {
	if handled, err := buildinfo.RunVersionCommand(os.Args[1:], os.Stdout); handled {
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "openrsvp:", err)
			os.Exit(1)
		}
		return
	}

	// --- Logger ---
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		With().
		Timestamp().
		Caller().
		Logger()

	// --- Config ---
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}

	if cfg.Env == "production" {
		logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}

	build := buildinfo.Current()
	logger.Info().
		Str("env", cfg.Env).
		Str("port", cfg.Port).
		Str("db_driver", cfg.DBDriver).
		Str("version", build.Version).
		Str("commit", build.Commit).
		Str("build_state", build.BuildState).
		Msg("starting openrsvp")

	// --- Database ---
	db, err := database.New(cfg)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer func() { _ = db.Close() }()

	logger.Info().Str("dialect", db.Dialect()).Msg("database connected")

	// --- Migrations ---
	if err := database.RunMigrations(db); err != nil {
		logger.Fatal().Err(err).Msg("failed to run migrations")
	}
	logger.Info().Msg("migrations applied")

	// --- Server ---
	srv := server.New(cfg, db, logger)

	// --- Graceful shutdown ---
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(ctx); err != nil {
		logger.Fatal().Err(err).Msg("server error")
	}
}

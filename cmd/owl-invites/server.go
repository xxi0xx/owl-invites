package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"

	"github.com/xxi0xx/owl-invites/internal/buildinfo"
	"github.com/xxi0xx/owl-invites/internal/config"
	"github.com/xxi0xx/owl-invites/internal/database"
	"github.com/xxi0xx/owl-invites/internal/server"
)

// runServer is the default owl-invites command. Operator subcommands share the
// same executable but do not start the HTTP service.
func runServer(stderr io.Writer) error {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: stderr}).
		With().
		Timestamp().
		Caller().
		Logger()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Env == "production" {
		logger = zerolog.New(stderr).With().Timestamp().Logger()
	}
	if cfg.DBDSNWarning != "" {
		logger.Warn().Msg(cfg.DBDSNWarning)
	}

	build := buildinfo.Current()
	logger.Info().
		Str("env", cfg.Env).
		Str("port", cfg.Port).
		Str("db_driver", cfg.DBDriver).
		Str("version", build.Version).
		Str("commit", build.Commit).
		Str("build_state", build.BuildState).
		Msg("starting Owl Invites")

	db, err := database.New(cfg)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() { _ = db.Close() }()

	logger.Info().Str("dialect", db.Dialect()).Msg("database connected")
	if err := database.RunMigrations(db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	logger.Info().Msg("migrations applied")

	srv := server.New(cfg, db, logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Start(ctx); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

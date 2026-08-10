package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/yannkr/openrsvp/internal/admincli"
	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/buildinfo"
	"github.com/yannkr/openrsvp/internal/config"
	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/instanceconfig"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "owl-invites:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if handled, err := buildinfo.RunVersionCommand(args, stdout); handled {
		return err
	}
	if len(args) < 2 || args[0] != "admin" || args[1] != "promote" {
		return errors.New("usage: owl-invites <version [--json] | admin promote --email user@example.com>")
	}
	flags := flag.NewFlagSet("admin promote", flag.ContinueOnError)
	flags.SetOutput(stderr)
	email := flags.String("email", "", "existing user email to activate and promote")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if *email == "" {
		return errors.New("--email is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	db, err := database.New(cfg)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := database.RunMigrations(db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	service := admincli.NewService(auth.NewStore(db), instanceconfig.NewStore(db))
	user, err := service.PromoteAdmin(context.Background(), *email)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "promoted %s to instance administrator\n", user.Email)
	return nil
}

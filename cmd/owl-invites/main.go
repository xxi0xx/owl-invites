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
	backupops "github.com/yannkr/openrsvp/internal/backup"
	"github.com/yannkr/openrsvp/internal/buildinfo"
	"github.com/yannkr/openrsvp/internal/config"
	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/instanceconfig"
	"github.com/yannkr/openrsvp/internal/secretkey"
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
	if len(args) >= 1 && args[0] == "migrate" {
		return runMigrate(args[1:], stdout)
	}
	if len(args) >= 1 && args[0] == "backup" {
		return runBackup(args[1:], stdout, stderr)
	}
	if len(args) == 2 && args[0] == "secret" && args[1] == "fingerprint" {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		_, err = fmt.Fprintln(stdout, secretkey.Fingerprint(cfg.InvitationSecretKey))
		return err
	}
	if len(args) < 2 || args[0] != "admin" || args[1] != "promote" {
		return errors.New("usage: owl-invites <version [--json] | secret fingerprint | migrate <status|version|up> | backup <sqlite|verify|restore> | admin promote --email user@example.com>")
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

func runMigrate(args []string, stdout io.Writer) error {
	if len(args) != 1 || (args[0] != "status" && args[0] != "version" && args[0] != "up") {
		return errors.New("usage: owl-invites migrate <status|version|up>")
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

	switch args[0] {
	case "status":
		status, statusErr := database.ReadMigrationStatus(db)
		if statusErr != nil {
			return fmt.Errorf("read migration status: %w", statusErr)
		}
		_, err = fmt.Fprintf(stdout, "current=%d\nlatest=%d\ndirty=%t\npending=%t\n", status.Current, status.Latest, status.Dirty, status.Pending)
		return err
	case "version":
		status, statusErr := database.ReadMigrationStatus(db)
		if statusErr != nil {
			return fmt.Errorf("read migration version: %w", statusErr)
		}
		_, err = fmt.Fprintf(stdout, "%d\n", status.Current)
		return err
	case "up":
		result, migrateErr := database.MigrateUp(db)
		if migrateErr != nil {
			return fmt.Errorf("apply migrations: %w", migrateErr)
		}
		if result.Before.Current == result.After.Current && !result.Before.Dirty {
			_, err = fmt.Fprintf(stdout, "schema already at version %d\n", result.After.Current)
			return err
		}
		_, err = fmt.Fprintf(stdout, "migrated schema from %d to %d\n", result.Before.Current, result.After.Current)
		return err
	}
	return nil
}

func runBackup(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: owl-invites backup <sqlite|verify|restore>")
	}
	switch args[0] {
	case "verify":
		if len(args) != 2 {
			return errors.New("usage: owl-invites backup verify BACKUP")
		}
		manifest, err := backupops.VerifySQLite(context.Background(), args[1])
		if err != nil {
			return fmt.Errorf("verify backup: %w", err)
		}
		_, err = fmt.Fprintf(stdout, "backup verified\nschema_version=%d\ndatabase_sha256=%s\nsecret_fingerprint=%s\n", manifest.Source.SchemaVersion, manifest.Database.SHA256, manifest.SecretFingerprint)
		return err
	case "sqlite":
		flags := flag.NewFlagSet("backup sqlite", flag.ContinueOnError)
		flags.SetOutput(stderr)
		output := flags.String("output", "", "new backup bundle directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *output == "" || flags.NArg() != 0 {
			return errors.New("usage: owl-invites backup sqlite --output DIRECTORY")
		}
		cfg, db, err := openConfiguredDatabase()
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		if cfg.DBDriver != "sqlite" {
			return errors.New("backup sqlite requires DB_DRIVER=sqlite")
		}
		manifest, err := backupops.CreateSQLite(context.Background(), db, cfg.UploadsDir, *output, cfg.InvitationSecretKey)
		if err != nil {
			return fmt.Errorf("create SQLite backup: %w", err)
		}
		_, err = fmt.Fprintf(stdout, "backup created at %s\ndatabase_sha256=%s\nsecret_fingerprint=%s\n", *output, manifest.Database.SHA256, manifest.SecretFingerprint)
		return err
	case "restore":
		if len(args) < 2 {
			return errors.New("usage: owl-invites backup restore BACKUP --database NEW_PATH --uploads NEW_DIRECTORY")
		}
		bundle := args[1]
		flags := flag.NewFlagSet("backup restore", flag.ContinueOnError)
		flags.SetOutput(stderr)
		databasePath := flags.String("database", "", "new SQLite database path")
		uploadsPath := flags.String("uploads", "", "new uploads directory")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if *databasePath == "" || *uploadsPath == "" || flags.NArg() != 0 {
			return errors.New("usage: owl-invites backup restore BACKUP --database NEW_PATH --uploads NEW_DIRECTORY")
		}
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		if cfg.DBDriver != "sqlite" {
			return errors.New("backup restore requires DB_DRIVER=sqlite")
		}
		manifest, err := backupops.RestoreSQLite(context.Background(), bundle, *databasePath, *uploadsPath, cfg.InvitationSecretKey)
		if err != nil {
			return fmt.Errorf("restore SQLite backup: %w", err)
		}
		_, err = fmt.Fprintf(stdout, "backup restored\nschema_version=%d\nsecret_fingerprint=%s\n", manifest.Source.SchemaVersion, manifest.SecretFingerprint)
		return err
	default:
		return errors.New("usage: owl-invites backup <sqlite|verify|restore>")
	}
}

func openConfiguredDatabase() (*config.Config, database.DB, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load configuration: %w", err)
	}
	db, err := database.New(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("connect database: %w", err)
	}
	return cfg, db, nil
}

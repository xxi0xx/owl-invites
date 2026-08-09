package database

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending database migrations.
func RunMigrations(db DB) error {
	m, err := newMigrator(db)
	if err != nil {
		return err
	}

	// Recover from dirty migration state. When a previous migration was
	// interrupted the schema_migrations table is left with dirty=true,
	// which causes Up() to refuse to proceed. Force the current version
	// to clear the dirty flag so the migration can be retried.
	version, dirty, verr := m.Version()
	if verr == nil && dirty {
		if ferr := m.Force(int(version)); ferr != nil {
			return fmt.Errorf("force dirty version %d: %w", version, ferr)
		}
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}

	return nil
}

// RunMigrationsTo applies migrations up or down to an exact version. It is
// primarily used by cross-dialect migration tests that seed a pre-upgrade
// schema before applying the next migration.
func RunMigrationsTo(db DB, target uint) error {
	m, err := newMigrator(db)
	if err != nil {
		return err
	}
	if err := m.Migrate(target); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate to %d: %w", target, err)
	}
	return nil
}

func newMigrator(db DB) (*migrate.Migrate, error) {
	source, err := iofs.New(migrationsFS, "migrations/"+db.Dialect())
	if err != nil {
		return nil, fmt.Errorf("migration source: %w", err)
	}

	var driver database.Driver

	switch db.Dialect() {
	case "sqlite":
		driver, err = sqlite3.WithInstance(db.Underlying(), &sqlite3.Config{})
		if err != nil {
			return nil, fmt.Errorf("sqlite migration driver: %w", err)
		}
	case "postgres":
		driver, err = postgres.WithInstance(db.Underlying(), &postgres.Config{})
		if err != nil {
			return nil, fmt.Errorf("postgres migration driver: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported dialect for migrations: %s", db.Dialect())
	}

	m, err := migrate.NewWithInstance("iofs", source, db.Dialect(), driver)
	if err != nil {
		return nil, fmt.Errorf("migrate instance: %w", err)
	}

	return m, nil
}

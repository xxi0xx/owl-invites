package database

import (
	"embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationsFS embed.FS

const irreversibleGate2Version uint = 36

var ErrGate2RollbackUnsupported = errors.New("migration 36 is irreversible: restore a verified pre-upgrade backup to roll back Gate 2")

// MigrationStatus is the operator-visible database schema state.
type MigrationStatus struct {
	Current uint
	Latest  uint
	Dirty   bool
	Pending bool
}

// MigrationResult describes the schema transition performed by MigrateUp.
type MigrationResult struct {
	Before MigrationStatus
	After  MigrationStatus
}

// ReadMigrationStatus returns the installed and available schema versions.
func ReadMigrationStatus(db DB) (MigrationStatus, error) {
	latest, err := LatestMigrationVersion(db.Dialect())
	if err != nil {
		return MigrationStatus{}, err
	}
	m, err := newMigrator(db)
	if err != nil {
		return MigrationStatus{}, err
	}
	current, dirty, err := m.Version()
	if err != nil {
		if !errors.Is(err, migrate.ErrNilVersion) {
			return MigrationStatus{}, fmt.Errorf("read migration version: %w", err)
		}
		current = 0
		dirty = false
	}
	return MigrationStatus{
		Current: current,
		Latest:  latest,
		Dirty:   dirty,
		Pending: current < latest || dirty,
	}, nil
}

// LatestMigrationVersion returns the highest embedded up migration.
func LatestMigrationVersion(dialect string) (uint, error) {
	entries, err := migrationsFS.ReadDir("migrations/" + dialect)
	if err != nil {
		return 0, fmt.Errorf("read %s migrations: %w", dialect, err)
	}
	var latest uint64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			return 0, fmt.Errorf("invalid migration filename: %s", name)
		}
		version, parseErr := strconv.ParseUint(prefix, 10, 64)
		if parseErr != nil {
			return 0, fmt.Errorf("invalid migration filename %s: %w", name, parseErr)
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		return 0, fmt.Errorf("no up migrations found for dialect %s", dialect)
	}
	return uint(latest), nil
}

// MigrateUp applies pending migrations and returns the before/after state.
func MigrateUp(db DB) (MigrationResult, error) {
	before, err := ReadMigrationStatus(db)
	if err != nil {
		return MigrationResult{}, err
	}
	if err := RunMigrations(db); err != nil {
		return MigrationResult{Before: before}, err
	}
	after, err := ReadMigrationStatus(db)
	if err != nil {
		return MigrationResult{Before: before}, err
	}
	return MigrationResult{Before: before, After: after}, nil
}

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
	current, _, versionErr := m.Version()
	if versionErr != nil && versionErr != migrate.ErrNilVersion {
		return fmt.Errorf("read migration version: %w", versionErr)
	}
	if versionErr == nil && current >= irreversibleGate2Version && target < irreversibleGate2Version {
		return ErrGate2RollbackUnsupported
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

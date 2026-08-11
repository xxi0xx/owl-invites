package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/xxi0xx/owl-invites/internal/config"
)

// DB is the database interface used throughout the application. It wraps the
// standard library's *sql.DB with a Dialect method so callers can branch on
// the underlying engine when needed.
type DB interface {
	// Dialect returns "sqlite" or "postgres".
	Dialect() string

	// Close releases database resources.
	Close() error

	// Standard query methods.
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)

	// Underlying returns the raw *sql.DB for use by migration tooling or
	// any code that genuinely needs the concrete type.
	Underlying() *sql.DB
}

// Tx is the transaction interface used throughout the application. It mirrors
// the subset of *sql.Tx the codebase relies on. The postgres implementation
// wraps *sql.Tx so it can rewrite `?` placeholders before delegating; *sql.Tx
// satisfies this interface structurally for the SQLite path.
type Tx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	Commit() error
	Rollback() error
}

// New creates a database connection based on the supplied configuration.
func New(cfg *config.Config) (DB, error) {
	switch cfg.DBDriver {
	case "sqlite":
		return newSQLite(cfg)
	case "postgres":
		return newPostgres(cfg)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.DBDriver)
	}
}

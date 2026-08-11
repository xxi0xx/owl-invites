package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/xxi0xx/owl-invites/internal/config"
)

// postgresDB implements the DB interface for PostgreSQL.
type postgresDB struct {
	db *sql.DB
}

func newPostgres(cfg *config.Config) (DB, error) {
	db, err := sql.Open("postgres", cfg.DBDSN)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	return &postgresDB{db: db}, nil
}

func (p *postgresDB) Dialect() string     { return "postgres" }
func (p *postgresDB) Close() error        { return p.db.Close() }
func (p *postgresDB) Underlying() *sql.DB { return p.db }

func (p *postgresDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.db.ExecContext(ctx, rewritePlaceholders(query), args...)
}

func (p *postgresDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, rewritePlaceholders(query), args...)
}

func (p *postgresDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.db.QueryRowContext(ctx, rewritePlaceholders(query), args...)
}

func (p *postgresDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	tx, err := p.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &postgresTx{tx: tx}, nil
}

// postgresTx wraps *sql.Tx so that `?` placeholders are rewritten to `$N`
// before each statement is executed, matching the behaviour of postgresDB.
type postgresTx struct {
	tx *sql.Tx
}

func (t *postgresTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, rewritePlaceholders(query), args...)
}

func (t *postgresTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, rewritePlaceholders(query), args...)
}

func (t *postgresTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, rewritePlaceholders(query), args...)
}

func (t *postgresTx) Commit() error   { return t.tx.Commit() }
func (t *postgresTx) Rollback() error { return t.tx.Rollback() }

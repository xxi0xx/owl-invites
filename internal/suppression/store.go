package suppression

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/xxi0xx/owl-invites/internal/database"
)

// Store handles database operations for email suppressions and unsubscribe tokens.
type Store struct {
	db database.DB
}

// NewStore creates a new suppression Store.
func NewStore(db database.DB) *Store {
	return &Store{db: db}
}

// IsSuppressed reports whether the email is suppressed either globally
// (event_id IS NULL) or for the given event. A nil/empty eventID checks only
// global suppression.
func (s *Store) IsSuppressed(ctx context.Context, email string, eventID *string) (bool, error) {
	var count int
	if eventID == nil || *eventID == "" {
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM email_suppressions WHERE email = ? AND event_id IS NULL`,
			email,
		).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("query suppression: %w", err)
		}
		return count > 0, nil
	}

	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_suppressions
		 WHERE email = ? AND (event_id IS NULL OR event_id = ?)`,
		email, *eventID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("query suppression: %w", err)
	}
	return count > 0, nil
}

// Suppress records a suppression entry. It is idempotent: re-suppressing the
// same (email, event_id) pair is a no-op. Because SQL UNIQUE indexes treat NULL
// values as distinct, idempotency for global (NULL event_id) suppressions is
// enforced here with an explicit existence check rather than relying solely on
// the unique index.
func (s *Store) Suppress(ctx context.Context, email string, eventID *string, reason string) error {
	exists, err := s.existsExact(ctx, email, eventID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.Must(uuid.NewV7()).String()

	var eventIDArg any
	if eventID != nil && *eventID != "" {
		eventIDArg = *eventID
	} else {
		eventIDArg = nil
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO email_suppressions (id, email, event_id, reason, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, email, eventIDArg, reason, now,
	)
	if err != nil {
		// A concurrent insert may have populated the row (event-scoped
		// rows are protected by the unique index); treat that as success.
		if again, checkErr := s.existsExact(ctx, email, eventID); checkErr == nil && again {
			return nil
		}
		return fmt.Errorf("insert suppression: %w", err)
	}
	return nil
}

// existsExact reports whether a suppression row exists for exactly this
// (email, event_id) pair, correctly matching NULL event_id.
func (s *Store) existsExact(ctx context.Context, email string, eventID *string) (bool, error) {
	var count int
	var err error
	if eventID == nil || *eventID == "" {
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM email_suppressions WHERE email = ? AND event_id IS NULL`,
			email,
		).Scan(&count)
	} else {
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM email_suppressions WHERE email = ? AND event_id = ?`,
			email, *eventID,
		).Scan(&count)
	}
	if err != nil {
		return false, fmt.Errorf("check suppression exists: %w", err)
	}
	return count > 0, nil
}

// CreateToken persists an unsubscribe token (storing only its hash) and returns
// the row id. The caller stores the raw token in the unsubscribe link.
func (s *Store) CreateToken(ctx context.Context, tokenHash, email string, eventID *string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.Must(uuid.NewV7()).String()

	var eventIDArg any
	if eventID != nil && *eventID != "" {
		eventIDArg = *eventID
	} else {
		eventIDArg = nil
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO unsubscribe_tokens (id, token_hash, email, event_id, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, tokenHash, email, eventIDArg, now,
	)
	if err != nil {
		return fmt.Errorf("insert unsubscribe token: %w", err)
	}
	return nil
}

// FindTokenByHash looks up the email and event scope for a token hash. It
// returns ok=false when no matching token exists.
func (s *Store) FindTokenByHash(ctx context.Context, tokenHash string) (email string, eventID *string, ok bool, err error) {
	var dbEmail string
	var dbEventID sql.NullString

	scanErr := s.db.QueryRowContext(ctx,
		`SELECT email, event_id FROM unsubscribe_tokens WHERE token_hash = ?`,
		tokenHash,
	).Scan(&dbEmail, &dbEventID)
	if scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return "", nil, false, nil
		}
		return "", nil, false, fmt.Errorf("find unsubscribe token: %w", scanErr)
	}

	if dbEventID.Valid {
		eid := dbEventID.String
		return dbEmail, &eid, true, nil
	}
	return dbEmail, nil, true, nil
}

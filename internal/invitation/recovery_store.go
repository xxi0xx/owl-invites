package invitation

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AllowRecovery records a privacy-preserving recovery attempt and enforces
// source, destination, and event budgets in the database. Fingerprints are
// keyed HMAC values; raw contacts and client identifiers are not stored.
func (s *Store) AllowRecovery(ctx context.Context, eventID, sourceFingerprint, destinationFingerprint string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin recovery limit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The counts and insert form one budget decision. PostgreSQL advisory locks
	// serialize this decision across application instances. SQLite's no-op write
	// upgrades the deferred transaction before it reads the counters, acquiring
	// the engine's database write reservation without changing a row.
	switch s.db.Dialect() {
	case "postgres":
		const recoveryBudgetLock int64 = 0x4f574c5245434f56 // "OWLRECOV"
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(?)`, recoveryBudgetLock); err != nil {
			return false, fmt.Errorf("lock recovery budget: %w", err)
		}
	case "sqlite":
		if _, err := tx.ExecContext(ctx, `UPDATE invitation_recovery_attempts
			SET created_at = created_at WHERE 0`); err != nil {
			return false, fmt.Errorf("lock recovery budget: %w", err)
		}
	default:
		return false, fmt.Errorf("lock recovery budget: unsupported dialect %q", s.db.Dialect())
	}

	cutoff := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	checks := []struct {
		query string
		arg   string
		limit int
	}{
		{`SELECT COUNT(*) FROM invitation_recovery_attempts
			WHERE source_fingerprint = ? AND created_at > ?`, sourceFingerprint, 5},
		{`SELECT COUNT(*) FROM invitation_recovery_attempts
			WHERE destination_fingerprint = ? AND created_at > ?`, destinationFingerprint, 3},
		{`SELECT COUNT(*) FROM invitation_recovery_attempts
			WHERE event_id = ? AND created_at > ?`, eventID, 30},
	}
	for _, check := range checks {
		var count int
		if err := tx.QueryRowContext(ctx, check.query, check.arg, cutoff).Scan(&count); err != nil {
			return false, fmt.Errorf("count recovery attempts: %w", err)
		}
		if count >= check.limit {
			return false, nil
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO invitation_recovery_attempts (
		id, event_id, source_fingerprint, destination_fingerprint, created_at
	) VALUES (?, ?, ?, ?, ?)`, uuid.Must(uuid.NewV7()).String(), eventID,
		sourceFingerprint, destinationFingerprint, now)
	if err != nil {
		return false, fmt.Errorf("record recovery attempt: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("commit recovery limit: %w", err)
	}
	return true, nil
}

func (s *Store) FindRecoveryMatches(ctx context.Context, eventID, normalizedContact string) ([]*Invitation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+invitationColumns+` FROM invitations
		WHERE event_id = ? AND revoked_at IS NULL
		AND (normalized_contact_email = ? OR normalized_contact_phone = ?)
		ORDER BY created_at, id`, eventID, normalizedContact, normalizedContact)
	if err != nil {
		return nil, fmt.Errorf("find recovery invitations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]*Invitation, 0)
	for rows.Next() {
		inv, scanErr := scanInvitationRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, inv)
	}
	return result, rows.Err()
}

func (s *Store) CreateRecoveryToken(ctx context.Context, invitationID, tokenHash, destination string, expiresAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO invitation_recovery_tokens (
		id, invitation_id, token_hash, destination, expires_at, created_at
	) VALUES (?, ?, ?, ?, ?, ?)`, uuid.Must(uuid.NewV7()).String(), invitationID,
		tokenHash, destination, expiresAt.UTC().Format(time.RFC3339Nano), now)
	if err != nil {
		return fmt.Errorf("create recovery token: %w", err)
	}
	return nil
}

// ConsumeRecoveryAndCreateSession atomically consumes exactly one live
// recovery token and creates an invitation session. Replays cannot mint a
// second session even under concurrent requests.
func (s *Store) ConsumeRecoveryAndCreateSession(ctx context.Context, recoveryHash, sessionHash string, sessionExpiresAt time.Time) (*Invitation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin recovery exchange: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	var invitationID string
	var tokenVersion int
	err = tx.QueryRowContext(ctx, `SELECT i.id, i.token_version
		FROM invitation_recovery_tokens rt
		JOIN invitations i ON i.id = rt.invitation_id
		WHERE rt.token_hash = ? AND rt.consumed_at IS NULL AND rt.expires_at > ?
		AND i.revoked_at IS NULL`, recoveryHash, now).Scan(&invitationID, &tokenVersion)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidCapability
	}
	if err != nil {
		return nil, fmt.Errorf("load recovery token: %w", err)
	}

	result, err := tx.ExecContext(ctx, `UPDATE invitation_recovery_tokens
		SET consumed_at = ? WHERE token_hash = ? AND consumed_at IS NULL
		AND expires_at > ?`, now, recoveryHash, now)
	if err != nil {
		return nil, fmt.Errorf("consume recovery token: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return nil, ErrInvalidCapability
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO invitation_sessions (
		id, invitation_id, token_hash, issued_token_version, expires_at, created_at
	) VALUES (?, ?, ?, ?, ?, ?)`, uuid.Must(uuid.NewV7()).String(), invitationID,
		sessionHash, tokenVersion, sessionExpiresAt.UTC().Format(time.RFC3339Nano), now)
	if err != nil {
		return nil, fmt.Errorf("create recovery session: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit recovery exchange: %w", err)
	}
	return s.FindByID(ctx, invitationID)
}

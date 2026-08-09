package instanceconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/yannkr/openrsvp/internal/database"
)

const singletonInstanceID = "default"

// Store handles the singleton typed instance record and bootstrap audit write.
type Store struct {
	db database.DB
}

func NewStore(db database.DB) *Store { return &Store{db: db} }

func (s *Store) BeginTx(ctx context.Context) (database.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}

func (s *Store) GetInstance(ctx context.Context) (*Instance, error) {
	return scanInstance(s.db.QueryRowContext(ctx, `SELECT id, name, default_timezone,
		allow_signups, support_email, setup_completed_at, created_at, updated_at
		FROM instances WHERE id = ?`, singletonInstanceID))
}

// GetAll retains the startup override interface while sourcing values from the
// typed singleton instead of the legacy key/value table.
func (s *Store) GetAll(ctx context.Context) (map[string]string, error) {
	instance, err := s.GetInstance(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		KeyInstanceName:    instance.Name,
		KeyDefaultTimezone: instance.DefaultTimezone,
		KeyAllowSignups:    boolToString(instance.AllowSignups),
		KeySupportEmail:    instance.SupportEmail,
		KeyConfigured:      boolToString(instance.SetupCompletedAt != nil),
	}, nil
}

func (s *Store) IsConfigured(ctx context.Context) (bool, error) {
	instance, err := s.GetInstance(ctx)
	if err != nil {
		return false, err
	}
	return instance.SetupCompletedAt != nil, nil
}

// UpdateSettings persists ongoing settings without touching setup completion.
func (s *Store) UpdateSettings(ctx context.Context, in *Settings) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `UPDATE instances SET name = ?, default_timezone = ?,
		allow_signups = ?, support_email = ?, updated_at = ? WHERE id = ?`,
		in.InstanceName, in.DefaultTimezone, in.AllowSignups, in.SupportEmail, now, singletonInstanceID)
	if err != nil {
		return fmt.Errorf("update instance settings: %w", err)
	}
	return nil
}

// ClaimBootstrapTx atomically closes setup-required mode and writes initial
// settings. It succeeds exactly once and only while no active admin exists.
func (s *Store) ClaimBootstrapTx(ctx context.Context, tx database.Tx, in *BootstrapRequest, at time.Time) (bool, error) {
	now := at.UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, `UPDATE instances SET name = ?, default_timezone = ?,
		allow_signups = ?, support_email = ?, setup_completed_at = ?, updated_at = ?
		WHERE id = ? AND setup_completed_at IS NULL
		AND NOT EXISTS (
			SELECT 1 FROM users WHERE instance_role = 'admin' AND status = 'active'
		)`,
		in.InstanceName, in.DefaultTimezone, in.AllowSignups, in.SupportEmail,
		now, now, singletonInstanceID)
	if err != nil {
		return false, fmt.Errorf("claim bootstrap: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read bootstrap claim result: %w", err)
	}
	return affected == 1, nil
}

func (s *Store) InsertAuditTx(ctx context.Context, tx database.Tx, actorUserID *string, actorKind, action string, targetUserID, eventID *string, metadata map[string]any, at time.Time) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log (
		id, actor_user_id, actor_kind, action, target_user_id, event_id, metadata_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.Must(uuid.NewV7()).String(), actorUserID, actorKind, action,
		targetUserID, eventID, string(metadataJSON), at.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert admin audit: %w", err)
	}
	return nil
}

func scanInstance(row *sql.Row) (*Instance, error) {
	var instance Instance
	var setupCompleted sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(
		&instance.ID, &instance.Name, &instance.DefaultTimezone,
		&instance.AllowSignups, &instance.SupportEmail, &setupCompleted,
		&createdAt, &updatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan instance: %w", err)
	}
	var err error
	instance.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse instance created_at: %w", err)
	}
	instance.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse instance updated_at: %w", err)
	}
	if setupCompleted.Valid {
		t, parseErr := time.Parse(time.RFC3339, setupCompleted.String)
		if parseErr != nil {
			return nil, fmt.Errorf("parse setup_completed_at: %w", parseErr)
		}
		instance.SetupCompletedAt = &t
	}
	return &instance, nil
}

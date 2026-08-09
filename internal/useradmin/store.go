package useradmin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/database"
)

type Store struct {
	db database.DB
}

func NewStore(db database.DB) *Store { return &Store{db: db} }

func (s *Store) BeginTx(ctx context.Context) (database.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}

func (s *Store) ListUsers(ctx context.Context) ([]*auth.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, normalized_email, display_name,
		timezone, instance_role, status, invited_by_user_id, activated_at,
		last_login_at, created_at, updated_at FROM users ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := make([]*auth.User, 0)
	for rows.Next() {
		user, scanErr := scanUserRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) CreateInviteTx(ctx context.Context, tx database.Tx, target *auth.User, actorID, tokenHash string, expiresAt, now time.Time) (*AccountInvite, error) {
	invite := &AccountInvite{
		ID: uuid.Must(uuid.NewV7()).String(), TargetUserID: target.ID,
		Email: target.NormalizedEmail, InvitedByUserID: actorID,
		ExpiresAt: expiresAt.UTC(), CreatedAt: now.UTC(),
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO account_invites (
		id, target_user_id, normalized_email, token_hash, invited_by_user_id,
		expires_at, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)`, invite.ID, invite.TargetUserID, invite.Email,
		tokenHash, invite.InvitedByUserID, invite.ExpiresAt.Format(time.RFC3339),
		invite.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("create account invite: %w", err)
	}
	return invite, nil
}

func (s *Store) RevokePendingInvitesTx(ctx context.Context, tx database.Tx, targetUserID string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE account_invites SET revoked_at = ?
		WHERE target_user_id = ? AND accepted_at IS NULL AND revoked_at IS NULL`,
		now.UTC().Format(time.RFC3339), targetUserID)
	if err != nil {
		return fmt.Errorf("revoke pending invites: %w", err)
	}
	return nil
}

func (s *Store) ListPendingInvites(ctx context.Context, now time.Time) ([]AccountInvite, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, target_user_id, normalized_email,
		invited_by_user_id, event_id, event_role, expires_at, accepted_at,
		revoked_at, created_at FROM account_invites
		WHERE accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?
		ORDER BY created_at DESC, id DESC`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("list pending account invites: %w", err)
	}
	defer func() { _ = rows.Close() }()
	invites := make([]AccountInvite, 0)
	for rows.Next() {
		invite, scanErr := scanInvite(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		invites = append(invites, *invite)
	}
	return invites, rows.Err()
}

func (s *Store) RevokeInviteByIDTx(ctx context.Context, tx database.Tx, id string, now time.Time) (*AccountInvite, error) {
	row := tx.QueryRowContext(ctx, `SELECT id, target_user_id, normalized_email,
		invited_by_user_id, event_id, event_role, expires_at, accepted_at,
		revoked_at, created_at FROM account_invites
		WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, id)
	invite, err := scanInvite(row)
	if err != nil {
		return nil, err
	}
	if invite == nil {
		return nil, ErrInvalidInvite
	}
	result, err := tx.ExecContext(ctx, `UPDATE account_invites SET revoked_at = ?
		WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL`,
		now.UTC().Format(time.RFC3339), id)
	if err != nil {
		return nil, fmt.Errorf("revoke account invite: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return nil, ErrInvalidInvite
	}
	return invite, nil
}

func (s *Store) FindLiveInviteByHashTx(ctx context.Context, tx database.Tx, tokenHash string, now time.Time) (*AccountInvite, error) {
	row := tx.QueryRowContext(ctx, `SELECT id, target_user_id, normalized_email,
		invited_by_user_id, event_id, event_role, expires_at, accepted_at,
		revoked_at, created_at FROM account_invites
		WHERE token_hash = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?`,
		tokenHash, now.UTC().Format(time.RFC3339))
	return scanInvite(row)
}

func (s *Store) ConsumeInviteTx(ctx context.Context, tx database.Tx, id string, now time.Time) error {
	nowText := now.UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, `UPDATE account_invites SET accepted_at = ?
		WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?`,
		nowText, id, nowText)
	if err != nil {
		return fmt.Errorf("consume account invite: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read account invite consume result: %w", err)
	}
	if affected != 1 {
		return ErrInvalidInvite
	}
	return nil
}

// LockAdminMutationTx serializes privilege changes through the singleton
// instance row, preventing concurrent requests from removing the last admin.
func (s *Store) LockAdminMutationTx(ctx context.Context, tx database.Tx) error {
	_, err := tx.ExecContext(ctx, "UPDATE instances SET updated_at = updated_at WHERE id = 'default'")
	if err != nil {
		return fmt.Errorf("lock admin mutation: %w", err)
	}
	return nil
}

func (s *Store) CountActiveAdminsTx(ctx context.Context, tx database.Tx) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users
		WHERE instance_role = ? AND status = ?`, auth.InstanceRoleAdmin, auth.UserStatusActive).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active admins: %w", err)
	}
	return count, nil
}

func (s *Store) UpdateStatusTx(ctx context.Context, tx database.Tx, userID, status string, now time.Time) error {
	nowText := now.UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, "UPDATE users SET status = ?, updated_at = ? WHERE id = ?", status, nowText, userID)
	if err != nil {
		return fmt.Errorf("update user status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return ErrUserNotFound
	}
	if status == auth.UserStatusDisabled {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE organizer_id = ?", userID); err != nil {
			return fmt.Errorf("revoke disabled user sessions: %w", err)
		}
		if err := s.RevokePendingInvitesTx(ctx, tx, userID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateRoleTx(ctx context.Context, tx database.Tx, userID, role string, now time.Time) error {
	nowText := now.UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, "UPDATE users SET instance_role = ?, updated_at = ? WHERE id = ?", role, nowText, userID)
	if err != nil {
		return fmt.Errorf("update user role: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return ErrUserNotFound
	}
	if _, err := tx.ExecContext(ctx, "UPDATE organizers SET is_admin = ?, updated_at = ? WHERE id = ?", role == auth.InstanceRoleAdmin, nowText, userID); err != nil {
		return fmt.Errorf("update organizer compatibility role: %w", err)
	}
	return nil
}

func (s *Store) InsertAuditTx(ctx context.Context, tx database.Tx, actorID, action, targetID string, metadata map[string]any, now time.Time) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log (
		id, actor_user_id, actor_kind, action, target_user_id, metadata_json, created_at
	) VALUES (?, ?, 'user', ?, ?, ?, ?)`, uuid.Must(uuid.NewV7()).String(), actorID,
		action, targetID, string(encoded), now.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert admin audit: %w", err)
	}
	return nil
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, actor_user_id, actor_kind, action,
		target_user_id, event_id, metadata_json, created_at FROM admin_audit_log
		ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list admin audit: %w", err)
	}
	defer func() { _ = rows.Close() }()
	entries := make([]AuditEntry, 0)
	for rows.Next() {
		var entry AuditEntry
		var actorID, targetID, eventID sql.NullString
		var metadata, createdAt string
		if err := rows.Scan(&entry.ID, &actorID, &entry.ActorKind, &entry.Action,
			&targetID, &eventID, &metadata, &createdAt); err != nil {
			return nil, fmt.Errorf("scan admin audit: %w", err)
		}
		entry.ActorUserID = nullStringPointer(actorID)
		entry.TargetUserID = nullStringPointer(targetID)
		entry.EventID = nullStringPointer(eventID)
		entry.Metadata = json.RawMessage(metadata)
		parsed, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse audit created_at: %w", err)
		}
		entry.CreatedAt = parsed
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInvite(row rowScanner) (*AccountInvite, error) {
	var invite AccountInvite
	var eventID, eventRole, acceptedAt, revokedAt sql.NullString
	var expiresAt, createdAt string
	if err := row.Scan(&invite.ID, &invite.TargetUserID, &invite.Email,
		&invite.InvitedByUserID, &eventID, &eventRole, &expiresAt, &acceptedAt,
		&revokedAt, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan account invite: %w", err)
	}
	invite.EventID = nullStringPointer(eventID)
	invite.EventRole = nullStringPointer(eventRole)
	var err error
	invite.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse invite expires_at: %w", err)
	}
	invite.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse invite created_at: %w", err)
	}
	invite.AcceptedAt, err = parseOptionalTime(acceptedAt)
	if err != nil {
		return nil, err
	}
	invite.RevokedAt, err = parseOptionalTime(revokedAt)
	if err != nil {
		return nil, err
	}
	return &invite, nil
}

func scanUserRows(rows *sql.Rows) (*auth.User, error) {
	var user auth.User
	var invitedBy, activatedAt, lastLoginAt sql.NullString
	var createdAt, updatedAt string
	if err := rows.Scan(&user.ID, &user.Email, &user.NormalizedEmail, &user.Name,
		&user.Timezone, &user.InstanceRole, &user.Status, &invitedBy, &activatedAt,
		&lastLoginAt, &createdAt, &updatedAt); err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	user.IsAdmin = user.InstanceRole == auth.InstanceRoleAdmin
	user.InvitedByUserID = nullStringPointer(invitedBy)
	var err error
	user.ActivatedAt, err = parseOptionalTime(activatedAt)
	if err != nil {
		return nil, err
	}
	user.LastLoginAt, err = parseOptionalTime(lastLoginAt)
	if err != nil {
		return nil, err
	}
	user.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, err
	}
	user.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func parseOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value.String)
	if err != nil {
		return nil, fmt.Errorf("parse optional time: %w", err)
	}
	return &parsed, nil
}

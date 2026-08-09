package auth

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yannkr/openrsvp/internal/database"
)

// executor is a minimal interface satisfied by both database.DB and *sql.Tx,
// allowing store methods to run inside or outside a transaction.
type executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store handles database operations for authentication.
type Store struct {
	db database.DB
}

// NewStore creates a new auth Store.
func NewStore(db database.DB) *Store {
	return &Store{db: db}
}

// BeginTx starts a new database transaction.
func (s *Store) BeginTx(ctx context.Context) (database.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}

// FindOrganizerByEmail retrieves an organizer by their email address.
func (s *Store) FindOrganizerByEmail(ctx context.Context, email string) (*Organizer, error) {
	row := s.db.QueryRowContext(ctx,
		userSelect+" WHERE normalized_email = ?",
		normalizeEmail(email),
	)

	return scanOrganizer(row)
}

// FindUserByEmailTx retrieves a user inside a caller-owned transaction.
func (s *Store) FindUserByEmailTx(ctx context.Context, tx database.Tx, email string) (*User, error) {
	row := tx.QueryRowContext(ctx, userSelect+" WHERE normalized_email = ?", normalizeEmail(email))
	return scanOrganizer(row)
}

// FindOrganizerByID retrieves an organizer by their ID.
func (s *Store) FindOrganizerByID(ctx context.Context, id string) (*Organizer, error) {
	return findOrganizerByID(ctx, s.db, id)
}

// FindOrganizerByIDTx retrieves an organizer by their ID within a transaction.
func (s *Store) FindOrganizerByIDTx(ctx context.Context, tx database.Tx, id string) (*Organizer, error) {
	return findOrganizerByID(ctx, tx, id)
}

func findOrganizerByID(ctx context.Context, exec executor, id string) (*Organizer, error) {
	row := exec.QueryRowContext(ctx,
		userSelect+" WHERE id = ?",
		id,
	)

	return scanOrganizer(row)
}

// CreateOrganizer creates a new organizer with the given email. The ID is
// generated as a UUIDv7.
func (s *Store) CreateOrganizer(ctx context.Context, email string) (*Organizer, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	user, err := createUser(ctx, tx, email, "", "UTC", InstanceRoleUser, UserStatusActive, nil)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create user: %w", err)
	}
	return user, nil
}

// CreateUserTx creates a persistent user and its temporary legacy organizer
// shadow in one transaction. The shadow is required only until event foreign
// keys are migrated to users in the membership slice.
func (s *Store) CreateUserTx(ctx context.Context, tx database.Tx, email, name, timezone, role, status string, invitedBy *string) (*User, error) {
	return createUser(ctx, tx, email, name, timezone, role, status, invitedBy)
}

func createUser(ctx context.Context, exec executor, email, name, timezone, role, status string, invitedBy *string) (*User, error) {
	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC().Format(time.RFC3339)
	email = strings.TrimSpace(email)
	if timezone == "" {
		timezone = "UTC"
	}
	var activatedAt any
	if status == UserStatusActive {
		activatedAt = now
	}

	_, err := exec.ExecContext(ctx,
		`INSERT INTO users (
			id, email, normalized_email, display_name, timezone, instance_role,
			status, invited_by_user_id, activated_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, email, normalizeEmail(email), name, timezone, role, status, invitedBy, activatedAt, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	_, err = exec.ExecContext(ctx,
		"INSERT INTO organizers (id, email, name, timezone, is_admin, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, email, name, timezone, role == InstanceRoleAdmin, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create organizer compatibility row: %w", err)
	}

	return findOrganizerByID(ctx, exec, id)
}

// UpdateOrganizer updates the name, timezone, and updated_at timestamp for an organizer.
func (s *Store) UpdateOrganizer(ctx context.Context, organizer *Organizer) error {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		"UPDATE users SET display_name = ?, timezone = ?, updated_at = ? WHERE id = ?",
		organizer.Name, organizer.Timezone, now, organizer.ID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		"UPDATE organizers SET name = ?, timezone = ?, updated_at = ? WHERE id = ?",
		organizer.Name, organizer.Timezone, now, organizer.ID); err != nil {
		return fmt.Errorf("update organizer compatibility row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update user: %w", err)
	}
	return nil
}

// CreateMagicLink stores a new magic link with a hashed token.
func (s *Store) CreateMagicLink(ctx context.Context, tokenHash, organizerID string, expiresAt time.Time) error {
	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC().Format(time.RFC3339)
	exp := expiresAt.UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO magic_links (id, token_hash, organizer_id, expires_at, created_at) VALUES (?, ?, ?, ?, ?)",
		id, tokenHash, organizerID, exp, now,
	)
	if err != nil {
		return fmt.Errorf("create magic link: %w", err)
	}

	return nil
}

// FindMagicLinkByHash retrieves a magic link by its token hash.
func (s *Store) FindMagicLinkByHash(ctx context.Context, tokenHash string) (*MagicLink, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, token_hash, organizer_id, expires_at, used_at, created_at FROM magic_links WHERE token_hash = ?",
		tokenHash,
	)

	var ml MagicLink
	var expiresAt, createdAt string
	var usedAt sql.NullString

	err := row.Scan(&ml.ID, &ml.TokenHash, &ml.OrganizerID, &expiresAt, &usedAt, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find magic link by hash: %w", err)
	}

	ml.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}

	ml.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	if usedAt.Valid {
		t, err := time.Parse(time.RFC3339, usedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse used_at: %w", err)
		}
		ml.UsedAt = &t
	}

	return &ml, nil
}

// MarkMagicLinkUsed sets the used_at timestamp for a magic link.
func (s *Store) MarkMagicLinkUsed(ctx context.Context, id string) error {
	return markMagicLinkUsed(ctx, s.db, id, time.Now().UTC())
}

// MarkMagicLinkUsedTx atomically consumes a live magic link within a
// transaction. A link that is already used or expired is indistinguishable
// from any other invalid token.
func (s *Store) MarkMagicLinkUsedTx(ctx context.Context, tx database.Tx, id string, now time.Time) error {
	return markMagicLinkUsed(ctx, tx, id, now)
}

func markMagicLinkUsed(ctx context.Context, exec executor, id string, now time.Time) error {
	nowText := now.UTC().Format(time.RFC3339)

	result, err := exec.ExecContext(ctx,
		"UPDATE magic_links SET used_at = ? WHERE id = ? AND used_at IS NULL AND expires_at > ?",
		nowText, id, nowText,
	)
	if err != nil {
		return fmt.Errorf("mark magic link used: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read magic link consume result: %w", err)
	}
	if affected != 1 {
		return ErrInvalidToken
	}

	return nil
}

// CreateSession creates a new session and returns it.
func (s *Store) CreateSession(ctx context.Context, tokenHash, organizerID string, expiresAt time.Time) (*Session, error) {
	return createSession(ctx, s.db, tokenHash, organizerID, expiresAt)
}

// CreateSessionTx creates a new session within a transaction and returns it.
func (s *Store) CreateSessionTx(ctx context.Context, tx database.Tx, tokenHash, organizerID string, expiresAt time.Time) (*Session, error) {
	return createSession(ctx, tx, tokenHash, organizerID, expiresAt)
}

func createSession(ctx context.Context, exec executor, tokenHash, organizerID string, expiresAt time.Time) (*Session, error) {
	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC().Format(time.RFC3339)
	exp := expiresAt.UTC().Format(time.RFC3339)

	_, err := exec.ExecContext(ctx,
		"INSERT INTO sessions (id, token_hash, organizer_id, expires_at, created_at) VALUES (?, ?, ?, ?, ?)",
		id, tokenHash, organizerID, exp, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Session{
		ID:          id,
		TokenHash:   tokenHash,
		OrganizerID: organizerID,
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

// FindSessionByHash retrieves a session by its token hash.
func (s *Store) FindSessionByHash(ctx context.Context, tokenHash string) (*Session, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, token_hash, organizer_id, expires_at, created_at FROM sessions WHERE token_hash = ?",
		tokenHash,
	)

	var sess Session
	var expiresAt, createdAt string

	err := row.Scan(&sess.ID, &sess.TokenHash, &sess.OrganizerID, &expiresAt, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find session by hash: %w", err)
	}

	sess.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}

	sess.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	return &sess, nil
}

// DeleteSession removes a session by ID.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes all sessions whose expires_at is in the past.
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

// DeleteExpiredMagicLinks removes all magic links whose expires_at is in the past.
func (s *Store) DeleteExpiredMagicLinks(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, "DELETE FROM magic_links WHERE expires_at < ?", now)
	if err != nil {
		return fmt.Errorf("delete expired magic links: %w", err)
	}
	return nil
}

// SetAdminStatus updates the is_admin flag for an organizer.
func (s *Store) SetAdminStatus(ctx context.Context, id string, isAdmin bool) error {
	role := InstanceRoleUser
	if isAdmin {
		role = InstanceRoleAdmin
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin set admin status: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "UPDATE users SET instance_role = ?, updated_at = ? WHERE id = ?", role, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		return fmt.Errorf("set user role: %w", err)
	}
	_, err = tx.ExecContext(ctx, "UPDATE organizers SET is_admin = ? WHERE id = ?", isAdmin, id)
	if err != nil {
		return fmt.Errorf("set organizer compatibility role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set admin status: %w", err)
	}
	return nil
}

// TouchLastLoginTx records a successful authentication in the same
// transaction that consumes the credential and creates the session.
func (s *Store) TouchLastLoginTx(ctx context.Context, tx database.Tx, id string, at time.Time) error {
	now := at.UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx,
		"UPDATE users SET last_login_at = ?, updated_at = ? WHERE id = ? AND status = ?",
		now, now, id, UserStatusActive)
	if err != nil {
		return fmt.Errorf("touch last login: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read last login result: %w", err)
	}
	if affected != 1 {
		return ErrInvalidToken
	}
	return nil
}

// PromoteBootstrapAdminTx activates an existing legacy-migrated identity as
// the first persistent administrator. It is only called after the bootstrap
// transaction has atomically claimed setup-required mode.
func (s *Store) PromoteBootstrapAdminTx(ctx context.Context, tx database.Tx, id, name, timezone string, at time.Time) (*User, error) {
	now := at.UTC().Format(time.RFC3339)
	if timezone == "" {
		timezone = "UTC"
	}
	result, err := tx.ExecContext(ctx, `UPDATE users SET display_name = ?, timezone = ?,
		instance_role = ?, status = ?, activated_at = COALESCE(activated_at, ?), updated_at = ?
		WHERE id = ?`, name, timezone, InstanceRoleAdmin, UserStatusActive, now, now, id)
	if err != nil {
		return nil, fmt.Errorf("promote bootstrap admin: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return nil, fmt.Errorf("promote bootstrap admin: user not found")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE organizers SET name = ?, timezone = ?,
		is_admin = ?, updated_at = ? WHERE id = ?`, name, timezone, true, now, id); err != nil {
		return nil, fmt.Errorf("promote organizer compatibility row: %w", err)
	}
	return findOrganizerByID(ctx, tx, id)
}

// ActivateInvitedUserTx transitions exactly one invited identity to active.
// It is intentionally conditional so an account-invitation capability cannot
// be reused after another concurrent acceptance wins.
func (s *Store) ActivateInvitedUserTx(ctx context.Context, tx database.Tx, id string, at time.Time) (*User, error) {
	now := at.UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, `UPDATE users SET status = ?, activated_at = ?,
		updated_at = ? WHERE id = ? AND status = ?`, UserStatusActive, now, now, id, UserStatusInvited)
	if err != nil {
		return nil, fmt.Errorf("activate invited user: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return nil, ErrInvalidToken
	}
	return findOrganizerByID(ctx, tx, id)
}

// ExportOrganizerData gathers every record owned by the organizer into a
// single document suitable for a GDPR-style data export. It reads the
// organizer's profile, their events, and every child record belonging to
// those events. Only data owned by the given organizer is returned.
func (s *Store) ExportOrganizerData(ctx context.Context, organizerID string) (*ExportDocument, error) {
	organizer, err := s.FindOrganizerByID(ctx, organizerID)
	if err != nil {
		return nil, fmt.Errorf("find organizer: %w", err)
	}
	if organizer == nil {
		return nil, sql.ErrNoRows
	}

	doc := &ExportDocument{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Organizer:  organizer,
		Events:     []map[string]any{},
		Series:     []map[string]any{},
	}

	// Collect the event IDs owned by this organizer so child rows can be
	// scoped without trusting the child tables' own foreign keys.
	eventIDs, err := s.organizerEventIDs(ctx, organizerID)
	if err != nil {
		return nil, fmt.Errorf("list event ids: %w", err)
	}

	doc.Events, err = queryRowsByOrganizer(ctx, s.db,
		"SELECT * FROM events WHERE organizer_id = ? ORDER BY created_at", organizerID)
	if err != nil {
		return nil, fmt.Errorf("export events: %w", err)
	}

	doc.Series, err = queryRowsByOrganizer(ctx, s.db,
		"SELECT * FROM event_series WHERE organizer_id = ? ORDER BY created_at", organizerID)
	if err != nil {
		return nil, fmt.Errorf("export series: %w", err)
	}

	if len(eventIDs) > 0 {
		in, args := inClause(eventIDs)

		// Child tables keyed directly by event_id.
		eventChildren := map[string]*[]map[string]any{
			"SELECT * FROM attendees WHERE event_id IN " + in:        &doc.Attendees,
			"SELECT * FROM event_questions WHERE event_id IN " + in:  &doc.Questions,
			"SELECT * FROM event_comments WHERE event_id IN " + in:   &doc.Comments,
			"SELECT * FROM messages WHERE event_id IN " + in:         &doc.Messages,
			"SELECT * FROM webhooks WHERE event_id IN " + in:         &doc.Webhooks,
			"SELECT * FROM reminders WHERE event_id IN " + in:        &doc.Reminders,
			"SELECT * FROM invite_cards WHERE event_id IN " + in:     &doc.InviteCards,
			"SELECT * FROM notification_log WHERE event_id IN " + in: &doc.NotificationLog,
		}
		for query, dest := range eventChildren {
			rows, err := queryRows(ctx, s.db, query, args...)
			if err != nil {
				return nil, fmt.Errorf("export event children: %w", err)
			}
			*dest = rows
		}
	}

	return doc, nil
}

// DeleteOrganizerCascade permanently deletes the organizer and every record
// they own, in a single transaction. Deletion proceeds children-first so the
// operation succeeds regardless of whether foreign-key cascade is enforced.
// Every statement is scoped strictly to the given organizer's data.
func (s *Store) DeleteOrganizerCascade(ctx context.Context, organizerID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Grandchildren / records reachable only through an event the organizer
	// owns. Scoped via a subselect on events.organizer_id so no other
	// organizer's data is ever touched.
	ev := "(SELECT id FROM events WHERE organizer_id = ?)"

	stmts := []struct {
		query string
		args  []any
	}{
		// attendee_answers -> attendees -> events
		{"DELETE FROM attendee_answers WHERE attendee_id IN (SELECT id FROM attendees WHERE event_id IN " + ev + ")", []any{organizerID}},
		// webhook_deliveries -> webhooks -> events
		{"DELETE FROM webhook_deliveries WHERE webhook_id IN (SELECT id FROM webhooks WHERE event_id IN " + ev + ")", []any{organizerID}},
		// event_comments -> events
		{"DELETE FROM event_comments WHERE event_id IN " + ev, []any{organizerID}},
		// notification_log -> events
		{"DELETE FROM notification_log WHERE event_id IN " + ev, []any{organizerID}},
		// messages -> events
		{"DELETE FROM messages WHERE event_id IN " + ev, []any{organizerID}},
		// reminders -> events
		{"DELETE FROM reminders WHERE event_id IN " + ev, []any{organizerID}},
		// webhooks -> events
		{"DELETE FROM webhooks WHERE event_id IN " + ev, []any{organizerID}},
		// event_questions -> events
		{"DELETE FROM event_questions WHERE event_id IN " + ev, []any{organizerID}},
		// invite_cards -> events
		{"DELETE FROM invite_cards WHERE event_id IN " + ev, []any{organizerID}},
		// attendees -> events
		{"DELETE FROM attendees WHERE event_id IN " + ev, []any{organizerID}},
		// cohost rows on the organizer's own events
		{"DELETE FROM event_cohosts WHERE event_id IN " + ev, []any{organizerID}},
		// cohost rows where this organizer is a cohost or the inviter on
		// someone else's event.
		{"DELETE FROM event_cohosts WHERE organizer_id = ? OR added_by = ?", []any{organizerID, organizerID}},
		// events themselves
		{"DELETE FROM events WHERE organizer_id = ?", []any{organizerID}},
		// event series owned by the organizer (events.series_id is ON DELETE
		// SET NULL, and the events are already gone above).
		{"DELETE FROM event_series WHERE organizer_id = ?", []any{organizerID}},
		// auth records tied directly to the organizer
		{"DELETE FROM magic_links WHERE organizer_id = ?", []any{organizerID}},
		{"DELETE FROM sessions WHERE organizer_id = ?", []any{organizerID}},
		{"DELETE FROM account_invites WHERE target_user_id = ? OR invited_by_user_id = ?", []any{organizerID, organizerID}},
		{"DELETE FROM admin_audit_log WHERE actor_user_id = ? OR target_user_id = ?", []any{organizerID, organizerID}},
		{"UPDATE users SET invited_by_user_id = NULL WHERE invited_by_user_id = ?", []any{organizerID}},
		// finally the organizer row itself
		{"DELETE FROM organizers WHERE id = ?", []any{organizerID}},
		{"DELETE FROM users WHERE id = ?", []any{organizerID}},
	}

	for _, st := range stmts {
		if _, err := tx.ExecContext(ctx, st.query, st.args...); err != nil {
			return fmt.Errorf("cascade delete (%s): %w", st.query, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// organizerEventIDs returns the IDs of every event owned by the organizer.
func (s *Store) organizerEventIDs(ctx context.Context, organizerID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id FROM events WHERE organizer_id = ?", organizerID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// inClause builds a parameterised "(?, ?, ...)" IN clause and the matching
// argument slice for the given ids.
func inClause(ids []string) (string, []any) {
	placeholders := make([]byte, 0, len(ids)*2)
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args[i] = id
	}
	return "(" + string(placeholders) + ")", args
}

// queryRowsByOrganizer runs a single-argument query and returns each row as a
// generic map keyed by column name.
func queryRowsByOrganizer(ctx context.Context, db database.DB, query, organizerID string) ([]map[string]any, error) {
	return queryRows(ctx, db, query, organizerID)
}

// queryRows runs an arbitrary query and returns each row as a generic map
// keyed by column name, so export output stays decoupled from the per-domain
// model structs.
func queryRows(ctx context.Context, db database.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = normalizeValue(vals[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// normalizeValue converts driver byte-slice values to strings so the JSON
// export renders text columns as strings rather than base64.
func normalizeValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// scanOrganizer scans a single row into an Organizer.
const userSelect = `SELECT id, email, normalized_email, display_name, timezone,
	instance_role, status, invited_by_user_id, activated_at, last_login_at,
	created_at, updated_at FROM users`

func scanOrganizer(row *sql.Row) (*Organizer, error) {
	var o Organizer
	var createdAt, updatedAt string
	var invitedBy, activatedAt, lastLoginAt sql.NullString

	err := row.Scan(
		&o.ID, &o.Email, &o.NormalizedEmail, &o.Name, &o.Timezone,
		&o.InstanceRole, &o.Status, &invitedBy, &activatedAt, &lastLoginAt,
		&createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan organizer: %w", err)
	}
	o.IsAdmin = o.InstanceRole == InstanceRoleAdmin
	if invitedBy.Valid {
		o.InvitedByUserID = &invitedBy.String
	}
	if activatedAt.Valid {
		t, parseErr := time.Parse(time.RFC3339, activatedAt.String)
		if parseErr != nil {
			return nil, fmt.Errorf("parse activated_at: %w", parseErr)
		}
		o.ActivatedAt = &t
	}
	if lastLoginAt.Valid {
		t, parseErr := time.Parse(time.RFC3339, lastLoginAt.String)
		if parseErr != nil {
			return nil, fmt.Errorf("parse last_login_at: %w", parseErr)
		}
		o.LastLoginAt = &t
	}

	o.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	o.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return &o, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

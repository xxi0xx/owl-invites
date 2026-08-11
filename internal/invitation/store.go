package invitation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xxi0xx/owl-invites/internal/database"
)

var (
	ErrNotFound  = errors.New("invitation not found")
	ErrConflict  = errors.New("response version conflict")
	ErrAllowance = errors.New("additional guest allowance exceeded")
	ErrCapacity  = errors.New("open enrollment capacity exceeded")
)

type Store struct {
	db database.DB
}

func NewStore(db database.DB) *Store { return &Store{db: db} }

// ImportRecord is one prevalidated household to insert as part of an atomic
// import. It intentionally contains no import grouping key.
type ImportRecord struct {
	Invitation *Invitation
	Guests     []*Guest
}

const invitationColumns = `id, event_id, label, contact_email, contact_phone,
	preferred_delivery_method, additional_guest_allowance, source,
	open_enrollment_id, access_id, token_version, created_by_user_id,
	revoked_at, revocation_reason, created_at, updated_at`

func (s *Store) Create(ctx context.Context, invitation *Invitation, guests []*Guest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin invitation create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var email, normalizedEmail, phone, normalizedPhone any
	if invitation.ContactEmail != nil {
		email = *invitation.ContactEmail
		normalizedEmail = normalizeEmail(*invitation.ContactEmail)
	}
	if invitation.ContactPhone != nil {
		phone = *invitation.ContactPhone
		normalizedPhone = normalizePhone(*invitation.ContactPhone)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO invitations (
		id, event_id, label, contact_email, normalized_contact_email,
		contact_phone, normalized_contact_phone, preferred_delivery_method,
		additional_guest_allowance, source, open_enrollment_id, access_id,
		token_version, created_by_user_id, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		invitation.ID, invitation.EventID, invitation.Label, email, normalizedEmail,
		phone, normalizedPhone, invitation.PreferredDeliveryMethod,
		invitation.AdditionalGuestAllowance, invitation.Source,
		invitation.OpenEnrollmentID, invitation.AccessID, invitation.TokenVersion,
		invitation.CreatedByUserID, now, now)
	if err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}

	responseID := uuid.Must(uuid.NewV7()).String()
	_, err = tx.ExecContext(ctx, `INSERT INTO rsvp_responses (
		id, invitation_id, version, created_at, updated_at
	) VALUES (?, ?, 1, ?, ?)`, responseID, invitation.ID, now, now)
	if err != nil {
		return fmt.Errorf("create response: %w", err)
	}

	for _, guest := range guests {
		if _, err = tx.ExecContext(ctx, `INSERT INTO guests (
			id, invitation_id, name, origin, sort_order, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`, guest.ID, invitation.ID, guest.Name,
			guest.Origin, guest.SortOrder, now, now); err != nil {
			return fmt.Errorf("create guest: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO guest_responses (
			id, rsvp_response_id, guest_id, attendance, created_at, updated_at
		) VALUES (?, ?, ?, 'pending', ?, ?)`, uuid.Must(uuid.NewV7()).String(),
			responseID, guest.ID, now, now); err != nil {
			return fmt.Errorf("create guest response: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit invitation create: %w", err)
	}
	parsed, _ := time.Parse(time.RFC3339Nano, now)
	invitation.CreatedAt, invitation.UpdatedAt = parsed, parsed
	for _, guest := range guests {
		guest.InvitationID = invitation.ID
		guest.Attendance = AttendancePending
		guest.CreatedAt, guest.UpdatedAt = parsed, parsed
	}
	return nil
}

// Import creates every household, response, assigned guest, and guest response
// in one transaction. A failure anywhere rolls the complete import back.
func (s *Store) Import(ctx context.Context, records []ImportRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin invitation import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, record := range records {
		invitation := record.Invitation
		var email, normalizedEmail, phone, normalizedPhone any
		if invitation.ContactEmail != nil {
			email = *invitation.ContactEmail
			normalizedEmail = normalizeEmail(*invitation.ContactEmail)
		}
		if invitation.ContactPhone != nil {
			phone = *invitation.ContactPhone
			normalizedPhone = normalizePhone(*invitation.ContactPhone)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO invitations (
			id, event_id, label, contact_email, normalized_contact_email,
			contact_phone, normalized_contact_phone, preferred_delivery_method,
			additional_guest_allowance, source, open_enrollment_id, access_id,
			token_version, created_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			invitation.ID, invitation.EventID, invitation.Label, email, normalizedEmail,
			phone, normalizedPhone, invitation.PreferredDeliveryMethod,
			invitation.AdditionalGuestAllowance, invitation.Source,
			invitation.OpenEnrollmentID, invitation.AccessID, invitation.TokenVersion,
			invitation.CreatedByUserID, now, now)
		if err != nil {
			return fmt.Errorf("import invitation: %w", err)
		}

		responseID := uuid.Must(uuid.NewV7()).String()
		if _, err = tx.ExecContext(ctx, `INSERT INTO rsvp_responses (
			id, invitation_id, version, created_at, updated_at
		) VALUES (?, ?, 1, ?, ?)`, responseID, invitation.ID, now, now); err != nil {
			return fmt.Errorf("import response: %w", err)
		}
		for _, guest := range record.Guests {
			if _, err = tx.ExecContext(ctx, `INSERT INTO guests (
				id, invitation_id, name, origin, sort_order, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)`, guest.ID, invitation.ID, guest.Name,
				guest.Origin, guest.SortOrder, now, now); err != nil {
				return fmt.Errorf("import guest: %w", err)
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO guest_responses (
				id, rsvp_response_id, guest_id, attendance, created_at, updated_at
			) VALUES (?, ?, ?, 'pending', ?, ?)`, uuid.Must(uuid.NewV7()).String(),
				responseID, guest.ID, now, now); err != nil {
				return fmt.Errorf("import guest response: %w", err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit invitation import: %w", err)
	}
	return nil
}

func (s *Store) FindByID(ctx context.Context, id string) (*Invitation, error) {
	return scanInvitation(s.db.QueryRowContext(ctx,
		`SELECT `+invitationColumns+` FROM invitations WHERE id = ?`, id))
}

func (s *Store) FindByIDForEvent(ctx context.Context, id, eventID string) (*Invitation, error) {
	return scanInvitation(s.db.QueryRowContext(ctx,
		`SELECT `+invitationColumns+` FROM invitations WHERE id = ? AND event_id = ?`, id, eventID))
}

func (s *Store) FindByAccessID(ctx context.Context, accessID string) (*Invitation, error) {
	return scanInvitation(s.db.QueryRowContext(ctx,
		`SELECT `+invitationColumns+` FROM invitations WHERE access_id = ?`, accessID))
}

func (s *Store) ListByEvent(ctx context.Context, eventID string) ([]*Household, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+invitationColumns+`
		FROM invitations WHERE event_id = ? ORDER BY created_at, id`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	invitations := make([]*Invitation, 0)
	for rows.Next() {
		inv, scanErr := scanInvitationRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		invitations = append(invitations, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invitations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close invitations: %w", err)
	}

	// Close the list cursor before nested loads. SQLite test and production
	// configurations may intentionally use a single connection.
	result := make([]*Household, 0, len(invitations))
	for _, inv := range invitations {
		household, loadErr := s.LoadHousehold(ctx, inv.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		result = append(result, household)
	}
	return result, nil
}

func (s *Store) ListDeliveryTargets(ctx context.Context, eventID, recipientGroup string) ([]*Invitation, error) {
	query := `SELECT ` + invitationColumns + ` FROM invitations i
		WHERE i.event_id = ? AND i.revoked_at IS NULL AND i.contact_email IS NOT NULL`
	args := []any{eventID}
	if recipientGroup != "all" {
		query += ` AND EXISTS (SELECT 1 FROM guests g
			JOIN guest_responses gr ON gr.guest_id = g.id
			WHERE g.invitation_id = i.id AND g.removed_at IS NULL AND gr.attendance = ?)`
		args = append(args, recipientGroup)
	}
	query += ` ORDER BY i.created_at, i.id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list invitation delivery targets: %w", err)
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

func (s *Store) CreateMessage(ctx context.Context, message *InvitationMessage) error {
	now := time.Now().UTC()
	message.ID = uuid.Must(uuid.NewV7()).String()
	message.CreatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO invitation_messages (
		id, event_id, sender_user_id, recipient_group, subject, body, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)`, message.ID, message.EventID,
		message.SenderUserID, message.RecipientGroup, message.Subject, message.Body,
		now.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create invitation message: %w", err)
	}
	return nil
}

func (s *Store) Rotate(ctx context.Context, id, eventID string) (*Invitation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin rotate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE invitations SET token_version = token_version + 1,
		updated_at = ? WHERE id = ? AND event_id = ? AND revoked_at IS NULL`, now, id, eventID)
	if err != nil {
		return nil, fmt.Errorf("rotate invitation: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return nil, ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `UPDATE invitation_sessions SET revoked_at = ?
		WHERE invitation_id = ? AND revoked_at IS NULL`, now, id); err != nil {
		return nil, fmt.Errorf("revoke rotated sessions: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit rotation: %w", err)
	}
	return s.FindByID(ctx, id)
}

func (s *Store) Revoke(ctx context.Context, id, eventID, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin revoke: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE invitations SET revoked_at = ?,
		revocation_reason = ?, updated_at = ? WHERE id = ? AND event_id = ?
		AND revoked_at IS NULL`, now, reason, now, id, eventID)
	if err != nil {
		return fmt.Errorf("revoke invitation: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `UPDATE invitation_sessions SET revoked_at = ?
		WHERE invitation_id = ? AND revoked_at IS NULL`, now, id); err != nil {
		return fmt.Errorf("revoke invitation sessions: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit revoke: %w", err)
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, invitationID, tokenHash string, tokenVersion int, expiresAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO invitation_sessions (
		id, invitation_id, token_hash, issued_token_version, expires_at, created_at
	) VALUES (?, ?, ?, ?, ?, ?)`, uuid.Must(uuid.NewV7()).String(), invitationID,
		tokenHash, tokenVersion, expiresAt.UTC().Format(time.RFC3339Nano), now)
	if err != nil {
		return fmt.Errorf("create invitation session: %w", err)
	}
	return nil
}

func (s *Store) InvitationForSession(ctx context.Context, tokenHash string) (*Invitation, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	inv, err := scanInvitation(s.db.QueryRowContext(ctx, `SELECT `+prefixedInvitationColumns("i")+`
		FROM invitation_sessions s JOIN invitations i ON i.id = s.invitation_id
		WHERE s.token_hash = ? AND s.expires_at > ? AND s.revoked_at IS NULL
		AND i.revoked_at IS NULL AND s.issued_token_version = i.token_version`, tokenHash, now))
	if err != nil || inv == nil {
		return inv, err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE invitation_sessions SET last_used_at = ?
		WHERE token_hash = ?`, now, tokenHash)
	return inv, nil
}

func prefixedInvitationColumns(alias string) string {
	parts := strings.Split(invitationColumns, ",")
	for i, part := range parts {
		parts[i] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func (s *Store) LoadHousehold(ctx context.Context, invitationID string) (*Household, error) {
	inv, err := s.FindByID(ctx, invitationID)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, ErrNotFound
	}

	eventSummary, err := scanEventSummary(s.db.QueryRowContext(ctx, `SELECT id, title,
		description, event_date, end_date, location, timezone, status
		FROM events WHERE id = ?`, inv.EventID))
	if err != nil {
		return nil, err
	}

	response, err := scanResponse(s.db.QueryRowContext(ctx, `SELECT id,
		invitation_id, version, submitted_at, created_at, updated_at
		FROM rsvp_responses WHERE invitation_id = ?`, invitationID))
	if err != nil {
		return nil, err
	}

	guests, err := s.listGuests(ctx, invitationID)
	if err != nil {
		return nil, err
	}
	questions, err := s.listQuestions(ctx, inv.EventID)
	if err != nil {
		return nil, err
	}
	invAnswers, guestAnswers, err := s.listAnswers(ctx, invitationID)
	if err != nil {
		return nil, err
	}
	return &Household{
		Invitation: inv, Event: eventSummary, Response: response, Guests: guests,
		Questions: questions, InvitationAnswers: invAnswers, GuestAnswers: guestAnswers,
	}, nil
}

func (s *Store) listGuests(ctx context.Context, invitationID string) ([]*Guest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.id, g.invitation_id, g.name,
		g.origin, g.sort_order, COALESCE(gr.attendance, 'pending'), g.removed_at,
		g.created_at, g.updated_at FROM guests g
		LEFT JOIN guest_responses gr ON gr.guest_id = g.id
		WHERE g.invitation_id = ? AND g.removed_at IS NULL
		ORDER BY g.sort_order, g.created_at, g.id`, invitationID)
	if err != nil {
		return nil, fmt.Errorf("list guests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	guests := make([]*Guest, 0)
	for rows.Next() {
		guest, scanErr := scanGuestRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		guests = append(guests, guest)
	}
	return guests, rows.Err()
}

func (s *Store) listQuestions(ctx context.Context, eventID string) ([]*Question, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, label, type, options,
		required, scope, sort_order FROM event_questions
		WHERE event_id = ? AND deleted = 0 ORDER BY sort_order, created_at`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list invitation questions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	questions := make([]*Question, 0)
	for rows.Next() {
		var q Question
		var options string
		var required bool
		if err := rows.Scan(&q.ID, &q.Label, &q.Type, &options, &required, &q.Scope, &q.SortOrder); err != nil {
			return nil, fmt.Errorf("scan invitation question: %w", err)
		}
		q.Required = required
		if err := json.Unmarshal([]byte(options), &q.Options); err != nil {
			return nil, fmt.Errorf("parse question options: %w", err)
		}
		questions = append(questions, &q)
	}
	return questions, rows.Err()
}

func (s *Store) listAnswers(ctx context.Context, invitationID string) ([]Answer, []GuestAnswer, error) {
	invAnswers := make([]Answer, 0)
	rows, err := s.db.QueryContext(ctx, `SELECT question_id, answer
		FROM invitation_answers WHERE invitation_id = ?`, invitationID)
	if err != nil {
		return nil, nil, fmt.Errorf("list invitation answers: %w", err)
	}
	for rows.Next() {
		var a Answer
		if err := rows.Scan(&a.QuestionID, &a.Answer); err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("scan invitation answer: %w", err)
		}
		invAnswers = append(invAnswers, a)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}

	guestAnswers := make([]GuestAnswer, 0)
	rows, err = s.db.QueryContext(ctx, `SELECT ga.guest_id, ga.question_id, ga.answer
		FROM guest_answers ga JOIN guests g ON g.id = ga.guest_id
		WHERE g.invitation_id = ? AND g.removed_at IS NULL`, invitationID)
	if err != nil {
		return nil, nil, fmt.Errorf("list guest answers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var a GuestAnswer
		if err := rows.Scan(&a.GuestID, &a.QuestionID, &a.Answer); err != nil {
			return nil, nil, fmt.Errorf("scan guest answer: %w", err)
		}
		guestAnswers = append(guestAnswers, a)
	}
	return invAnswers, guestAnswers, rows.Err()
}

func scanInvitation(row *sql.Row) (*Invitation, error) {
	var inv Invitation
	var email, phone, openID, creator, revoked, reason sql.NullString
	var created, updated string
	err := row.Scan(&inv.ID, &inv.EventID, &inv.Label, &email, &phone,
		&inv.PreferredDeliveryMethod, &inv.AdditionalGuestAllowance, &inv.Source,
		&openID, &inv.AccessID, &inv.TokenVersion, &creator, &revoked, &reason,
		&created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan invitation: %w", err)
	}
	return parseInvitation(&inv, email, phone, openID, creator, revoked, reason, created, updated)
}

func scanInvitationRows(rows *sql.Rows) (*Invitation, error) {
	var inv Invitation
	var email, phone, openID, creator, revoked, reason sql.NullString
	var created, updated string
	if err := rows.Scan(&inv.ID, &inv.EventID, &inv.Label, &email, &phone,
		&inv.PreferredDeliveryMethod, &inv.AdditionalGuestAllowance, &inv.Source,
		&openID, &inv.AccessID, &inv.TokenVersion, &creator, &revoked, &reason,
		&created, &updated); err != nil {
		return nil, fmt.Errorf("scan invitation row: %w", err)
	}
	return parseInvitation(&inv, email, phone, openID, creator, revoked, reason, created, updated)
}

func parseInvitation(inv *Invitation, email, phone, openID, creator, revoked, reason sql.NullString, created, updated string) (*Invitation, error) {
	inv.ContactEmail = nullString(email)
	inv.ContactPhone = nullString(phone)
	inv.OpenEnrollmentID = nullString(openID)
	inv.CreatedByUserID = nullString(creator)
	inv.RevocationReason = nullString(reason)
	var err error
	if inv.CreatedAt, err = parseDBTime(created); err != nil {
		return nil, err
	}
	if inv.UpdatedAt, err = parseDBTime(updated); err != nil {
		return nil, err
	}
	if revoked.Valid {
		value, parseErr := parseDBTime(revoked.String)
		if parseErr != nil {
			return nil, parseErr
		}
		inv.RevokedAt = &value
	}
	return inv, nil
}

func scanGuestRows(rows *sql.Rows) (*Guest, error) {
	var guest Guest
	var removed sql.NullString
	var created, updated string
	if err := rows.Scan(&guest.ID, &guest.InvitationID, &guest.Name, &guest.Origin,
		&guest.SortOrder, &guest.Attendance, &removed, &created, &updated); err != nil {
		return nil, fmt.Errorf("scan guest: %w", err)
	}
	var err error
	if guest.CreatedAt, err = parseDBTime(created); err != nil {
		return nil, err
	}
	if guest.UpdatedAt, err = parseDBTime(updated); err != nil {
		return nil, err
	}
	if removed.Valid {
		value, parseErr := parseDBTime(removed.String)
		if parseErr != nil {
			return nil, parseErr
		}
		guest.RemovedAt = &value
	}
	return &guest, nil
}

func scanResponse(row *sql.Row) (*Response, error) {
	var response Response
	var submitted sql.NullString
	var created, updated string
	err := row.Scan(&response.ID, &response.InvitationID, &response.Version,
		&submitted, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan response: %w", err)
	}
	var parseErr error
	if response.CreatedAt, parseErr = parseDBTime(created); parseErr != nil {
		return nil, parseErr
	}
	if response.UpdatedAt, parseErr = parseDBTime(updated); parseErr != nil {
		return nil, parseErr
	}
	if submitted.Valid {
		value, err := parseDBTime(submitted.String)
		if err != nil {
			return nil, err
		}
		response.SubmittedAt = &value
	}
	return &response, nil
}

func scanEventSummary(row *sql.Row) (*EventSummary, error) {
	var event EventSummary
	var eventDate string
	var endDate sql.NullString
	if err := row.Scan(&event.ID, &event.Title, &event.Description, &eventDate,
		&endDate, &event.Location, &event.Timezone, &event.Status); err != nil {
		return nil, fmt.Errorf("scan invitation event: %w", err)
	}
	var err error
	if event.EventDate, err = parseDBTime(eventDate); err != nil {
		return nil, err
	}
	if endDate.Valid {
		value, parseErr := parseDBTime(endDate.String)
		if parseErr != nil {
			return nil, parseErr
		}
		event.EndDate = &value
	}
	return &event, nil
}

func parseDBTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse database time: %w", err)
	}
	return parsed, nil
}

func nullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizePhone(value string) string {
	var result strings.Builder
	for i, r := range strings.TrimSpace(value) {
		if r >= '0' && r <= '9' {
			result.WriteRune(r)
		} else if r == '+' && i == 0 {
			result.WriteRune(r)
		}
	}
	return result.String()
}

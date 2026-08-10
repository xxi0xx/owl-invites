package invitation

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const openEnrollmentColumns = `id, event_id, access_id, token_version, enabled,
	opens_at, closes_at, max_party_size, capacity, revoked_at, created_at, updated_at`

func (s *Store) FindOpenByEvent(ctx context.Context, eventID string) (*OpenEnrollmentConfig, error) {
	return scanOpenEnrollment(s.db.QueryRowContext(ctx, `SELECT `+openEnrollmentColumns+`
		FROM open_enrollments WHERE event_id = ?`, eventID))
}

func (s *Store) FindOpenByAccessID(ctx context.Context, accessID string) (*OpenEnrollmentConfig, error) {
	return scanOpenEnrollment(s.db.QueryRowContext(ctx, `SELECT `+openEnrollmentColumns+`
		FROM open_enrollments WHERE access_id = ?`, accessID))
}

func (s *Store) ConfigureOpen(ctx context.Context, config *OpenEnrollmentConfig, creatorUserID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var opensAt, closesAt, capacity any
	if config.OpensAt != nil {
		opensAt = config.OpensAt.UTC().Format(time.RFC3339Nano)
	}
	if config.ClosesAt != nil {
		closesAt = config.ClosesAt.UTC().Format(time.RFC3339Nano)
	}
	if config.Capacity != nil {
		capacity = *config.Capacity
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO open_enrollments (
		id, event_id, access_id, token_version, enabled, opens_at, closes_at,
		max_party_size, capacity, created_by_user_id, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(event_id) DO UPDATE SET enabled = excluded.enabled,
		opens_at = excluded.opens_at, closes_at = excluded.closes_at,
		max_party_size = excluded.max_party_size, capacity = excluded.capacity,
		updated_at = excluded.updated_at`, config.ID, config.EventID, config.AccessID,
		config.TokenVersion, config.Enabled, opensAt, closesAt, config.MaxPartySize,
		capacity, creatorUserID, now, now)
	if err != nil {
		return fmt.Errorf("configure open enrollment: %w", err)
	}
	return nil
}

func (s *Store) RotateOpen(ctx context.Context, eventID string) (*OpenEnrollmentConfig, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE open_enrollments
		SET token_version = token_version + 1, updated_at = ?
		WHERE event_id = ? AND revoked_at IS NULL`, now, eventID)
	if err != nil {
		return nil, fmt.Errorf("rotate open enrollment: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return nil, ErrNotFound
	}
	return s.FindOpenByEvent(ctx, eventID)
}

// EnrollOpen creates a new invitation without performing any contact lookup.
// The locked enrollment row serializes capacity consumption.
func (s *Store) EnrollOpen(ctx context.Context, config *OpenEnrollmentConfig, invitation *Invitation, guests []*Guest, sessionHash string, sessionExpiresAt time.Time) (*Response, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin open enrollment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)

	result, err := tx.ExecContext(ctx, `UPDATE open_enrollments SET updated_at = updated_at
		WHERE id = ? AND access_id = ? AND token_version = ? AND enabled = ?
		AND revoked_at IS NULL`, config.ID, config.AccessID, config.TokenVersion, true)
	if err != nil {
		return nil, fmt.Errorf("lock open enrollment: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return nil, ErrInvalidCapability
	}

	var opensAt, closesAt sql.NullString
	var maxParty int
	var capacity sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT opens_at, closes_at, max_party_size,
		capacity FROM open_enrollments WHERE id = ?`, config.ID).Scan(&opensAt, &closesAt,
		&maxParty, &capacity); err != nil {
		return nil, fmt.Errorf("load open enrollment limits: %w", err)
	}
	if len(guests) < 1 || len(guests) > maxParty {
		return nil, ErrAllowance
	}
	if opensAt.Valid {
		value, err := parseDBTime(opensAt.String)
		if err != nil {
			return nil, err
		}
		if nowTime.Before(value) {
			return nil, ErrInvalidCapability
		}
	}
	if closesAt.Valid {
		value, err := parseDBTime(closesAt.String)
		if err != nil {
			return nil, err
		}
		if !nowTime.Before(value) {
			return nil, ErrInvalidCapability
		}
	}
	if capacity.Valid {
		var allocated int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM guests g
			JOIN invitations i ON i.id = g.invitation_id
			WHERE i.open_enrollment_id = ? AND i.revoked_at IS NULL
			AND g.removed_at IS NULL`, config.ID).Scan(&allocated); err != nil {
			return nil, fmt.Errorf("count open enrollment capacity: %w", err)
		}
		if allocated+len(guests) > int(capacity.Int64) {
			return nil, ErrCapacity
		}
	}

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
		token_version, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 'open', ?, ?, 1, ?, ?)`,
		invitation.ID, invitation.EventID, invitation.Label, email, normalizedEmail,
		phone, normalizedPhone, invitation.PreferredDeliveryMethod, config.ID,
		invitation.AccessID, now, now)
	if err != nil {
		return nil, fmt.Errorf("create open invitation: %w", err)
	}
	responseID := uuid.Must(uuid.NewV7()).String()
	if _, err := tx.ExecContext(ctx, `INSERT INTO rsvp_responses (
		id, invitation_id, version, created_at, updated_at
	) VALUES (?, ?, 1, ?, ?)`, responseID, invitation.ID, now, now); err != nil {
		return nil, fmt.Errorf("create open response: %w", err)
	}
	for _, guest := range guests {
		if _, err := tx.ExecContext(ctx, `INSERT INTO guests (
			id, invitation_id, name, origin, sort_order, created_at, updated_at
		) VALUES (?, ?, ?, 'assigned', ?, ?, ?)`, guest.ID, invitation.ID,
			guest.Name, guest.SortOrder, now, now); err != nil {
			return nil, fmt.Errorf("create open guest: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO guest_responses (
			id, rsvp_response_id, guest_id, attendance, created_at, updated_at
		) VALUES (?, ?, ?, 'pending', ?, ?)`, uuid.Must(uuid.NewV7()).String(),
			responseID, guest.ID, now, now); err != nil {
			return nil, fmt.Errorf("create open guest response: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO invitation_sessions (
		id, invitation_id, token_hash, issued_token_version, expires_at, created_at
	) VALUES (?, ?, ?, 1, ?, ?)`, uuid.Must(uuid.NewV7()).String(), invitation.ID,
		sessionHash, sessionExpiresAt.UTC().Format(time.RFC3339Nano), now); err != nil {
		return nil, fmt.Errorf("create open invitation session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit open enrollment: %w", err)
	}
	invitation.CreatedAt, invitation.UpdatedAt = nowTime, nowTime
	for _, guest := range guests {
		guest.InvitationID = invitation.ID
		guest.Attendance = AttendancePending
		guest.CreatedAt, guest.UpdatedAt = nowTime, nowTime
	}
	return &Response{ID: responseID, InvitationID: invitation.ID, Version: 1,
		CreatedAt: nowTime, UpdatedAt: nowTime}, nil
}

func scanOpenEnrollment(row *sql.Row) (*OpenEnrollmentConfig, error) {
	var config OpenEnrollmentConfig
	var opensAt, closesAt, revokedAt sql.NullString
	var capacity sql.NullInt64
	var created, updated string
	err := row.Scan(&config.ID, &config.EventID, &config.AccessID,
		&config.TokenVersion, &config.Enabled, &opensAt, &closesAt,
		&config.MaxPartySize, &capacity, &revokedAt, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan open enrollment: %w", err)
	}
	var parseErr error
	if config.CreatedAt, parseErr = parseDBTime(created); parseErr != nil {
		return nil, parseErr
	}
	if config.UpdatedAt, parseErr = parseDBTime(updated); parseErr != nil {
		return nil, parseErr
	}
	if opensAt.Valid {
		value, err := parseDBTime(opensAt.String)
		if err != nil {
			return nil, err
		}
		config.OpensAt = &value
	}
	if closesAt.Valid {
		value, err := parseDBTime(closesAt.String)
		if err != nil {
			return nil, err
		}
		config.ClosesAt = &value
	}
	if capacity.Valid {
		value := int(capacity.Int64)
		config.Capacity = &value
	}
	if revokedAt.Valid {
		value, err := parseDBTime(revokedAt.String)
		if err != nil {
			return nil, err
		}
		config.RevokedAt = &value
	}
	return &config, nil
}

package invitation

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/xxi0xx/owl-invites/internal/errcode"
)

func (s *Store) UpdateInvitation(ctx context.Context, eventID, invitationID string, req UpdateInvitationRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin invitation update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var email, normalizedEmail, phone, normalizedPhone any
	if req.ContactEmail != nil {
		email, normalizedEmail = *req.ContactEmail, normalizeEmail(*req.ContactEmail)
	}
	if req.ContactPhone != nil {
		phone, normalizedPhone = *req.ContactPhone, normalizePhone(*req.ContactPhone)
	}
	result, err := tx.ExecContext(ctx, `UPDATE invitations SET label = ?, contact_email = ?,
		normalized_contact_email = ?, contact_phone = ?, normalized_contact_phone = ?,
		preferred_delivery_method = ?, additional_guest_allowance = ?, updated_at = ?
		WHERE id = ? AND event_id = ? AND revoked_at IS NULL`, req.Label, email,
		normalizedEmail, phone, normalizedPhone, req.PreferredDeliveryMethod,
		req.AdditionalGuestAllowance, now, invitationID, eventID)
	if err != nil {
		return fmt.Errorf("update invitation metadata: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrNotFound
	}

	var responseID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM rsvp_responses WHERE invitation_id = ?`, invitationID).Scan(&responseID); err != nil {
		return fmt.Errorf("load invitation response for update: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM guests WHERE invitation_id = ?
		AND origin = 'assigned' AND removed_at IS NULL`, invitationID)
	if err != nil {
		return fmt.Errorf("list assigned guests for update: %w", err)
	}
	existing := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan assigned guest for update: %w", err)
		}
		existing[id] = true
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close assigned guests for update: %w", err)
	}

	kept := make(map[string]bool)
	for index, guest := range req.AssignedGuests {
		if guest.ID == "" {
			guest.ID = uuid.Must(uuid.NewV7()).String()
			if _, err := tx.ExecContext(ctx, `INSERT INTO guests (
				id, invitation_id, name, origin, sort_order, created_at, updated_at
			) VALUES (?, ?, ?, 'assigned', ?, ?, ?)`, guest.ID, invitationID,
				guest.Name, index, now, now); err != nil {
				return fmt.Errorf("add assigned guest: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO guest_responses (
				id, rsvp_response_id, guest_id, attendance, created_at, updated_at
			) VALUES (?, ?, ?, 'pending', ?, ?)`, uuid.Must(uuid.NewV7()).String(),
				responseID, guest.ID, now, now); err != nil {
				return fmt.Errorf("add assigned guest response: %w", err)
			}
		} else {
			if !existing[guest.ID] {
				return errcode.Validationf("assigned guest does not belong to this invitation")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE guests SET name = ?, sort_order = ?,
				updated_at = ? WHERE id = ? AND invitation_id = ? AND origin = 'assigned'
				AND removed_at IS NULL`, guest.Name, index, now, guest.ID, invitationID); err != nil {
				return fmt.Errorf("rename assigned guest: %w", err)
			}
		}
		kept[guest.ID] = true
	}
	for id := range existing {
		if kept[id] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE guests SET removed_at = ?, updated_at = ?
			WHERE id = ? AND invitation_id = ? AND origin = 'assigned'
			AND removed_at IS NULL`, now, now, id, invitationID); err != nil {
			return fmt.Errorf("soft-remove assigned guest: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit invitation update: %w", err)
	}
	return nil
}

func (s *Store) LoadOrganizerHousehold(ctx context.Context, invitationID string) (*Household, error) {
	household, err := s.LoadHousehold(ctx, invitationID)
	if err != nil {
		return nil, err
	}
	household.LatestDelivery, err = s.loadLatestDelivery(ctx, invitationID)
	return household, err
}

func (s *Store) loadLatestDelivery(ctx context.Context, invitationID string) (*DeliverySummary, error) {
	var summary DeliverySummary
	var sentAt sql.NullString
	var attemptedAt string
	err := s.db.QueryRowContext(ctx, `SELECT status, delivery_status, provider,
		COALESCE(error, ''), created_at, sent_at FROM notification_log
		WHERE invitation_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`, invitationID).
		Scan(&summary.Status, &summary.DeliveryStatus, &summary.Provider, &summary.Error,
			&attemptedAt, &sentAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load latest invitation delivery: %w", err)
	}
	summary.AttemptedAt, err = parseDBTime(attemptedAt)
	if err != nil {
		return nil, err
	}
	if sentAt.Valid {
		value, parseErr := parseDBTime(sentAt.String)
		if parseErr != nil {
			return nil, parseErr
		}
		summary.SentAt = &value
	}
	return &summary, nil
}

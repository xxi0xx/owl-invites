package invitation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/errcode"
)

// SubmitResponse atomically applies a complete household response. The
// optimistic version update and invitation row lock serialize allowance checks
// across both supported database engines.
func (s *Store) SubmitResponse(ctx context.Context, invitationID string, req SubmitRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin response: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	lockResult, err := tx.ExecContext(ctx, `UPDATE invitations SET updated_at = updated_at
		WHERE id = ? AND revoked_at IS NULL`, invitationID)
	if err != nil {
		return fmt.Errorf("lock invitation: %w", err)
	}
	locked, _ := lockResult.RowsAffected()
	if locked != 1 {
		return ErrNotFound
	}

	var eventID string
	var allowance int
	if err := tx.QueryRowContext(ctx, `SELECT event_id, additional_guest_allowance
		FROM invitations WHERE id = ?`, invitationID).Scan(&eventID, &allowance); err != nil {
		return fmt.Errorf("load invitation allowance: %w", err)
	}
	if len(req.AdditionalGuests) > allowance {
		return ErrAllowance
	}

	result, err := tx.ExecContext(ctx, `UPDATE rsvp_responses SET version = version + 1,
		submitted_at = COALESCE(submitted_at, ?), updated_at = ?
		WHERE invitation_id = ? AND version = ?`, now, now, invitationID, req.Version)
	if err != nil {
		return fmt.Errorf("advance response version: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrConflict
	}

	var responseID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM rsvp_responses
		WHERE invitation_id = ?`, invitationID).Scan(&responseID); err != nil {
		return fmt.Errorf("load response id: %w", err)
	}

	existing, err := loadActiveGuestsTx(ctx, tx, invitationID)
	if err != nil {
		return err
	}
	assigned := make(map[string]*Guest)
	additional := make(map[string]*Guest)
	for _, guest := range existing {
		if guest.Origin == GuestOriginAssigned {
			assigned[guest.ID] = guest
		} else {
			additional[guest.ID] = guest
		}
	}

	for _, update := range req.AssignedGuests {
		if assigned[update.GuestID] == nil {
			return errcode.Validationf("assigned guest does not belong to this invitation")
		}
		if !validAttendance(update.Attendance) {
			return errcode.Validationf("invalid attendance")
		}
		if err := updateGuestAttendance(ctx, tx, responseID, update.GuestID, update.Attendance, now); err != nil {
			return err
		}
	}

	keptAdditional := make(map[string]bool)
	for i := range req.AdditionalGuests {
		input := &req.AdditionalGuests[i]
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return errcode.Validationf("additional guest name is required")
		}
		if len(name) > 200 {
			return errcode.Validationf("additional guest name must be 200 characters or fewer")
		}
		if !validAttendance(input.Attendance) {
			return errcode.Validationf("invalid attendance")
		}

		guestID := input.ID
		if guestID == "" {
			guestID = uuid.Must(uuid.NewV7()).String()
			input.ID = guestID
			if _, err := tx.ExecContext(ctx, `INSERT INTO guests (
				id, invitation_id, name, origin, sort_order, created_at, updated_at
			) VALUES (?, ?, ?, 'additional', ?, ?, ?)`, guestID, invitationID,
				name, len(assigned)+i, now, now); err != nil {
				return fmt.Errorf("create additional guest: %w", err)
			}
		} else {
			if additional[guestID] == nil {
				return errcode.Validationf("additional guest does not belong to this invitation")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE guests SET name = ?, updated_at = ?
				WHERE id = ? AND invitation_id = ? AND origin = 'additional'
				AND removed_at IS NULL`, name, now, guestID, invitationID); err != nil {
				return fmt.Errorf("rename additional guest: %w", err)
			}
		}
		keptAdditional[guestID] = true
		if err := updateGuestAttendance(ctx, tx, responseID, guestID, input.Attendance, now); err != nil {
			return err
		}
	}

	for guestID := range additional {
		if keptAdditional[guestID] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE guests SET removed_at = ?, updated_at = ?
			WHERE id = ? AND invitation_id = ? AND origin = 'additional'
			AND removed_at IS NULL`, now, now, guestID, invitationID); err != nil {
			return fmt.Errorf("remove additional guest: %w", err)
		}
	}

	activeGuests, err := loadActiveGuestsTx(ctx, tx, invitationID)
	if err != nil {
		return err
	}
	questions, err := loadQuestionsTx(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if err := validateResponseAnswers(questions, activeGuests, req); err != nil {
		return err
	}
	if err := replaceAnswers(ctx, tx, invitationID, activeGuests, req, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit response: %w", err)
	}
	return nil
}

func updateGuestAttendance(ctx context.Context, tx database.Tx, responseID, guestID, attendance, now string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO guest_responses (
		id, rsvp_response_id, guest_id, attendance, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(guest_id) DO UPDATE SET attendance = excluded.attendance,
		updated_at = excluded.updated_at`, uuid.Must(uuid.NewV7()).String(), responseID,
		guestID, attendance, now, now)
	if err != nil {
		return fmt.Errorf("update guest attendance: %w", err)
	}
	return nil
}

func loadActiveGuestsTx(ctx context.Context, tx database.Tx, invitationID string) ([]*Guest, error) {
	rows, err := tx.QueryContext(ctx, `SELECT g.id, g.invitation_id, g.name, g.origin,
		g.sort_order, COALESCE(gr.attendance, 'pending'), g.removed_at,
		g.created_at, g.updated_at FROM guests g
		LEFT JOIN guest_responses gr ON gr.guest_id = g.id
		WHERE g.invitation_id = ? AND g.removed_at IS NULL`, invitationID)
	if err != nil {
		return nil, fmt.Errorf("load active guests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]*Guest, 0)
	for rows.Next() {
		guest, err := scanGuestRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, guest)
	}
	return result, rows.Err()
}

func loadQuestionsTx(ctx context.Context, tx database.Tx, eventID string) ([]*Question, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, label, type, options, required,
		scope, sort_order FROM event_questions WHERE event_id = ? AND deleted = 0`, eventID)
	if err != nil {
		return nil, fmt.Errorf("load response questions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]*Question, 0)
	for rows.Next() {
		var q Question
		var options string
		var required bool
		if err := rows.Scan(&q.ID, &q.Label, &q.Type, &options, &required,
			&q.Scope, &q.SortOrder); err != nil {
			return nil, fmt.Errorf("scan response question: %w", err)
		}
		q.Required = required
		if err := json.Unmarshal([]byte(options), &q.Options); err != nil {
			return nil, fmt.Errorf("parse response question options: %w", err)
		}
		result = append(result, &q)
	}
	return result, rows.Err()
}

func validateResponseAnswers(questions []*Question, guests []*Guest, req SubmitRequest) error {
	questionByID := make(map[string]*Question, len(questions))
	for _, q := range questions {
		questionByID[q.ID] = q
		if q.Scope == QuestionScopeInvitation && q.Required && strings.TrimSpace(req.InvitationAnswers[q.ID]) == "" {
			return errcode.Validationf("answer required for question: %s", q.Label)
		}
	}

	guestByID := make(map[string]*Guest, len(guests))
	for _, guest := range guests {
		guestByID[guest.ID] = guest
		if guest.Attendance != AttendanceAttending {
			continue
		}
		for _, q := range questions {
			if q.Scope == QuestionScopeGuest && q.Required && strings.TrimSpace(req.GuestAnswers[guest.ID][q.ID]) == "" {
				return errcode.Validationf("answer required for %s: %s", guest.Name, q.Label)
			}
		}
	}

	for questionID, answer := range req.InvitationAnswers {
		q := questionByID[questionID]
		if q == nil || q.Scope != QuestionScopeInvitation {
			return errcode.Validationf("invalid invitation-scoped question")
		}
		if err := validateAnswer(q, answer); err != nil {
			return err
		}
	}
	for guestID, answers := range req.GuestAnswers {
		if guestByID[guestID] == nil {
			return errcode.Validationf("guest answer does not belong to this invitation")
		}
		for questionID, answer := range answers {
			q := questionByID[questionID]
			if q == nil || q.Scope != QuestionScopeGuest {
				return errcode.Validationf("invalid guest-scoped question")
			}
			if err := validateAnswer(q, answer); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAnswer(q *Question, answer string) error {
	if len(answer) > 1000 {
		return errcode.Validationf("answer for %q exceeds 1000 characters", q.Label)
	}
	switch q.Type {
	case "text":
		return nil
	case "select":
		if answer == "" {
			return nil
		}
		for _, option := range q.Options {
			if option == answer {
				return nil
			}
		}
		return errcode.Validationf("invalid option for %q", q.Label)
	case "checkbox":
		if answer == "" {
			return nil
		}
		var selected []string
		if err := json.Unmarshal([]byte(answer), &selected); err != nil {
			return errcode.Validationf("checkbox answer for %q must be an array", q.Label)
		}
		allowed := make(map[string]bool, len(q.Options))
		for _, option := range q.Options {
			allowed[option] = true
		}
		for _, option := range selected {
			if !allowed[option] {
				return errcode.Validationf("invalid option for %q", q.Label)
			}
		}
		return nil
	default:
		return errcode.Validationf("invalid question type")
	}
}

func replaceAnswers(ctx context.Context, tx database.Tx, invitationID string, guests []*Guest, req SubmitRequest, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM invitation_answers
		WHERE invitation_id = ?`, invitationID); err != nil {
		return fmt.Errorf("replace invitation answers: %w", err)
	}
	for questionID, answer := range req.InvitationAnswers {
		if _, err := tx.ExecContext(ctx, `INSERT INTO invitation_answers (
			id, invitation_id, question_id, answer, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)`, uuid.Must(uuid.NewV7()).String(), invitationID,
			questionID, answer, now, now); err != nil {
			return fmt.Errorf("save invitation answer: %w", err)
		}
	}

	guestIDs := make(map[string]bool, len(guests))
	for _, guest := range guests {
		guestIDs[guest.ID] = true
		if _, err := tx.ExecContext(ctx, `DELETE FROM guest_answers WHERE guest_id = ?`, guest.ID); err != nil {
			return fmt.Errorf("replace guest answers: %w", err)
		}
	}
	for guestID, answers := range req.GuestAnswers {
		if !guestIDs[guestID] {
			return errcode.Validationf("guest answer does not belong to this invitation")
		}
		for questionID, answer := range answers {
			if _, err := tx.ExecContext(ctx, `INSERT INTO guest_answers (
				id, guest_id, question_id, answer, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?)`, uuid.Must(uuid.NewV7()).String(), guestID,
				questionID, answer, now, now); err != nil {
				return fmt.Errorf("save guest answer: %w", err)
			}
		}
	}
	return nil
}

func validAttendance(value string) bool {
	switch value {
	case AttendancePending, AttendanceAttending, AttendanceMaybe, AttendanceDeclined:
		return true
	default:
		return false
	}
}

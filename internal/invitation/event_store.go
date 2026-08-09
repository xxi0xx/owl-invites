package invitation

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *Store) EventExists(ctx context.Context, eventID string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM events WHERE id = ?`, eventID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find recovery event: %w", err)
	}
	return true, nil
}

func (s *Store) EventSummary(ctx context.Context, eventID string) (*EventSummary, error) {
	return scanEventSummary(s.db.QueryRowContext(ctx, `SELECT id, title,
		description, event_date, end_date, location, timezone, status
		FROM events WHERE id = ?`, eventID))
}

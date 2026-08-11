package stats

import (
	"context"
	"fmt"

	"github.com/xxi0xx/owl-invites/internal/database"
)

// Store handles database queries for instance-wide aggregate statistics.
type Store struct {
	db database.DB
}

// NewStore creates a new stats Store.
func NewStore(db database.DB) *Store {
	return &Store{db: db}
}

// GetInstanceStats returns aggregate statistics across the entire instance.
// All queries return only aggregate counts — no individual records or PII.
func (s *Store) GetInstanceStats(ctx context.Context) (*InstanceStats, error) {
	stats := &InstanceStats{}

	if err := s.loadEventStats(ctx, &stats.Events); err != nil {
		return nil, fmt.Errorf("event stats: %w", err)
	}

	if err := s.loadGuestStats(ctx, &stats.Guests); err != nil {
		return nil, fmt.Errorf("guest stats: %w", err)
	}

	if err := s.loadUserStats(ctx, &stats.Users); err != nil {
		return nil, fmt.Errorf("user stats: %w", err)
	}

	if err := s.loadFeatureAdoption(ctx, &stats.Features); err != nil {
		return nil, fmt.Errorf("feature stats: %w", err)
	}

	if err := s.loadNotificationStats(ctx, &stats.Notifications); err != nil {
		return nil, fmt.Errorf("notification stats: %w", err)
	}

	return stats, nil
}

func (s *Store) loadEventStats(ctx context.Context, out *EventStats) error {
	rows, err := s.db.QueryContext(ctx, "SELECT status, COUNT(*) FROM events GROUP BY status")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return err
		}
		out.Total += count
		switch status {
		case "draft":
			out.Draft = count
		case "published":
			out.Published = count
		case "cancelled":
			out.Cancelled = count
		case "archived":
			out.Archived = count
		}
	}
	return rows.Err()
}

func (s *Store) loadGuestStats(ctx context.Context, out *GuestStats) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(gr.attendance, 'pending'), COUNT(*) FROM guests g
		 LEFT JOIN guest_responses gr ON gr.guest_id = g.id
		 WHERE g.removed_at IS NULL GROUP BY COALESCE(gr.attendance, 'pending')`,
	)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return err
		}
		out.Total += count
		out.TotalHeadcount += count
		switch status {
		case "attending":
			out.Attending = count
		case "maybe":
			out.Maybe = count
		case "declined":
			out.Declined = count
		case "pending":
			out.Pending = count
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	var eventsWithGuests int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT i.event_id) FROM guests g
		 JOIN invitations i ON i.id = g.invitation_id WHERE g.removed_at IS NULL`,
	).Scan(&eventsWithGuests)
	if err != nil {
		return err
	}
	if eventsWithGuests > 0 {
		out.AvgPerEvent = float64(out.Total) / float64(eventsWithGuests)
	}

	return nil
}

func (s *Store) loadUserStats(ctx context.Context, out *UserStats) error {
	return s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM users",
	).Scan(&out.Total)
}

func (s *Store) loadFeatureAdoption(ctx context.Context, out *FeatureAdoption) error {
	// Each query is a simple COUNT — portable across SQLite and PostgreSQL.
	queries := []struct {
		query string
		dest  *int
	}{
		{"SELECT COUNT(*) FROM open_enrollments WHERE enabled = ?", &out.OpenEnrollmentEvents},
		{"SELECT COUNT(DISTINCT event_id) FROM event_memberships WHERE role = 'cohost'", &out.CohostedEvents},
		{"SELECT COUNT(DISTINCT event_id) FROM event_questions", &out.EventsWithQuestions},
		{"SELECT COUNT(*) FROM open_enrollments WHERE capacity IS NOT NULL", &out.EventsWithCapacity},
		{"SELECT COUNT(*) FROM events WHERE series_id IS NOT NULL", &out.SeriesEvents},
	}

	for _, q := range queries {
		var err error
		if q.query == queries[0].query {
			var enabled any = 1
			if s.db.Dialect() == "postgres" {
				enabled = true
			}
			err = s.db.QueryRowContext(ctx, q.query, enabled).Scan(q.dest)
		} else {
			err = s.db.QueryRowContext(ctx, q.query).Scan(q.dest)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) loadNotificationStats(ctx context.Context, out *NotificationStats) error {
	rows, err := s.db.QueryContext(ctx,
		"SELECT status, delivery_status, COUNT(*) FROM notification_log GROUP BY status, delivery_status",
	)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var status, deliveryStatus string
		var count int
		if err := rows.Scan(&status, &deliveryStatus, &count); err != nil {
			return err
		}
		out.Total += count
		switch status {
		case "sent":
			out.Sent += count
		case "failed":
			out.Failed += count
		}
		switch deliveryStatus {
		case "delivered":
			out.Delivered += count
		case "opened":
			out.Opened += count
		case "bounced":
			out.Bounced += count
		case "complained":
			out.Complained += count
		}
	}
	return rows.Err()
}

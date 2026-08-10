package stats

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/testutil"
)

const statsNow = "2026-01-01T00:00:00Z"

func seedStatsEvent(t *testing.T, db database.DB, id, status string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO events (id, title, event_date, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`, id, "Event "+id, statsNow, status, statsNow, statsNow)
	require.NoError(t, err)
}

func seedStatsInvitation(t *testing.T, db database.DB, eventID, invitationID string, attendances ...string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `INSERT INTO invitations (
		id, event_id, label, preferred_delivery_method, additional_guest_allowance,
		source, access_id, token_version, created_at, updated_at
	) VALUES (?, ?, ?, 'none', 0, 'private', ?, 1, ?, ?)`,
		invitationID, eventID, "Household "+invitationID, "access-"+invitationID, statsNow, statsNow)
	require.NoError(t, err)
	responseID := "response-" + invitationID
	_, err = db.ExecContext(context.Background(), `INSERT INTO rsvp_responses (
		id, invitation_id, version, submitted_at, created_at, updated_at
	) VALUES (?, ?, 1, ?, ?, ?)`, responseID, invitationID, statsNow, statsNow, statsNow)
	require.NoError(t, err)
	for index, attendance := range attendances {
		guestID := fmt.Sprintf("guest-%s-%d", invitationID, index)
		_, err = db.ExecContext(context.Background(), `INSERT INTO guests (
			id, invitation_id, name, origin, sort_order, created_at, updated_at
		) VALUES (?, ?, ?, 'assigned', ?, ?, ?)`, guestID, invitationID, guestID, index, statsNow, statsNow)
		require.NoError(t, err)
		if attendance != "pending" {
			_, err = db.ExecContext(context.Background(), `INSERT INTO guest_responses (
				id, rsvp_response_id, guest_id, attendance, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?)`, "guest-response-"+guestID, responseID, guestID, attendance, statsNow, statsNow)
			require.NoError(t, err)
		}
	}
}

func TestGetInstanceStatsEmpty(t *testing.T) {
	stats, err := NewStore(testutil.NewTestDB(t)).GetInstanceStats(context.Background())
	require.NoError(t, err)
	assert.Zero(t, stats.Events.Total)
	assert.Zero(t, stats.Guests.Total)
	assert.Zero(t, stats.Users.Total)
	assert.Zero(t, stats.Notifications.Total)
}

func TestGetInstanceStatsUsesInvitationGuests(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedUser(t, db, "owner-stats", "owner-stats@example.com", "Owner")
	seedStatsEvent(t, db, "stats-event-1", "published")
	seedStatsEvent(t, db, "stats-event-2", "draft")
	seedStatsEvent(t, db, "stats-event-3", "cancelled")
	seedStatsInvitation(t, db, "stats-event-1", "stats-invitation-1", "attending", "maybe")
	seedStatsInvitation(t, db, "stats-event-2", "stats-invitation-2", "declined", "pending", "attending")

	stats, err := NewStore(db).GetInstanceStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Events.Total)
	assert.Equal(t, 1, stats.Events.Published)
	assert.Equal(t, 1, stats.Events.Draft)
	assert.Equal(t, 1, stats.Events.Cancelled)
	assert.Equal(t, 5, stats.Guests.Total)
	assert.Equal(t, 5, stats.Guests.TotalHeadcount)
	assert.Equal(t, 2, stats.Guests.Attending)
	assert.Equal(t, 1, stats.Guests.Maybe)
	assert.Equal(t, 1, stats.Guests.Declined)
	assert.Equal(t, 1, stats.Guests.Pending)
	assert.Equal(t, 2.5, stats.Guests.AvgPerEvent)
	assert.Equal(t, 1, stats.Users.Total)
}

func TestGetInstanceStatsFeatureAdoption(t *testing.T) {
	db := testutil.NewTestDB(t)
	testutil.SeedUser(t, db, "owner-features", "owner-features@example.com", "Owner")
	testutil.SeedUser(t, db, "cohost-features", "cohost-features@example.com", "Cohost")
	seedStatsEvent(t, db, "feature-event", "published")
	testutil.SeedEventOwner(t, db, "feature-event", "owner-features")
	_, err := db.ExecContext(context.Background(), `INSERT INTO event_memberships (
		id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
	) VALUES ('feature-cohost-membership', 'feature-event', 'cohost-features', 'cohost',
		'owner-features', ?, ?)`, statsNow, statsNow)
	require.NoError(t, err)
	var enabled any = 1
	if db.Dialect() == "postgres" {
		enabled = true
	}
	_, err = db.ExecContext(context.Background(), `INSERT INTO open_enrollments (
		id, event_id, access_id, token_version, enabled, max_party_size, capacity,
		created_by_user_id, created_at, updated_at
	) VALUES ('feature-open', 'feature-event', 'feature-open-access', 1, ?, 4, 25,
		'owner-features', ?, ?)`, enabled, statsNow, statsNow)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `INSERT INTO event_questions (
		id, event_id, label, type, options, required, scope, sort_order, deleted, created_at, updated_at
	) VALUES ('feature-question', 'feature-event', 'Dietary needs?', 'text', '[]', 0,
		'guest', 0, 0, ?, ?)`, statsNow, statsNow)
	require.NoError(t, err)

	stats, err := NewStore(db).GetInstanceStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Features.OpenEnrollmentEvents)
	assert.Equal(t, 1, stats.Features.EventsWithCapacity)
	assert.Equal(t, 1, stats.Features.CohostedEvents)
	assert.Equal(t, 1, stats.Features.EventsWithQuestions)
}

func TestGetInstanceStatsNotificationOwnership(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedStatsEvent(t, db, "notification-event", "published")
	seedStatsInvitation(t, db, "notification-event", "notification-invitation", "attending")
	for index, row := range []struct{ status, delivery string }{
		{"sent", "delivered"}, {"sent", "opened"}, {"sent", "bounced"}, {"failed", "unknown"},
	} {
		_, err := db.ExecContext(context.Background(), `INSERT INTO notification_log (
			id, event_id, invitation_id, channel, provider, status, delivery_status,
			recipient, subject, created_at
		) VALUES (?, 'notification-event', 'notification-invitation', 'email', 'smtp',
			?, ?, 'guest@example.com', 'Subject', ?)`, fmt.Sprintf("notification-%d", index), row.status, row.delivery, statsNow)
		require.NoError(t, err)
	}

	stats, err := NewStore(db).GetInstanceStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 4, stats.Notifications.Total)
	assert.Equal(t, 3, stats.Notifications.Sent)
	assert.Equal(t, 1, stats.Notifications.Failed)
	assert.Equal(t, 1, stats.Notifications.Delivered)
	assert.Equal(t, 1, stats.Notifications.Opened)
	assert.Equal(t, 1, stats.Notifications.Bounced)
}

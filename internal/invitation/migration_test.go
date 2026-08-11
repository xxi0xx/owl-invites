package invitation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/database"
	"github.com/xxi0xx/owl-invites/internal/testutil"
)

// This test runs unchanged against SQLite locally and PostgreSQL in CI when
// TEST_DATABASE_URL is set. It seeds the exact Gate 1 schema before applying
// the Gate 2 migration, rather than merely testing a fresh install.
func TestLegacyAttendeeMigrationCreatesOneInvitationAndNoPlaceholderGuests(t *testing.T) {
	db := testutil.NewTestDBAtVersion(t, 33)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ownerID := "legacy-owner"
	eventID := "legacy-event"
	attendeeID := "legacy-attendee"
	seriesID := "legacy-series"

	_, err := db.ExecContext(ctx, `INSERT INTO organizers (
		id, email, name, timezone, is_admin, created_at, updated_at
	) VALUES (?, 'owner@example.com', 'Owner', 'UTC', ?, ?, ?)`, ownerID, false, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO users (
		id, email, normalized_email, display_name, timezone, instance_role,
		status, activated_at, created_at, updated_at
	) VALUES (?, 'owner@example.com', 'owner@example.com', 'Owner', 'UTC',
		'user', 'active', ?, ?, ?)`, ownerID, now, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO magic_links (
		id, token_hash, organizer_id, expires_at, created_at
	) VALUES ('legacy-magic-link', 'legacy-magic-hash', ?, ?, ?)`, ownerID,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO sessions (
		id, token_hash, organizer_id, expires_at, created_at
	) VALUES ('legacy-session', 'legacy-session-hash', ?, ?, ?)`, ownerID,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO event_series (
		id, organizer_id, title, event_time, recurrence_rule, created_at, updated_at
	) VALUES (?, ?, 'Legacy Series', '18:00', 'weekly', ?, ?)`, seriesID, ownerID, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO events (
		id, organizer_id, title, description, event_date, location, timezone,
		retention_days, status, share_token, series_id, created_at, updated_at
	) VALUES (?, ?, 'Legacy Event', '', ?, '', 'UTC', 30, 'published',
		'legacy-share-token', ?, ?, ?)`, eventID, ownerID,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), seriesID, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO event_memberships (
		id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
	) VALUES ('legacy-membership', ?, ?, 'owner', ?, ?, ?)`, eventID, ownerID, ownerID, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO attendees (
		id, event_id, name, email, phone, rsvp_status, rsvp_token,
		contact_method, dietary_notes, plus_ones, created_at, updated_at
	) VALUES (?, ?, 'Legacy Guest', 'Shared@Example.com', NULL, 'waitlisted',
		'legacy-random-rsvp-selector', 'email', 'not reconstructable as a scoped answer',
		2, ?, ?)`, attendeeID, eventID, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO event_questions (
		id, event_id, label, type, options, required, sort_order, deleted,
		created_at, updated_at
	) VALUES ('legacy-question', ?, 'Meal', 'text', '[]', 0, 0, 0, ?, ?)`, eventID, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO attendee_answers (
		id, attendee_id, question_id, answer, created_at, updated_at
	) VALUES ('legacy-answer', ?, 'legacy-question', 'Vegetarian', ?, ?)`, attendeeID, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO notification_log (
		id, event_id, attendee_id, channel, provider, status, recipient, subject,
		delivery_status, created_at
	) VALUES ('legacy-notification', ?, ?, 'email', 'smtp', 'sent',
		'guest@example.com', 'Legacy notice', 'delivered', ?)`, eventID, attendeeID, now)
	require.NoError(t, err)

	require.NoError(t, database.RunMigrationsTo(db, 35))

	// Seed a live invitation session after the one-way mapping and identity
	// shadow removal. SQLite migration 36 rebuilds events and must preserve this
	// Gate 2 child rather than relying only on fresh-install behavior.
	storeAt35 := NewStore(db)
	require.NoError(t, storeAt35.CreateSession(ctx, "legacy-invitation:"+attendeeID,
		hashToken("preserved-invitation-session"), 1, time.Now().UTC().Add(time.Hour)))
	require.NoError(t, database.RunMigrationsTo(db, 36))

	var migratedUserID string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT user_id FROM magic_links WHERE id = 'legacy-magic-link'").Scan(&migratedUserID))
	assert.Equal(t, ownerID, migratedUserID)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT user_id FROM sessions WHERE id = 'legacy-session'").Scan(&migratedUserID))
	assert.Equal(t, ownerID, migratedUserID)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT owner_user_id FROM event_series WHERE id = ?", seriesID).Scan(&migratedUserID))
	assert.Equal(t, ownerID, migratedUserID)
	var migratedSeriesID string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT series_id FROM events WHERE id = ?", eventID).Scan(&migratedSeriesID))
	assert.Equal(t, seriesID, migratedSeriesID)
	assert.Error(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM organizers").Scan(new(int)), "organizers shadow must be removed")
	assert.Error(t, db.QueryRowContext(ctx, "SELECT organizer_id FROM events LIMIT 1").Scan(new(string)), "events.organizer_id shadow must be removed")

	store := NewStore(db)
	household, err := store.LoadHousehold(ctx, "legacy-invitation:"+attendeeID)
	require.NoError(t, err)
	assert.Equal(t, SourcePrivate, household.Invitation.Source)
	assert.Equal(t, 2, household.Invitation.AdditionalGuestAllowance)
	assert.Equal(t, "legacy-random-rsvp-selector", household.Invitation.AccessID)
	require.NotNil(t, household.Invitation.ContactEmail)
	assert.Equal(t, "Shared@Example.com", *household.Invitation.ContactEmail)
	require.Len(t, household.Guests, 1, "numeric plus-ones must not create unnamed placeholder guests")
	assert.Equal(t, GuestOriginAssigned, household.Guests[0].Origin)
	assert.Equal(t, AttendancePending, household.Guests[0].Attendance, "Gate 2 has no waitlist")
	require.Len(t, household.GuestAnswers, 1)
	assert.Equal(t, "Vegetarian", household.GuestAnswers[0].Answer)
	require.Len(t, household.Questions, 1)
	assert.Equal(t, QuestionScopeGuest, household.Questions[0].Scope)

	var notificationInvitationID string
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT invitation_id FROM notification_log WHERE id = 'legacy-notification'").Scan(&notificationInvitationID))
	assert.Equal(t, "legacy-invitation:"+attendeeID, notificationInvitationID)
	var preservedSessionCount int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM invitation_sessions
		WHERE invitation_id = ? AND token_hash = ?`, "legacy-invitation:"+attendeeID,
		hashToken("preserved-invitation-session")).Scan(&preservedSessionCount))
	assert.Equal(t, 1, preservedSessionCount)

	service, err := NewService(store, testSecret, "https://invites.example", time.Hour, 15*time.Minute)
	require.NoError(t, err)
	_, _, err = service.ExchangePrivate(ctx, "legacy-random-rsvp-selector")
	assert.ErrorIs(t, err, ErrInvalidCapability, "legacy token is only a selector and never a capability")

	for _, query := range []string{
		"SELECT COUNT(*) FROM attendees",
		"SELECT COUNT(*) FROM attendee_answers",
		"SELECT COUNT(*) FROM event_comments",
		"SELECT COUNT(*) FROM messages",
		"SELECT share_token FROM events LIMIT 1",
		"SELECT contact_requirement FROM events LIMIT 1",
		"SELECT max_capacity FROM events LIMIT 1",
		"SELECT waitlist_enabled FROM events LIMIT 1",
		"SELECT comments_enabled FROM events LIMIT 1",
		"SELECT contact_requirement FROM event_series LIMIT 1",
		"SELECT max_capacity FROM event_series LIMIT 1",
		"SELECT attendee_id FROM notification_log LIMIT 1",
	} {
		assert.Error(t, db.QueryRowContext(ctx, query).Scan(new(any)), query+" must be removed")
	}
	var invitationMessageCount int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM invitation_messages").Scan(&invitationMessageCount))
	assert.Zero(t, invitationMessageCount)
}

func TestIdentityShadowRemovalRejectsOwnerParityMismatch(t *testing.T) {
	db := testutil.NewTestDBAtVersion(t, 33)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	for _, user := range []struct {
		id, email string
	}{{"parity-user-a", "a@example.com"}, {"parity-user-b", "b@example.com"}} {
		_, err := db.ExecContext(ctx, `INSERT INTO organizers (
			id, email, name, timezone, is_admin, created_at, updated_at
		) VALUES (?, ?, '', 'UTC', ?, ?, ?)`, user.id, user.email, false, now, now)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, `INSERT INTO users (
			id, email, normalized_email, display_name, timezone, instance_role,
			status, activated_at, created_at, updated_at
		) VALUES (?, ?, ?, '', 'UTC', 'user', 'active', ?, ?, ?)`, user.id,
			user.email, user.email, now, now, now)
		require.NoError(t, err)
	}

	_, err := db.ExecContext(ctx, `INSERT INTO events (
		id, organizer_id, title, event_date, status, share_token, created_at, updated_at
	) VALUES ('parity-event', 'parity-user-a', 'Parity Event', ?, 'draft',
		'parity-share', ?, ?)`, now, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO event_memberships (
		id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
	) VALUES ('parity-owner', 'parity-event', 'parity-user-b', 'owner',
		'parity-user-b', ?, ?)`, now, now)
	require.NoError(t, err)

	err = database.RunMigrations(db)
	require.Error(t, err)
	var organizers int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM organizers").Scan(&organizers))
	assert.Equal(t, 2, organizers, "guard must fail before removing compatibility data")
}

func TestMigration36RollbackIsRejectedBeforeSchemaMutation(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	err := database.RunMigrationsTo(db, 35)
	require.ErrorIs(t, err, database.ErrGate2RollbackUnsupported)
	assert.Contains(t, err.Error(), "restore a verified pre-upgrade backup")

	var version uint
	var dirty bool
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
	assert.Equal(t, uint(36), version)
	assert.False(t, dirty)

	var invitationMessageCount int
	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM invitation_messages").Scan(&invitationMessageCount))
	assert.Zero(t, invitationMessageCount)
	assert.Error(t, db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages").Scan(new(int)),
		"unsupported rollback must not recreate any legacy table")
}

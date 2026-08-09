package invitation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/testutil"
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

	_, err := db.ExecContext(ctx, `INSERT INTO organizers (
		id, email, name, timezone, is_admin, created_at, updated_at
	) VALUES (?, 'owner@example.com', 'Owner', 'UTC', 0, ?, ?)`, ownerID, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO users (
		id, email, normalized_email, display_name, timezone, instance_role,
		status, activated_at, created_at, updated_at
	) VALUES (?, 'owner@example.com', 'owner@example.com', 'Owner', 'UTC',
		'user', 'active', ?, ?, ?)`, ownerID, now, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO events (
		id, organizer_id, title, description, event_date, location, timezone,
		retention_days, status, share_token, created_at, updated_at
	) VALUES (?, ?, 'Legacy Event', '', ?, '', 'UTC', 30, 'published',
		'legacy-share-token', ?, ?)`, eventID, ownerID,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), now, now)
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

	require.NoError(t, database.RunMigrations(db))

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
}

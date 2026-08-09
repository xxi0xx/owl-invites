package invitation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yannkr/openrsvp/internal/errcode"
	"github.com/yannkr/openrsvp/internal/testutil"
)

const testSecret = "test-only-owl-invites-secret-key-32-bytes"

type serviceFixture struct {
	t       *testing.T
	store   *Store
	service *Service
	eventID string
	userID  string
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	db := testutil.NewTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	userID := "user-invitation-owner"
	eventID := "event-invitation-domain"
	_, err := db.ExecContext(context.Background(), `INSERT INTO organizers (
		id, email, name, timezone, is_admin, created_at, updated_at
	) VALUES (?, ?, ?, 'UTC', 0, ?, ?)`, userID, "owner@example.com", "Owner", now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `INSERT INTO users (
		id, email, normalized_email, display_name, timezone, instance_role,
		status, activated_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, 'UTC', 'user', 'active', ?, ?, ?)`, userID,
		"owner@example.com", "owner@example.com", "Owner", now, now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `INSERT INTO events (
		id, organizer_id, title, description, event_date, location, timezone,
		retention_days, status, share_token, created_at, updated_at
	) VALUES (?, ?, 'Invitation Test', '', ?, '', 'UTC', 30, 'published', ?, ?, ?)`,
		eventID, userID, time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339),
		"legacy-share-invitation-test", now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `INSERT INTO event_memberships (
		id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
	) VALUES ('membership-invitation-owner', ?, ?, 'owner', ?, ?, ?)`, eventID,
		userID, userID, now, now)
	require.NoError(t, err)

	store := NewStore(db)
	service, err := NewService(store, testSecret, "https://invites.example", 24*time.Hour, 15*time.Minute)
	require.NoError(t, err)
	return &serviceFixture{t: t, store: store, service: service, eventID: eventID, userID: userID}
}

func ptr(value string) *string { return &value }

func (f *serviceFixture) create(label, email string, allowance int, names ...string) *CreateResult {
	f.t.Helper()
	result, err := f.service.CreatePrivate(context.Background(), f.eventID, f.userID, CreateRequest{
		Label: label, ContactEmail: ptr(email), PreferredDeliveryMethod: "email",
		AdditionalGuestAllowance: allowance, AssignedGuestNames: names,
	})
	require.NoError(f.t, err)
	return result
}

func capabilityFromURL(value string) string {
	parts := strings.SplitN(value, "#", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func TestContactIsNotIdentityAndInvitationSessionsAreIsolated(t *testing.T) {
	f := newServiceFixture(t)
	first := f.create("First household", "same@example.com", 1, "Alex", "Bailey")
	second := f.create("Second household", "same@example.com", 0, "Casey")

	items, err := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.NotEqual(t, first.Invitation.ID, second.Invitation.ID)

	sessionA, householdA, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(first.AccessURL))
	require.NoError(t, err)
	assert.Equal(t, first.Invitation.ID, householdA.Invitation.ID)

	_, err = f.service.SubmitForSession(context.Background(), sessionA, SubmitRequest{
		Version:        1,
		AssignedGuests: []GuestAttendanceInput{{GuestID: second.Guests[0].ID, Attendance: AttendanceAttending}},
	})
	require.Error(t, err)
	assert.True(t, errcode.IsValidation(err))

	unchanged, err := f.store.LoadHousehold(context.Background(), second.Invitation.ID)
	require.NoError(t, err)
	assert.Equal(t, AttendancePending, unchanged.Guests[0].Attendance)
}

func TestAllowanceRequiredQuestionsAndOptimisticVersion(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Household", "household@example.com", 1, "Assigned One", "Assigned Two")
	_, err := f.store.db.ExecContext(context.Background(), `INSERT INTO event_questions (
		id, event_id, label, type, options, required, sort_order, deleted, created_at, updated_at, scope
	) VALUES ('guest-meal', ?, 'Meal', 'select', '["Fish","Veg"]', 1, 0, 0, ?, ?, 'guest')`,
		f.eventID, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)

	session, _, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(created.AccessURL))
	require.NoError(t, err)
	_, err = f.service.SubmitForSession(context.Background(), session, SubmitRequest{
		Version: 1,
		AssignedGuests: []GuestAttendanceInput{
			{GuestID: created.Guests[0].ID, Attendance: AttendanceAttending},
			{GuestID: created.Guests[1].ID, Attendance: AttendanceDeclined},
		},
		AdditionalGuests: []AdditionalGuestInput{{Name: "Allowed Guest", Attendance: AttendanceDeclined}},
	})
	require.Error(t, err)
	assert.True(t, errcode.IsValidation(err), err)

	updated, err := f.service.SubmitForSession(context.Background(), session, SubmitRequest{
		Version: 1,
		AssignedGuests: []GuestAttendanceInput{
			{GuestID: created.Guests[0].ID, Attendance: AttendanceAttending},
			{GuestID: created.Guests[1].ID, Attendance: AttendanceDeclined},
		},
		AdditionalGuests: []AdditionalGuestInput{{Name: "Allowed Guest", Attendance: AttendanceDeclined}},
		GuestAnswers: map[string]map[string]string{
			created.Guests[0].ID: {"guest-meal": "Veg"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Response.Version)
	require.Len(t, updated.Guests, 3)

	_, err = f.service.SubmitForSession(context.Background(), session, SubmitRequest{
		Version:          1,
		AdditionalGuests: []AdditionalGuestInput{{Name: "One", Attendance: AttendanceDeclined}},
	})
	assert.ErrorIs(t, err, ErrConflict)

	_, err = f.service.SubmitForSession(context.Background(), session, SubmitRequest{
		Version: 2,
		AdditionalGuests: []AdditionalGuestInput{
			{ID: updated.Guests[2].ID, Name: "Allowed Guest", Attendance: AttendanceDeclined},
			{Name: "Too Many", Attendance: AttendanceDeclined},
		},
	})
	assert.ErrorIs(t, err, ErrAllowance)
}

func TestRotationAndRevocationInvalidateCapabilitiesAndSessions(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Rotate me", "rotate@example.com", 0, "Guest")
	oldCapability := capabilityFromURL(created.AccessURL)
	oldSession, _, err := f.service.ExchangePrivate(context.Background(), oldCapability)
	require.NoError(t, err)

	rotated, err := f.service.Rotate(context.Background(), f.eventID, created.Invitation.ID)
	require.NoError(t, err)
	_, _, err = f.service.ExchangePrivate(context.Background(), oldCapability)
	assert.ErrorIs(t, err, ErrInvalidCapability)
	_, err = f.service.HouseholdForSession(context.Background(), oldSession)
	assert.ErrorIs(t, err, ErrInvalidCapability)

	newSession, _, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(rotated.AccessURL))
	require.NoError(t, err)
	require.NoError(t, f.service.Revoke(context.Background(), f.eventID, created.Invitation.ID, "cancelled"))
	_, err = f.service.HouseholdForSession(context.Background(), newSession)
	assert.ErrorIs(t, err, ErrInvalidCapability)
}

func TestRecoveryIsStoredDestinationOnlyAndSingleUse(t *testing.T) {
	f := newServiceFixture(t)
	f.create("Recover me", "stored@example.com", 0, "Guest")
	var mu sync.Mutex
	var recipients, links []string
	f.service.SetEmailSender(func(_ context.Context, to, _ string, _ string, plain string) error {
		mu.Lock()
		defer mu.Unlock()
		recipients = append(recipients, to)
		for _, line := range strings.Split(plain, "\n") {
			if strings.Contains(line, "/invitation/recover#") {
				links = append(links, line)
			}
		}
		return nil
	})

	require.NoError(t, f.service.RequestRecovery(context.Background(), f.eventID,
		"STORED@example.com", "192.0.2.10"))
	require.Equal(t, []string{"stored@example.com"}, recipients)
	require.Len(t, links, 1)
	raw := capabilityFromURL(links[0])
	_, household, err := f.service.ExchangeRecovery(context.Background(), raw)
	require.NoError(t, err)
	assert.Equal(t, "Recover me", household.Invitation.Label)
	_, _, err = f.service.ExchangeRecovery(context.Background(), raw)
	assert.ErrorIs(t, err, ErrInvalidCapability)
}

func TestOpenEnrollmentCreatesIsolatedInvitationAndKeepsPrivateContactMatchUntouched(t *testing.T) {
	f := newServiceFixture(t)
	private := f.create("Named household", "same@example.com", 0, "Named Guest")
	config, accessURL, err := f.service.ConfigureOpen(context.Background(), f.eventID, f.userID,
		ConfigureOpenRequest{Enabled: true, MaxPartySize: 2, Capacity: intPtr(2)})
	require.NoError(t, err)
	assert.True(t, config.Enabled)

	session, openHousehold, err := f.service.EnrollOpen(context.Background(), OpenEnrollmentRequest{
		Capability: capabilityFromURL(accessURL), Label: "Open household",
		ContactEmail: ptr("same@example.com"), PreferredDeliveryMethod: "email",
		GuestNames: []string{"Open One", "Open Two"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, session)
	assert.Equal(t, SourceOpen, openHousehold.Invitation.Source)
	assert.NotEqual(t, private.Invitation.ID, openHousehold.Invitation.ID)

	privateAfter, err := f.store.LoadHousehold(context.Background(), private.Invitation.ID)
	require.NoError(t, err)
	assert.Equal(t, "Named Guest", privateAfter.Guests[0].Name)
	assert.Equal(t, AttendancePending, privateAfter.Guests[0].Attendance)

	_, _, err = f.service.EnrollOpen(context.Background(), OpenEnrollmentRequest{
		Capability: capabilityFromURL(accessURL), Label: "Over capacity",
		ContactEmail: ptr("other@example.com"), PreferredDeliveryMethod: "email",
		GuestNames: []string{"Other"},
	})
	assert.ErrorIs(t, err, ErrCapacity)
}

func TestConcurrentResponsesAcceptExactlyOneOptimisticVersion(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Concurrent", "concurrent@example.com", 1, "Assigned")
	session, _, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(created.AccessURL))
	require.NoError(t, err)

	start := make(chan struct{})
	errorsByCall := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(index int) {
			<-start
			_, submitErr := f.service.SubmitForSession(context.Background(), session, SubmitRequest{
				Version:          1,
				AssignedGuests:   []GuestAttendanceInput{{GuestID: created.Guests[0].ID, Attendance: AttendanceAttending}},
				AdditionalGuests: []AdditionalGuestInput{{Name: "Additional " + string(rune('A'+index)), Attendance: AttendanceDeclined}},
			})
			errorsByCall <- submitErr
		}(i)
	}
	close(start)
	var successes, conflicts int
	for i := 0; i < 2; i++ {
		err := <-errorsByCall
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrent response error: %v", err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
	household, err := f.store.LoadHousehold(context.Background(), created.Invitation.ID)
	require.NoError(t, err)
	assert.Len(t, household.Guests, 2)
}

func TestConcurrentOpenCapacityCreatesExactlyOneInvitation(t *testing.T) {
	f := newServiceFixture(t)
	_, accessURL, err := f.service.ConfigureOpen(context.Background(), f.eventID, f.userID,
		ConfigureOpenRequest{Enabled: true, MaxPartySize: 1, Capacity: intPtr(1)})
	require.NoError(t, err)
	capability := capabilityFromURL(accessURL)

	start := make(chan struct{})
	errorsByCall := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(index int) {
			<-start
			_, _, enrollErr := f.service.EnrollOpen(context.Background(), OpenEnrollmentRequest{
				Capability: capability, Label: "Open", ContactEmail: ptr("open@example.com"),
				PreferredDeliveryMethod: "email", GuestNames: []string{"Guest " + string(rune('A'+index))},
			})
			errorsByCall <- enrollErr
		}(i)
	}
	close(start)
	var successes, capacityFailures int
	for i := 0; i < 2; i++ {
		err := <-errorsByCall
		if err == nil {
			successes++
		} else if errors.Is(err, ErrCapacity) {
			capacityFailures++
		} else {
			t.Fatalf("unexpected open enrollment error: %v", err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, capacityFailures)
}

func intPtr(value int) *int { return &value }

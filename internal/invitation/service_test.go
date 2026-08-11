package invitation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/errcode"
	"github.com/xxi0xx/owl-invites/internal/testutil"
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
	testutil.SeedUser(t, db, userID, "owner@example.com", "Owner")
	var err error
	_, err = db.ExecContext(context.Background(), `INSERT INTO events (
		id, title, description, event_date, location, timezone,
		retention_days, status, created_at, updated_at
	) VALUES (?, 'Invitation Test', '', ?, '', 'UTC', 30, 'published', ?, ?)`,
		eventID, time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339),
		now, now)
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

func TestSMSPreferredInvitationCannotSilentlySendEmail(t *testing.T) {
	f := newServiceFixture(t)
	deliveryAttempts := 0
	f.service.SetEmailSender(func(_ context.Context, _, _, _, _, _, _ string) error {
		deliveryAttempts++
		return nil
	})

	_, err := f.service.CreatePrivate(context.Background(), f.eventID, f.userID, CreateRequest{
		Label: "Unsupported immediate SMS", ContactEmail: ptr("metadata@example.com"),
		ContactPhone: ptr("+15551234567"), PreferredDeliveryMethod: "sms",
		AssignedGuestNames: []string{"Guest"}, Send: true,
	})
	require.Error(t, err)
	assert.True(t, errcode.IsValidation(err))
	items, listErr := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, listErr)
	assert.Empty(t, items, "unsupported send requests must fail before resource creation")

	created, err := f.service.CreatePrivate(context.Background(), f.eventID, f.userID, CreateRequest{
		Label: "Manual SMS metadata", ContactEmail: ptr("metadata@example.com"),
		ContactPhone: ptr("+15551234567"), PreferredDeliveryMethod: "sms",
		AssignedGuestNames: []string{"Guest"}, Send: false,
	})
	require.NoError(t, err)
	assert.Equal(t, DeliveryNotRequested, created.Delivery.Status)
	err = f.service.Deliver(context.Background(), created.Invitation.ID)
	require.Error(t, err)
	assert.True(t, errcode.IsValidation(err))
	assert.Equal(t, 0, deliveryAttempts)

	_, err = f.service.Broadcast(context.Background(), f.eventID, &f.userID, MessageRequest{
		RecipientGroup: "all", Subject: "Update", Body: "Household update",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, deliveryAttempts)
	require.NoError(t, f.service.RequestRecovery(context.Background(), f.eventID,
		"metadata@example.com", "192.0.2.30"))
	assert.Equal(t, 0, deliveryAttempts)
}

func TestManualDeliveryRemainsAvailableForEmailInvitation(t *testing.T) {
	f := newServiceFixture(t)
	deliveryAttempts := 0
	f.service.SetEmailSender(func(_ context.Context, _, _, _, _, _, _ string) error {
		deliveryAttempts++
		return nil
	})
	created := f.create("Manual resend", "resend@example.com", 0, "Guest")
	assert.Equal(t, DeliveryNotRequested, created.Delivery.Status)

	require.NoError(t, f.service.Deliver(context.Background(), created.Invitation.ID))
	assert.Equal(t, 1, deliveryAttempts)
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

func TestNewAdditionalGuestAnswersRequiredQuestionInAtomicSubmission(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Household", "household@example.com", 1, "Assigned Guest")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := f.store.db.ExecContext(context.Background(), `INSERT INTO event_questions (
		id, event_id, label, type, options, required, sort_order, deleted, created_at, updated_at, scope
	) VALUES ('required-new-guest-answer', ?, 'Meal choices', 'checkbox', '["Vegetarian","No nuts"]', 1, 0, 0, ?, ?, 'guest')`,
		f.eventID, now, now)
	require.NoError(t, err)

	session, household, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(created.AccessURL))
	require.NoError(t, err)
	updated, err := f.service.SubmitForSession(context.Background(), session, SubmitRequest{
		Version:        household.Response.Version,
		AssignedGuests: []GuestAttendanceInput{{GuestID: created.Guests[0].ID, Attendance: AttendanceDeclined}},
		AdditionalGuests: []AdditionalGuestInput{{
			ClientKey: "new-browser-guest", Name: "New Plus One", Attendance: AttendanceAttending,
		}},
		GuestAnswers: map[string]map[string]string{
			"new-browser-guest": {"required-new-guest-answer": `["Vegetarian"]`},
		},
	})
	require.NoError(t, err)
	require.Len(t, updated.Guests, 2)
	additional := updated.Guests[1]
	assert.Equal(t, GuestOriginAdditional, additional.Origin)
	assert.Equal(t, AttendanceAttending, additional.Attendance)
	assert.Contains(t, updated.GuestAnswers, GuestAnswer{
		GuestID: additional.ID, QuestionID: "required-new-guest-answer", Answer: `["Vegetarian"]`,
	})
	for _, answer := range updated.GuestAnswers {
		assert.NotEqual(t, "new-browser-guest", answer.GuestID, "the client correlation key must never be persisted")
	}
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

func TestPrivateCapabilityTamperingCannotSelectAnotherInvitation(t *testing.T) {
	f := newServiceFixture(t)
	first := f.create("First", "first@example.com", 0, "First Guest")
	second := f.create("Second", "second@example.com", 0, "Second Guest")

	firstParts := strings.Split(capabilityFromURL(first.AccessURL), ".")
	secondParts := strings.Split(capabilityFromURL(second.AccessURL), ".")
	require.Len(t, firstParts, 4)
	require.Len(t, secondParts, 4)
	firstParts[1] = secondParts[1]

	_, _, err := f.service.ExchangePrivate(context.Background(), strings.Join(firstParts, "."))
	assert.ErrorIs(t, err, ErrInvalidCapability)
}

func TestPrivateCapabilityReplayCreatesDistinctScopedSessionsUntilRotation(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Durable capability", "durable@example.com", 0, "Guest")
	capability := capabilityFromURL(created.AccessURL)

	firstSession, firstHousehold, err := f.service.ExchangePrivate(context.Background(), capability)
	require.NoError(t, err)
	secondSession, secondHousehold, err := f.service.ExchangePrivate(context.Background(), capability)
	require.NoError(t, err)
	assert.NotEqual(t, firstSession, secondSession)
	assert.Equal(t, created.Invitation.ID, firstHousehold.Invitation.ID)
	assert.Equal(t, created.Invitation.ID, secondHousehold.Invitation.ID)

	updated, err := f.service.SubmitForSession(context.Background(), firstSession, SubmitRequest{
		Version: 1,
		AssignedGuests: []GuestAttendanceInput{{
			GuestID: created.Guests[0].ID, Attendance: AttendanceAttending,
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Response.Version)

	visibleToSecond, err := f.service.HouseholdForSession(context.Background(), secondSession)
	require.NoError(t, err)
	assert.Equal(t, AttendanceAttending, visibleToSecond.Guests[0].Attendance)

	_, err = f.service.Rotate(context.Background(), f.eventID, created.Invitation.ID)
	require.NoError(t, err)
	_, err = f.service.HouseholdForSession(context.Background(), firstSession)
	assert.ErrorIs(t, err, ErrInvalidCapability)
	_, err = f.service.HouseholdForSession(context.Background(), secondSession)
	assert.ErrorIs(t, err, ErrInvalidCapability)
}

func TestExpiredInvitationSessionAndRecoveryTokenAreRejected(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Expiry", "expiry@example.com", 0, "Guest")

	expiredSession := strings.Repeat("s", 43)
	require.NoError(t, f.store.CreateSession(context.Background(), created.Invitation.ID,
		hashToken(expiredSession), created.Invitation.TokenVersion, time.Now().UTC().Add(-time.Minute)))
	_, err := f.service.HouseholdForSession(context.Background(), expiredSession)
	assert.ErrorIs(t, err, ErrInvalidCapability)

	expiredRecovery := strings.Repeat("r", 43)
	require.NoError(t, f.store.CreateRecoveryToken(context.Background(), created.Invitation.ID,
		hashToken(expiredRecovery), "email", time.Now().UTC().Add(-time.Minute)))
	_, _, err = f.service.ExchangeRecovery(context.Background(), expiredRecovery)
	assert.ErrorIs(t, err, ErrInvalidCapability)
}

func TestRecoveryIsStoredDestinationOnlyAndSingleUse(t *testing.T) {
	f := newServiceFixture(t)
	f.create("Recover me", "stored@example.com", 0, "Guest")
	var mu sync.Mutex
	var recipients, links []string
	f.service.SetEmailSender(func(_ context.Context, _, _, to, _ string, _ string, plain string) error {
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
	var managementRecipient, managementLink string
	f.service.SetEmailSender(func(_ context.Context, _, _, to, _ string, _ string, plain string) error {
		managementRecipient = to
		for _, line := range strings.Split(plain, "\n") {
			if strings.Contains(line, "/invitation/accept#") {
				managementLink = line
			}
		}
		return nil
	})
	config, accessURL, err := f.service.ConfigureOpen(context.Background(), f.eventID, f.userID,
		ConfigureOpenRequest{Enabled: true, MaxPartySize: 2, Capacity: intPtr(2)})
	require.NoError(t, err)
	assert.True(t, config.Enabled)

	session, openHousehold, delivery, err := f.service.EnrollOpen(context.Background(), OpenEnrollmentRequest{
		Capability: capabilityFromURL(accessURL), Label: "Open household",
		ContactEmail: ptr("same@example.com"), PreferredDeliveryMethod: "email",
		GuestNames: []string{"Open One", "Open Two"},
	})
	require.NoError(t, err)
	assert.Equal(t, DeliverySent, delivery.Status)
	assert.NotEmpty(t, session)
	assert.Equal(t, SourceOpen, openHousehold.Invitation.Source)
	assert.NotEqual(t, private.Invitation.ID, openHousehold.Invitation.ID)
	assert.Equal(t, "same@example.com", managementRecipient)
	assert.NotEmpty(t, managementLink)

	// The open capability stays in its enrollment-only HMAC domain. The
	// separately emailed household capability selects only the invitation that
	// this enrollment created, even though a private invitation shares its email.
	_, _, err = f.service.ExchangePrivate(context.Background(), capabilityFromURL(accessURL))
	assert.ErrorIs(t, err, ErrInvalidCapability)
	managementSession, managedOpen, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(managementLink))
	require.NoError(t, err)
	assert.Equal(t, openHousehold.Invitation.ID, managedOpen.Invitation.ID)
	_, err = f.service.SubmitForSession(context.Background(), managementSession, SubmitRequest{
		Version: 1,
		AssignedGuests: []GuestAttendanceInput{{
			GuestID: private.Guests[0].ID, Attendance: AttendanceAttending,
		}},
	})
	assert.True(t, errcode.IsValidation(err), err)
	managedOpen, err = f.service.SubmitForSession(context.Background(), managementSession, SubmitRequest{
		Version: 1,
		AssignedGuests: []GuestAttendanceInput{{
			GuestID: openHousehold.Guests[0].ID, Attendance: AttendanceAttending,
		}, {
			GuestID: openHousehold.Guests[1].ID, Attendance: AttendanceDeclined,
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, AttendanceAttending, managedOpen.Guests[0].Attendance)

	privateAfter, err := f.store.LoadHousehold(context.Background(), private.Invitation.ID)
	require.NoError(t, err)
	assert.Equal(t, "Named Guest", privateAfter.Guests[0].Name)
	assert.Equal(t, AttendancePending, privateAfter.Guests[0].Attendance)

	_, _, _, err = f.service.EnrollOpen(context.Background(), OpenEnrollmentRequest{
		Capability: capabilityFromURL(accessURL), Label: "Over capacity",
		ContactEmail: ptr("other@example.com"), PreferredDeliveryMethod: "email",
		GuestNames: []string{"Other"},
	})
	assert.ErrorIs(t, err, ErrCapacity)
}

func TestOpenOriginInvitationCanRecoverToStoredDestination(t *testing.T) {
	f := newServiceFixture(t)
	_, accessURL, err := f.service.ConfigureOpen(context.Background(), f.eventID, f.userID,
		ConfigureOpenRequest{Enabled: true, MaxPartySize: 1})
	require.NoError(t, err)
	f.service.SetEmailSender(func(_ context.Context, _, _, _, _, _, _ string) error { return nil })
	_, enrolled, delivery, err := f.service.EnrollOpen(context.Background(), OpenEnrollmentRequest{
		Capability: capabilityFromURL(accessURL), Label: "Recoverable open household",
		ContactEmail: ptr("open-recovery@example.com"), PreferredDeliveryMethod: "email",
		GuestNames: []string{"Open Guest"},
	})
	require.NoError(t, err)
	assert.Equal(t, DeliverySent, delivery.Status)

	var recipient, recoveryLink string
	f.service.SetEmailSender(func(_ context.Context, _, _, to, _ string, _ string, plain string) error {
		recipient = to
		for _, line := range strings.Split(plain, "\n") {
			if strings.Contains(line, "/invitation/recover#") {
				recoveryLink = line
			}
		}
		return nil
	})
	require.NoError(t, f.service.RequestRecovery(context.Background(), f.eventID,
		"OPEN-RECOVERY@example.com", "192.0.2.20"))
	assert.Equal(t, "open-recovery@example.com", recipient)
	require.NotEmpty(t, recoveryLink)
	_, recovered, err := f.service.ExchangeRecovery(context.Background(), capabilityFromURL(recoveryLink))
	require.NoError(t, err)
	assert.Equal(t, enrolled.Invitation.ID, recovered.Invitation.ID)
	assert.Equal(t, SourceOpen, recovered.Invitation.Source)
}

func TestOpenEnrollmentWindowsAndRotationAreEnforced(t *testing.T) {
	f := newServiceFixture(t)
	now := time.Now().UTC()
	futureOpen := now.Add(time.Hour).Format(time.RFC3339)
	futureClose := now.Add(2 * time.Hour).Format(time.RFC3339)
	_, futureURL, err := f.service.ConfigureOpen(context.Background(), f.eventID, f.userID,
		ConfigureOpenRequest{Enabled: true, OpensAt: &futureOpen, ClosesAt: &futureClose, MaxPartySize: 2})
	require.NoError(t, err)
	_, _, err = f.service.InspectOpen(context.Background(), capabilityFromURL(futureURL))
	assert.ErrorIs(t, err, ErrInvalidCapability)

	pastOpen := now.Add(-2 * time.Hour).Format(time.RFC3339)
	pastClose := now.Add(-time.Hour).Format(time.RFC3339)
	_, closedURL, err := f.service.ConfigureOpen(context.Background(), f.eventID, f.userID,
		ConfigureOpenRequest{Enabled: true, OpensAt: &pastOpen, ClosesAt: &pastClose, MaxPartySize: 2})
	require.NoError(t, err)
	_, _, err = f.service.InspectOpen(context.Background(), capabilityFromURL(closedURL))
	assert.ErrorIs(t, err, ErrInvalidCapability)

	activeClose := now.Add(time.Hour).Format(time.RFC3339)
	_, activeURL, err := f.service.ConfigureOpen(context.Background(), f.eventID, f.userID,
		ConfigureOpenRequest{Enabled: true, OpensAt: &pastOpen, ClosesAt: &activeClose, MaxPartySize: 2})
	require.NoError(t, err)
	_, _, err = f.service.InspectOpen(context.Background(), capabilityFromURL(activeURL))
	require.NoError(t, err)

	_, rotatedURL, err := f.service.RotateOpen(context.Background(), f.eventID)
	require.NoError(t, err)
	_, _, err = f.service.InspectOpen(context.Background(), capabilityFromURL(activeURL))
	assert.ErrorIs(t, err, ErrInvalidCapability)
	_, _, err = f.service.InspectOpen(context.Background(), capabilityFromURL(rotatedURL))
	require.NoError(t, err)
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
			_, _, _, enrollErr := f.service.EnrollOpen(context.Background(), OpenEnrollmentRequest{
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

func TestRecoveryBudgetsAreAtomicUnderConcurrency(t *testing.T) {
	tests := []struct {
		name            string
		attempts        int
		expectedAllowed int
		sourceFor       func(int) string
		destinationFor  func(int) string
	}{
		{name: "source", attempts: 12, expectedAllowed: 5,
			sourceFor:      func(int) string { return "same-source" },
			destinationFor: func(i int) string { return fmt.Sprintf("destination-%d", i) }},
		{name: "destination", attempts: 12, expectedAllowed: 3,
			sourceFor:      func(i int) string { return fmt.Sprintf("source-%d", i) },
			destinationFor: func(int) string { return "same-destination" }},
		{name: "event", attempts: 40, expectedAllowed: 30,
			sourceFor:      func(i int) string { return fmt.Sprintf("source-%d", i) },
			destinationFor: func(i int) string { return fmt.Sprintf("destination-%d", i) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newServiceFixture(t)
			start := make(chan struct{})
			results := make(chan bool, test.attempts)
			errorsByCall := make(chan error, test.attempts)
			for i := 0; i < test.attempts; i++ {
				go func(index int) {
					<-start
					// Separate Store values ensure the database, rather than a
					// process-local mutex, serializes the budget decision.
					allowed, allowErr := NewStore(f.store.db).AllowRecovery(context.Background(),
						f.eventID, test.sourceFor(index), test.destinationFor(index))
					results <- allowed
					errorsByCall <- allowErr
				}(i)
			}
			close(start)
			allowedCount := 0
			for i := 0; i < test.attempts; i++ {
				require.NoError(t, <-errorsByCall)
				if <-results {
					allowedCount++
				}
			}
			assert.Equal(t, test.expectedAllowed, allowedCount)
		})
	}
}

func intPtr(value int) *int { return &value }

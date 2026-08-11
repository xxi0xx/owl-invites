package invitation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizerUpdateSoftRemovesAssignedGuestsAndPreservesCapabilityAndAdditionalGuests(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Original household", "original@example.com", 1, "Alex", "Bailey")
	capability := capabilityFromURL(created.AccessURL)
	session, household, err := f.service.ExchangePrivate(context.Background(), capability)
	require.NoError(t, err)
	updatedByGuest, err := f.service.SubmitForSession(context.Background(), session, SubmitRequest{
		Version: household.Response.Version,
		AssignedGuests: []GuestAttendanceInput{
			{GuestID: created.Guests[0].ID, Attendance: AttendanceAttending},
			{GuestID: created.Guests[1].ID, Attendance: AttendanceDeclined},
		},
		AdditionalGuests:  []AdditionalGuestInput{{Name: "Household-managed Plus One", Attendance: AttendanceMaybe}},
		InvitationAnswers: map[string]string{}, GuestAnswers: map[string]map[string]string{},
	})
	require.NoError(t, err)
	additionalID := updatedByGuest.Guests[2].ID

	result, err := f.service.UpdateInvitation(context.Background(), f.eventID, created.Invitation.ID,
		UpdateInvitationRequest{
			Label: "Renamed household", ContactEmail: ptr("new@example.com"),
			PreferredDeliveryMethod: "email", AdditionalGuestAllowance: 2,
			AssignedGuests: []AssignedGuestEdit{
				{ID: created.Guests[0].ID, Name: "Alex Renamed"},
				{Name: "New Assigned Guest"},
			},
		})
	require.NoError(t, err)
	assert.Equal(t, "Renamed household", result.Invitation.Label)
	assert.Equal(t, 2, result.Invitation.AdditionalGuestAllowance)
	require.Len(t, result.Guests, 3)
	assert.Equal(t, "Alex Renamed", result.Guests[0].Name)
	assert.Equal(t, AttendanceAttending, result.Guests[0].Attendance,
		"renaming an assigned guest must retain response history")
	assert.Equal(t, "New Assigned Guest", result.Guests[1].Name)
	assert.Equal(t, AttendancePending, result.Guests[1].Attendance)
	assert.Equal(t, additionalID, result.Guests[2].ID)
	assert.Equal(t, GuestOriginAdditional, result.Guests[2].Origin)
	assert.Equal(t, "Household-managed Plus One", result.Guests[2].Name,
		"organizer edits must not convert or replace holder-managed additional guests")

	var removedAt *string
	err = f.store.db.QueryRowContext(context.Background(), `SELECT removed_at FROM guests WHERE id = ?`, created.Guests[1].ID).Scan(&removedAt)
	require.NoError(t, err)
	require.NotNil(t, removedAt, "removed assigned guests must be soft-removed")
	var responseRows int
	err = f.store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM guest_responses WHERE guest_id = ?`, created.Guests[1].ID).Scan(&responseRows)
	require.NoError(t, err)
	assert.Equal(t, 1, responseRows, "soft removal must retain prior response state")

	_, reopened, err := f.service.ExchangePrivate(context.Background(), capability)
	require.NoError(t, err, "safe metadata editing must not rotate the existing household capability")
	assert.Equal(t, "Renamed household", reopened.Invitation.Label)
	assert.Equal(t, 1, reopened.Invitation.TokenVersion)
}

func TestOrganizerSearchAndResponseAttendanceFiltersAreInvitationScoped(t *testing.T) {
	f := newServiceFixture(t)
	first := f.create("Smith Family", "shared@example.com", 0, "Alex Smith")
	second := f.create("Garcia Family", "shared@example.com", 0, "María García")
	session, household, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(first.AccessURL))
	require.NoError(t, err)
	_, err = f.service.SubmitForSession(context.Background(), session, SubmitRequest{
		Version:          household.Response.Version,
		AssignedGuests:   []GuestAttendanceInput{{GuestID: first.Guests[0].ID, Attendance: AttendanceAttending}},
		AdditionalGuests: []AdditionalGuestInput{}, InvitationAnswers: map[string]string{}, GuestAnswers: map[string]map[string]string{},
	})
	require.NoError(t, err)

	items, err := f.service.ListOrganizerHouseholds(context.Background(), f.eventID, InvitationListFilter{Search: "maría"})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, second.Invitation.ID, items[0].Invitation.ID)
	items, err = f.service.ListOrganizerHouseholds(context.Background(), f.eventID, InvitationListFilter{Search: "shared@example.com"})
	require.NoError(t, err)
	assert.Len(t, items, 2, "contact equality is searchable but never treated as identity")
	items, err = f.service.ListOrganizerHouseholds(context.Background(), f.eventID, InvitationListFilter{Response: "submitted", Attendance: AttendanceAttending})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, first.Invitation.ID, items[0].Invitation.ID)
	items, err = f.service.ListOrganizerHouseholds(context.Background(), f.eventID, InvitationListFilter{Response: "not_submitted", Attendance: AttendancePending})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, second.Invitation.ID, items[0].Invitation.ID)
}

func TestOrganizerDeliverySummaryUsesActualNotificationState(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Delivery household", "delivery@example.com", 0, "Guest")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := f.store.db.ExecContext(context.Background(), `INSERT INTO notification_log (
		id, event_id, invitation_id, channel, provider, status, error, recipient,
		subject, delivery_status, sent_at, created_at
	) VALUES (?, ?, ?, 'email', 'smtp', 'sent', NULL, ?, 'Invitation', 'unknown', ?, ?)`,
		"delivery-log", f.eventID, created.Invitation.ID, "delivery@example.com", now, now)
	require.NoError(t, err)
	household, err := f.store.LoadOrganizerHousehold(context.Background(), created.Invitation.ID)
	require.NoError(t, err)
	require.NotNil(t, household.LatestDelivery)
	assert.Equal(t, "sent", household.LatestDelivery.Status)
	assert.Equal(t, "unknown", household.LatestDelivery.DeliveryStatus,
		"SMTP acceptance must not be fabricated as end-recipient delivery")
	assert.Equal(t, "smtp", household.LatestDelivery.Provider)

	guestView, err := f.store.LoadHousehold(context.Background(), created.Invitation.ID)
	require.NoError(t, err)
	assert.Nil(t, guestView.LatestDelivery, "delivery diagnostics are organizer-only")
}

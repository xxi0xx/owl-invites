package invitation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBroadcastPreviewAndDetailedResultDoNotExposeDestinationsOrStopOnFailure(t *testing.T) {
	f := newServiceFixture(t)
	f.create("First", "first@example.com", 0, "First Guest")
	f.create("Second", "second@example.com", 0, "Second Guest")
	f.create("Third", "third@example.com", 0, "Third Guest")
	_, err := f.service.CreatePrivate(context.Background(), f.eventID, f.userID, CreateRequest{
		Label: "SMS metadata", ContactEmail: ptr("must-not-email@example.com"),
		ContactPhone: ptr("+15551234567"), PreferredDeliveryMethod: "sms",
		AssignedGuestNames: []string{"SMS Guest"},
	})
	require.NoError(t, err)

	preview, err := f.service.PreviewBroadcast(context.Background(), f.eventID,
		MessagePreviewRequest{RecipientGroup: "all"})
	require.NoError(t, err)
	assert.Equal(t, 3, preview.RecipientHouseholds,
		"email metadata on an SMS-preferred household must not make it deliverable")

	attempted := make([]string, 0)
	f.service.SetEmailSender(func(_ context.Context, _, _, to, _, _, _ string) error {
		attempted = append(attempted, to)
		if to == "second@example.com" {
			return errors.New("deliberate provider failure")
		}
		if to == "third@example.com" {
			return ErrDeliverySuppressed
		}
		return nil
	})
	result, err := f.service.BroadcastDetailed(context.Background(), f.eventID, &f.userID, MessageRequest{
		RecipientGroup: "all", Subject: "Event update", Body: "One-way update",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, result.Attempted)
	assert.Equal(t, 1, result.Accepted)
	assert.Equal(t, 1, result.Failed)
	assert.Equal(t, 1, result.Skipped)
	assert.Len(t, attempted, 3, "one failure must not prevent later household attempts")
	assert.NotContains(t, attempted, "must-not-email@example.com")

	var messages int
	err = f.store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM invitation_messages WHERE event_id = ?`, f.eventID).Scan(&messages)
	require.NoError(t, err)
	assert.Equal(t, 1, messages, "message intent is persisted separately from delivery outcomes")
}

func TestBroadcastAttendanceTargetCountsEachHouseholdOnce(t *testing.T) {
	f := newServiceFixture(t)
	first := f.create("Two attending guests", "first@example.com", 0, "One", "Two")
	f.create("Pending household", "pending@example.com", 0, "Pending")
	session, household, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(first.AccessURL))
	require.NoError(t, err)
	_, err = f.service.SubmitForSession(context.Background(), session, SubmitRequest{
		Version: household.Response.Version,
		AssignedGuests: []GuestAttendanceInput{
			{GuestID: first.Guests[0].ID, Attendance: AttendanceAttending},
			{GuestID: first.Guests[1].ID, Attendance: AttendanceAttending},
		},
		AdditionalGuests: []AdditionalGuestInput{}, InvitationAnswers: map[string]string{}, GuestAnswers: map[string]map[string]string{},
	})
	require.NoError(t, err)
	preview, err := f.service.PreviewBroadcast(context.Background(), f.eventID,
		MessagePreviewRequest{RecipientGroup: AttendanceAttending})
	require.NoError(t, err)
	assert.Equal(t, 1, preview.RecipientHouseholds,
		"attendance targeting is household delivery, not one message per matching guest")
}

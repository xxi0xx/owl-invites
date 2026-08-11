package invitation

import (
	"bytes"
	"context"
	"encoding/csv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventExportFlattensCurrentDomainAndDefangsSpreadsheetFormulas(t *testing.T) {
	f := newServiceFixture(t)
	first := f.create("Family, One", "shared@example.com", 1, "=HYPERLINK(\"https://bad.example\")")
	second := f.create("第二家庭", "shared@example.com", 0, "Zoë 王")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, question := range []struct {
		id, label, kind, options, scope string
		order                           int
	}{
		{"question-invitation-a", "Diet, notes", "text", "[]", QuestionScopeInvitation, 0},
		{"question-invitation-b", "Diet, notes", "text", "[]", QuestionScopeInvitation, 1},
		{"question-guest", "Selections", "checkbox", `["Vegan","No nuts"]`, QuestionScopeGuest, 2},
	} {
		_, err := f.store.db.ExecContext(context.Background(), `INSERT INTO event_questions (
			id, event_id, label, type, options, required, scope, sort_order, deleted, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 0, ?, ?, 0, ?, ?)`, question.id, f.eventID, question.label,
			question.kind, question.options, question.scope, question.order, now, now)
		require.NoError(t, err)
	}

	session, household, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(first.AccessURL))
	require.NoError(t, err)
	_, err = f.service.SubmitForSession(context.Background(), session, SubmitRequest{
		Version: household.Response.Version,
		AssignedGuests: []GuestAttendanceInput{{
			GuestID: first.Guests[0].ID, Attendance: AttendanceAttending,
		}},
		AdditionalGuests: []AdditionalGuestInput{{Name: "Guest, Plus", Attendance: AttendanceMaybe}},
		InvitationAnswers: map[string]string{
			"question-invitation-a": "=2+2",
			"question-invitation-b": "+SUM(1,1)",
		},
		GuestAnswers: map[string]map[string]string{
			first.Guests[0].ID: {"question-guest": `["Vegan","No nuts"]`},
		},
	})
	require.NoError(t, err)

	data, err := f.service.ExportEventCSV(context.Background(), f.eventID)
	require.NoError(t, err)
	reader := csv.NewReader(bytes.NewReader(data))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 4, "header plus assigned, additional, and second-household guests")
	header := records[0]
	require.Len(t, header, len(exportBaseColumns)+3)
	assert.NotEqual(t, header[len(exportBaseColumns)], header[len(exportBaseColumns)+1],
		"duplicate question labels remain unambiguous through stable question IDs")
	assert.Contains(t, header[len(exportBaseColumns)], "invitation_answer:")
	assert.Contains(t, header[len(exportBaseColumns)+2], "guest_answer:")

	firstRow := records[1]
	assert.Equal(t, first.Invitation.ID, firstRow[0])
	assert.Equal(t, "Family, One", firstRow[1], "quoted commas must round-trip")
	assert.Equal(t, "'=HYPERLINK(\"https://bad.example\")", firstRow[8])
	assert.Equal(t, "submitted", firstRow[11])
	assert.NotEmpty(t, firstRow[12])
	assert.NotEmpty(t, firstRow[13])
	assert.Equal(t, "'=2+2", firstRow[len(exportBaseColumns)])
	assert.Equal(t, "'+SUM(1,1)", firstRow[len(exportBaseColumns)+1])
	assert.Equal(t, "Vegan | No nuts", firstRow[len(exportBaseColumns)+2])

	secondHouseholdRow := records[3]
	assert.Equal(t, second.Invitation.ID, secondHouseholdRow[0])
	assert.NotEqual(t, firstRow[0], secondHouseholdRow[0],
		"equal email destinations remain distinct export households")
	assert.Equal(t, "shared@example.com", secondHouseholdRow[2])
	assert.Equal(t, "Zoë 王", secondHouseholdRow[8])
	assert.Equal(t, "no_submitted_response", secondHouseholdRow[11])
}

func TestDefangCSVCellCoversWhitespaceAndFormulaPrefixes(t *testing.T) {
	for _, value := range []string{"=cmd", "+cmd", "-cmd", "@cmd", "  =cmd", "\t=cmd", "\r=cmd"} {
		assert.Equal(t, "'"+value, defangCSVCell(value))
	}
	for _, value := range []string{"", "ordinary", "123", "hello@example.com"} {
		assert.Equal(t, value, defangCSVCell(value))
	}
}

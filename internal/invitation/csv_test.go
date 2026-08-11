package invitation

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const importHeader = "household_key,household_label,contact_email,contact_phone,preferred_delivery,additional_guest_allowance,guest_name\n"

func TestPreviewImportCSVNormalizesExplicitHouseholdsWithoutPersistence(t *testing.T) {
	f := newServiceFixture(t)
	csvData := "\ufeffhousehold_key,household_label,contact_email,contact_phone,preferred_delivery,additional_guest_allowance,guest_name\r\n" +
		"smith,\"Smith, Family\",SHARED@example.com,,email,1,Jane Smith\r\n" +
		"smith,\"Smith, Family\",SHARED@example.com,,email,1,Jane Smith\r\n" +
		"garcia,Garcia Family,shared@example.com,+1 (555) 123-4567,email,0,María García\r\n"

	preview, err := PreviewImportCSV(strings.NewReader(csvData))
	require.NoError(t, err)
	require.Empty(t, preview.Errors)
	assert.Equal(t, 2, preview.HouseholdCount)
	assert.Equal(t, 3, preview.AssignedGuestCount)
	require.Len(t, preview.Households, 2)
	assert.Equal(t, "Smith, Family", preview.Households[0].HouseholdLabel)
	assert.Equal(t, []string{"Jane Smith", "Jane Smith"}, preview.Households[0].AssignedGuestNames,
		"duplicate names are valid because names are not identity")
	assert.Equal(t, "shared@example.com", *preview.Households[0].ContactEmail)
	assert.Equal(t, "+15551234567", *preview.Households[1].ContactPhone)
	require.Len(t, preview.Warnings, 1)
	assert.Contains(t, preview.Warnings[0].Message, "remain separate invitations")

	items, err := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, err)
	assert.Empty(t, items, "preview must not persist invitations or guests")
}

func TestPreviewImportCSVValidation(t *testing.T) {
	tests := []struct {
		name     string
		csv      string
		contains string
	}{
		{"blank grouping key", importHeader + ",Family,a@example.com,,email,0,Guest\n", "household key is required"},
		{"invalid email", importHeader + "a,Family,not-an-email,,email,0,Guest\n", "invalid contact email"},
		{"invalid phone", importHeader + "a,Family,,12,sms,0,Guest\n", "invalid contact phone"},
		{"invalid allowance", importHeader + "a,Family,a@example.com,,email,51,Guest\n", "integer between 0 and 50"},
		{"conflicting household values", importHeader + "a,Family,a@example.com,,email,0,One\na,Other,a@example.com,,email,0,Two\n", "conflicts with household"},
		{"duplicate header", "household_key,household_key,household_label,preferred_delivery,additional_guest_allowance,guest_name\na,a,Family,email,0,Guest\n", "duplicate CSV header"},
		{"unsupported header", importHeader[:len(importHeader)-1] + ",nickname\na,Family,a@example.com,,email,0,Guest,G\n", "unsupported CSV column"},
		{"malformed csv", importHeader + "a,\"unterminated\n", "malformed CSV"},
		{"invalid utf8", importHeader + "a,Family,a@example.com,,email,0,\xff\n", "valid UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preview, err := PreviewImportCSV(strings.NewReader(tt.csv))
			require.NoError(t, err)
			require.NotEmpty(t, preview.Errors)
			messages := make([]string, len(preview.Errors))
			for i, issue := range preview.Errors {
				messages[i] = issue.Message
			}
			assert.Contains(t, strings.Join(messages, " | "), tt.contains)
		})
	}
}

func TestPreviewImportCSVRejectsOversizedInputAndExcessiveRows(t *testing.T) {
	oversized := strings.Repeat("x", MaxImportBytes+1)
	preview, err := PreviewImportCSV(strings.NewReader(oversized))
	require.NoError(t, err)
	require.NotEmpty(t, preview.Errors)
	assert.Contains(t, preview.Errors[0].Message, "byte limit")

	var csvData strings.Builder
	csvData.WriteString(importHeader)
	for i := 0; i < MaxImportRows+1; i++ {
		_, _ = fmt.Fprintf(&csvData, "h%d,Household %d,h%d@example.com,,email,0,Guest %d\n", i, i, i, i)
	}
	preview, err = PreviewImportCSV(strings.NewReader(csvData.String()))
	require.NoError(t, err)
	require.NotEmpty(t, preview.Errors)
	assert.Contains(t, preview.Errors[len(preview.Errors)-1].Message, "row limit")
}

func TestCommitImportRealisticScaleKeepsEqualContactsIsolated(t *testing.T) {
	f := newServiceFixture(t)
	var csvData strings.Builder
	csvData.WriteString(importHeader)
	expectedGuests := 0
	for household := 0; household < 40; household++ {
		guestCount := 1 + household%3
		expectedGuests += guestCount
		email := fmt.Sprintf("destination-%d@example.com", household%8)
		for guest := 0; guest < guestCount; guest++ {
			name := fmt.Sprintf("Guest %d-%d", household, guest)
			if household == 7 && guest == 0 {
				name = "Zoë Łukasz 王"
			}
			_, _ = fmt.Fprintf(&csvData, "house-%d,Household %d,%s,,email,%d,%s\n",
				household, household, email, household%3, name)
		}
	}
	preview, err := PreviewImportCSV(strings.NewReader(csvData.String()))
	require.NoError(t, err)
	require.Empty(t, preview.Errors)
	assert.Equal(t, 40, preview.HouseholdCount)
	assert.Equal(t, expectedGuests, preview.AssignedGuestCount)

	result, err := f.service.CommitImport(context.Background(), f.eventID, f.userID,
		ImportCommitRequest{Households: preview.Households})
	require.NoError(t, err)
	assert.Equal(t, 40, result.HouseholdCount)
	assert.Equal(t, expectedGuests, result.AssignedGuestCount)
	require.Len(t, result.InvitationIDs, 40)

	items, err := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, err)
	require.Len(t, items, 40, "equal contacts must never merge household invitations")
	seenIDs := make(map[string]bool)
	totalGuests := 0
	for _, household := range items {
		assert.False(t, seenIDs[household.Invitation.ID])
		seenIDs[household.Invitation.ID] = true
		totalGuests += len(household.Guests)
		assert.Equal(t, SourcePrivate, household.Invitation.Source)
		assert.Equal(t, AttendancePending, household.Guests[0].Attendance)
	}
	assert.Equal(t, expectedGuests, totalGuests)
}

func TestCommitImportRevalidatesAndDoesNotSend(t *testing.T) {
	f := newServiceFixture(t)
	deliveries := 0
	f.service.SetEmailSender(func(context.Context, string, string, string, string, string, string) error {
		deliveries++
		return nil
	})
	request := ImportCommitRequest{Households: []ImportHousehold{{
		HouseholdKey: "one", HouseholdLabel: "One", ContactEmail: ptr("one@example.com"),
		PreferredDelivery: "email", AssignedGuestNames: []string{"Guest"},
	}}}
	result, err := f.service.CommitImport(context.Background(), f.eventID, f.userID, request)
	require.NoError(t, err)
	assert.Equal(t, 1, result.HouseholdCount)
	assert.Zero(t, deliveries, "import must never send invitations")

	request.Households[0].ContactEmail = ptr("invalid")
	_, err = f.service.CommitImport(context.Background(), f.eventID, f.userID, request)
	require.Error(t, err, "commit must not trust previewed/client-normalized data")
	items, listErr := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, listErr)
	assert.Len(t, items, 1)
}

func TestStoreImportRollsBackEveryHouseholdOnLateDatabaseFailure(t *testing.T) {
	f := newServiceFixture(t)
	creator := f.userID
	first := &Invitation{ID: "duplicate-import-id", EventID: f.eventID, Label: "First",
		PreferredDeliveryMethod: "none", Source: SourcePrivate, AccessID: "access-one",
		TokenVersion: 1, CreatedByUserID: &creator}
	second := &Invitation{ID: "duplicate-import-id", EventID: f.eventID, Label: "Second",
		PreferredDeliveryMethod: "none", Source: SourcePrivate, AccessID: "access-two",
		TokenVersion: 1, CreatedByUserID: &creator}
	err := f.store.Import(context.Background(), []ImportRecord{
		{Invitation: first, Guests: []*Guest{{ID: "guest-one", Name: "One", Origin: GuestOriginAssigned}}},
		{Invitation: second, Guests: []*Guest{{ID: "guest-two", Name: "Two", Origin: GuestOriginAssigned}}},
	})
	require.Error(t, err)
	items, listErr := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, listErr)
	assert.Empty(t, items, "a late import failure must roll back earlier households")
}

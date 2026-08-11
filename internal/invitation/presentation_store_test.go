package invitation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHouseholdPresentationExposesOnlySafeInviteCardFields(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Presentation household", "card@example.com", 0, "Guest")
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := f.store.db.ExecContext(context.Background(), `INSERT INTO invite_cards (
		id, event_id, template_id, heading, body, footer, primary_color,
		secondary_color, font, custom_data, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "card-id", f.eventID,
		"garden-picnic", "Welcome to the garden", "Bring a picnic blanket", "See you there",
		"#22c55e", "#a3e635", "Inter", `{"backgroundImage":"/api/v1/uploads/card.webp","internalPath":"C:/private/uploads/card.webp","organizerNotes":"hidden"}`,
		now, now)
	require.NoError(t, err)

	_, household, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(created.AccessURL))
	require.NoError(t, err)
	require.NotNil(t, household.Presentation)
	assert.Equal(t, "garden-picnic", household.Presentation.TemplateID)
	assert.Equal(t, "Welcome to the garden", household.Presentation.Heading)
	assert.Equal(t, "/api/v1/uploads/card.webp", household.Presentation.BackgroundImage)

	encoded, err := json.Marshal(household)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "card-id")
	assert.NotContains(t, string(encoded), "internalPath")
	assert.NotContains(t, string(encoded), "C:/private")
	assert.NotContains(t, string(encoded), "organizerNotes")
	assert.NotContains(t, string(encoded), "accessId")
}

func TestGuestPresentationDefaultsAndRevalidatesStoredCSSValues(t *testing.T) {
	f := newServiceFixture(t)
	created := f.create("Default presentation", "default@example.com", 0, "Guest")
	household, err := f.store.LoadHousehold(context.Background(), created.Invitation.ID)
	require.NoError(t, err)
	assert.Equal(t, "clean-minimal", household.Presentation.TemplateID)
	assert.Equal(t, "#E54666", household.Presentation.PrimaryColor)

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = f.store.db.ExecContext(context.Background(), `INSERT INTO invite_cards (
		id, event_id, template_id, heading, body, footer, primary_color,
		secondary_color, font, custom_data, created_at, updated_at
	) VALUES (?, ?, ?, '', '', '', ?, ?, ?, ?, ?, ?)`, "unsafe-card", f.eventID,
		"unknown-template", "red; background:url(evil)", "#not-a-color", "Inter; color:red",
		`{"backgroundImage":"/private/files/secret.png"}`, now, now)
	require.NoError(t, err)
	household, err = f.store.LoadHousehold(context.Background(), created.Invitation.ID)
	require.NoError(t, err)
	assert.Equal(t, "clean-minimal", household.Presentation.TemplateID)
	assert.Equal(t, "#E54666", household.Presentation.PrimaryColor)
	assert.Equal(t, "#f472b6", household.Presentation.SecondaryColor)
	assert.Equal(t, "Inter", household.Presentation.Font)
	assert.Empty(t, household.Presentation.BackgroundImage)
}

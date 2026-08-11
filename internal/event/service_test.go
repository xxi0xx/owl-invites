package event

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/testutil"
)

func setupEvent(t *testing.T) (*Service, *auth.Store) {
	t.Helper()
	db := testutil.NewTestDB(t)
	store := NewStore(db)
	service := NewService(store, testutil.TestConfig().DefaultRetentionDays)
	service.SetCoHostStore(NewCoHostStore(db))
	return service, auth.NewStore(db)
}

func createOrganizer(t *testing.T, store *auth.Store) *auth.Organizer {
	t.Helper()
	organizer, err := store.CreateOrganizer(context.Background(), "organizer-"+t.Name()+"@example.com")
	require.NoError(t, err)
	return organizer
}

func TestEventLifecycleUsesMembershipOwnership(t *testing.T) {
	service, authStore := setupEvent(t)
	owner := createOrganizer(t, authStore)

	created, err := service.Create(context.Background(), owner.ID, CreateEventRequest{
		Title: "Birthday", EventDate: "2026-06-15T14:00", Location: "Park",
	})
	require.NoError(t, err)
	assert.Equal(t, owner.ID, created.OrganizerID)
	assert.Equal(t, "draft", created.Status)
	assert.Equal(t, "America/New_York", created.Timezone)

	created.Title = "ignored local mutation"
	found, err := service.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Birthday", found.Title)

	updatedTitle := "Updated birthday"
	updated, err := service.Update(context.Background(), created.ID, owner.ID, UpdateEventRequest{Title: &updatedTitle})
	require.NoError(t, err)
	assert.Equal(t, updatedTitle, updated.Title)

	published, err := service.Publish(context.Background(), created.ID, owner.ID)
	require.NoError(t, err)
	assert.Equal(t, "published", published.Status)

	cancelled, err := service.Cancel(context.Background(), created.ID, owner.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", cancelled.Status)
}

func TestEventDeadlineAndVisibility(t *testing.T) {
	service, authStore := setupEvent(t)
	owner := createOrganizer(t, authStore)
	visible := true
	deadline := "2026-06-14T14:00:00Z"

	created, err := service.Create(context.Background(), owner.ID, CreateEventRequest{
		Title: "Deadline", EventDate: "2026-06-15T14:00:00Z",
		RSVPDeadline: &deadline, ShowHeadcount: &visible, ShowGuestList: &visible,
	})
	require.NoError(t, err)
	require.NotNil(t, created.RSVPDeadline)
	assert.True(t, created.ShowHeadcount)
	assert.True(t, created.ShowGuestList)

	after := "2026-06-16T14:00:00Z"
	_, err = service.Update(context.Background(), created.ID, owner.ID, UpdateEventRequest{RSVPDeadline: &after})
	require.ErrorContains(t, err, "on or before")
}

func TestDuplicateCopiesEventMetadataWithoutInvitations(t *testing.T) {
	service, authStore := setupEvent(t)
	owner := createOrganizer(t, authStore)
	created, err := service.Create(context.Background(), owner.ID, CreateEventRequest{
		Title: "Original", EventDate: "2026-06-15T14:00:00Z", Description: "Details",
	})
	require.NoError(t, err)

	duplicate, err := service.Duplicate(context.Background(), created.ID, owner.ID)
	require.NoError(t, err)
	assert.NotEqual(t, created.ID, duplicate.ID)
	assert.Equal(t, "Copy of Original", duplicate.Title)
	assert.Equal(t, "draft", duplicate.Status)
}

func TestEventValidationAndAuthorization(t *testing.T) {
	service, authStore := setupEvent(t)
	owner := createOrganizer(t, authStore)
	other, err := authStore.CreateOrganizer(context.Background(), "other@example.com")
	require.NoError(t, err)

	_, err = service.Create(context.Background(), owner.ID, CreateEventRequest{EventDate: "2026-06-15T14:00"})
	require.ErrorContains(t, err, "title is required")

	created, err := service.Create(context.Background(), owner.ID, CreateEventRequest{Title: "Private", EventDate: "2026-06-15T14:00"})
	require.NoError(t, err)
	_, err = service.Update(context.Background(), created.ID, other.ID, UpdateEventRequest{})
	require.ErrorContains(t, err, "forbidden")
}

package eventadmin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/event"
	"github.com/yannkr/openrsvp/internal/instanceconfig"
	"github.com/yannkr/openrsvp/internal/testutil"
	"github.com/yannkr/openrsvp/internal/useradmin"
)

func setupEventAdmin(t *testing.T) (*Service, *event.Service, *auth.Store, *auth.User, *auth.User, *event.Event, *useradmin.Store) {
	t.Helper()
	db := testutil.NewTestDB(t)
	authStore := auth.NewStore(db)
	owner, err := authStore.CreateOrganizer(context.Background(), "owner@example.com")
	require.NoError(t, err)
	admin, err := authStore.CreateOrganizer(context.Background(), "admin@example.com")
	require.NoError(t, err)
	require.NoError(t, authStore.SetAdminStatus(context.Background(), admin.ID, true))
	admin, err = authStore.FindOrganizerByID(context.Background(), admin.ID)
	require.NoError(t, err)
	eventService := event.NewService(event.NewStore(db), 30)
	ev, err := eventService.Create(context.Background(), owner.ID, event.CreateEventRequest{
		Title: "Private event", EventDate: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339), Timezone: "UTC",
	})
	require.NoError(t, err)
	return NewService(db, authStore, instanceconfig.NewStore(db)), eventService, authStore, owner, admin, ev, useradmin.NewStore(db)
}

func TestInstanceAdminHasNoImplicitEventAccessAndCanJoinExplicitly(t *testing.T) {
	service, events, _, _, admin, ev, audit := setupEventAdmin(t)
	canManage, err := events.CanManageEvent(context.Background(), ev.ID, admin.ID)
	require.NoError(t, err)
	assert.False(t, canManage)

	require.NoError(t, service.AddSelf(context.Background(), admin, ev.ID))
	canManage, err = events.CanManageEvent(context.Background(), ev.ID, admin.ID)
	require.NoError(t, err)
	assert.True(t, canManage)

	entries, err := audit.ListAudit(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "administrator_joined_event", entries[0].Action)
}

func TestAdministrativeOwnershipTransferChangesExplicitRolesAndAudits(t *testing.T) {
	service, events, authStore, owner, admin, ev, audit := setupEventAdmin(t)
	newOwner, err := authStore.CreateOrganizer(context.Background(), "new-owner@example.com")
	require.NoError(t, err)

	require.NoError(t, service.TransferOwnership(context.Background(), admin, ev.ID, newOwner.ID))
	isOwner, err := events.IsEventOwner(context.Background(), ev.ID, newOwner.ID)
	require.NoError(t, err)
	assert.True(t, isOwner)
	isOwner, err = events.IsEventOwner(context.Background(), ev.ID, owner.ID)
	require.NoError(t, err)
	assert.False(t, isOwner)
	canManage, err := events.CanManageEvent(context.Background(), ev.ID, owner.ID)
	require.NoError(t, err)
	assert.True(t, canManage, "the previous owner remains an explicit cohost")

	entries, err := audit.ListAudit(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "event_ownership_transferred", entries[0].Action)
}

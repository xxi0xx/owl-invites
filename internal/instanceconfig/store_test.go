package instanceconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/testutil"
)

func setupStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(testutil.NewTestDB(t))
}

func TestStoreSeedsTypedSingletonWithSafeDefaults(t *testing.T) {
	store := setupStore(t)

	instance, err := store.GetInstance(context.Background())
	require.NoError(t, err)
	assert.Equal(t, singletonInstanceID, instance.ID)
	assert.Equal(t, "Owl Invites", instance.Name)
	assert.Equal(t, "UTC", instance.DefaultTimezone)
	assert.False(t, instance.AllowSignups)
	assert.Nil(t, instance.SetupCompletedAt)
}

func TestStoreUpdateSettingsDoesNotCompleteBootstrap(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()

	require.NoError(t, store.UpdateSettings(ctx, &Settings{
		InstanceName: "My Party Hub", DefaultTimezone: "America/New_York",
		AllowSignups: true, SupportEmail: "support@example.com",
	}))

	all, err := store.GetAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		KeyInstanceName: "My Party Hub", KeyDefaultTimezone: "America/New_York",
		KeyAllowSignups: "true", KeySupportEmail: "support@example.com",
		KeyConfigured: "false",
	}, all)
	configured, err := store.IsConfigured(ctx)
	require.NoError(t, err)
	assert.False(t, configured)
}

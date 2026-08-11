package instanceconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/testutil"
)

func setupService(t *testing.T) *Service {
	t.Helper()
	return NewService(NewStore(testutil.NewTestDB(t)))
}

func TestService_GetSettings_Empty(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	settings, err := svc.GetSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Owl Invites", settings.InstanceName)
	assert.Equal(t, "UTC", settings.DefaultTimezone)
	assert.False(t, settings.AllowSignups)
	assert.False(t, settings.Configured)
}

func TestService_SaveAndGetSettings(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	in := &Settings{
		InstanceName:    "Neighborhood Events",
		DefaultTimezone: "Europe/Paris",
		AllowSignups:    true,
		SupportEmail:    "help@example.org",
	}
	require.NoError(t, svc.SaveSettings(ctx, in))

	got, err := svc.GetSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Neighborhood Events", got.InstanceName)
	assert.Equal(t, "Europe/Paris", got.DefaultTimezone)
	assert.True(t, got.AllowSignups)
	assert.Equal(t, "help@example.org", got.SupportEmail)
	assert.False(t, got.Configured, "ongoing settings must not complete bootstrap")
}

func TestService_SaveSettings_CannotSetConfiguredFlag(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	configured, err := svc.IsConfigured(ctx)
	require.NoError(t, err)
	assert.False(t, configured)

	require.NoError(t, svc.SaveSettings(ctx, &Settings{
		InstanceName:    "Hub",
		DefaultTimezone: "UTC",
	}))

	configured, err = svc.IsConfigured(ctx)
	require.NoError(t, err)
	assert.False(t, configured)
}

func TestService_GetPublicConfig(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()

	require.NoError(t, svc.SaveSettings(ctx, &Settings{
		InstanceName:    "Public Hub",
		DefaultTimezone: "UTC",
		AllowSignups:    false,
		SupportEmail:    "support@example.com",
	}))

	pub, err := svc.GetPublicConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Public Hub", pub.InstanceName)
	assert.Equal(t, "UTC", pub.DefaultTimezone)
	assert.False(t, pub.AllowSignups)
	assert.Equal(t, "support@example.com", pub.SupportEmail)
}

package instanceconfig

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/testutil"
)

func bootstrapFixture(t *testing.T) (*BootstrapService, *Service, *auth.Service, *auth.Store) {
	t.Helper()
	db := testutil.NewTestDB(t)
	instanceStore := NewStore(db)
	authStore := auth.NewStore(db)
	cfg := testutil.TestConfig()
	cfg.AllowSignups = false
	cfg.BootstrapToken = "correct horse battery staple"
	return NewBootstrapService(instanceStore, authStore, cfg),
		NewService(instanceStore), auth.NewService(authStore, cfg, zerolog.Nop()), authStore
}

func validBootstrapRequest() *BootstrapRequest {
	return &BootstrapRequest{
		BootstrapToken:  "correct horse battery staple",
		AdminEmail:      "admin@example.com",
		AdminName:       "First Admin",
		InstanceName:    "Owl Invites Community",
		DefaultTimezone: "America/Chicago",
		AllowSignups:    false,
		SupportEmail:    "support@example.com",
	}
}

func TestBootstrapAtomicallyCreatesAdminSettingsAndSession(t *testing.T) {
	bootstrap, settingsService, authService, users := bootstrapFixture(t)

	result, err := bootstrap.Bootstrap(context.Background(), validBootstrapRequest())
	require.NoError(t, err)
	require.NotEmpty(t, result.SessionToken)
	assert.Equal(t, auth.InstanceRoleAdmin, result.User.InstanceRole)
	assert.True(t, result.User.IsAdmin)

	settings, err := settingsService.GetSettings(context.Background())
	require.NoError(t, err)
	assert.True(t, settings.Configured)
	assert.Equal(t, "Owl Invites Community", settings.InstanceName)
	assert.False(t, settings.AllowSignups)

	validated, err := authService.ValidateSession(context.Background(), result.SessionToken)
	require.NoError(t, err)
	assert.Equal(t, result.User.ID, validated.ID)

	persisted, err := users.FindOrganizerByEmail(context.Background(), "ADMIN@example.com")
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, auth.InstanceRoleAdmin, persisted.InstanceRole)
}

func TestBootstrapRejectsWrongOrMissingTokenWithoutChangingState(t *testing.T) {
	bootstrap, settingsService, _, users := bootstrapFixture(t)
	req := validBootstrapRequest()
	req.BootstrapToken = "wrong"

	_, err := bootstrap.Bootstrap(context.Background(), req)
	assert.ErrorIs(t, err, ErrBootstrapUnauthorized)

	settings, err := settingsService.GetSettings(context.Background())
	require.NoError(t, err)
	assert.False(t, settings.Configured)
	user, err := users.FindOrganizerByEmail(context.Background(), req.AdminEmail)
	require.NoError(t, err)
	assert.Nil(t, user)
}

func TestBootstrapIsPermanentlyClosedAfterCommit(t *testing.T) {
	bootstrap, _, _, _ := bootstrapFixture(t)
	_, err := bootstrap.Bootstrap(context.Background(), validBootstrapRequest())
	require.NoError(t, err)

	_, err = bootstrap.Bootstrap(context.Background(), validBootstrapRequest())
	assert.ErrorIs(t, err, ErrSetupComplete)
}

func TestConcurrentBootstrapClaimsOnlyOneAdministrator(t *testing.T) {
	bootstrap, _, _, _ := bootstrapFixture(t)
	var successes atomic.Int32
	var completed atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := bootstrap.Bootstrap(context.Background(), validBootstrapRequest())
			switch err {
			case nil:
				successes.Add(1)
			case ErrSetupComplete:
				completed.Add(1)
			default:
				t.Errorf("unexpected bootstrap result: %v", err)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), successes.Load())
	assert.Equal(t, int32(1), completed.Load())
}

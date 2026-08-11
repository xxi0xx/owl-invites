package admincli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/instanceconfig"
	"github.com/xxi0xx/owl-invites/internal/testutil"
	"github.com/xxi0xx/owl-invites/internal/useradmin"
)

func TestPromoteAdminRecoversExistingDisabledUserAndAuditsCLIActor(t *testing.T) {
	db := testutil.NewTestDB(t)
	authStore := auth.NewStore(db)
	user, err := authStore.CreateOrganizer(context.Background(), "recover@example.com")
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), "UPDATE users SET status = ? WHERE id = ?", auth.UserStatusDisabled, user.ID)
	require.NoError(t, err)

	service := NewService(authStore, instanceconfig.NewStore(db))
	recovered, err := service.PromoteAdmin(context.Background(), " RECOVER@example.com ")
	require.NoError(t, err)
	assert.True(t, recovered.IsAdmin)
	assert.Equal(t, auth.UserStatusActive, recovered.Status)

	entries, err := useradmin.NewStore(db).ListAudit(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "cli", entries[0].ActorKind)
	assert.Equal(t, "emergency_role_recovery", entries[0].Action)
}

func TestPromoteAdminDoesNotCreateUnknownUser(t *testing.T) {
	db := testutil.NewTestDB(t)
	service := NewService(auth.NewStore(db), instanceconfig.NewStore(db))
	_, err := service.PromoteAdmin(context.Background(), "missing@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

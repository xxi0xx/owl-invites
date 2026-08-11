package useradmin

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/event"
	"github.com/xxi0xx/owl-invites/internal/testutil"
)

type adminFixture struct {
	service     *Service
	authService *auth.Service
	authStore   *auth.Store
	admin       *auth.User
	sentBody    string
}

func setupAdminService(t *testing.T) *adminFixture {
	t.Helper()
	db := testutil.NewTestDB(t)
	authStore := auth.NewStore(db)
	admin, err := authStore.CreateOrganizer(context.Background(), "admin@example.com")
	require.NoError(t, err)
	require.NoError(t, authStore.SetAdminStatus(context.Background(), admin.ID, true))
	admin, err = authStore.FindOrganizerByID(context.Background(), admin.ID)
	require.NoError(t, err)

	cfg := testutil.TestConfig()
	cfg.AllowSignups = false
	cfg.AccountInviteExpiry = 72 * time.Hour
	service := NewService(NewStore(db), authStore, cfg, zerolog.Nop())
	fixture := &adminFixture{
		service: service, authService: auth.NewService(authStore, cfg, zerolog.Nop()),
		authStore: authStore, admin: admin,
	}
	service.SetEmailSender(func(_ context.Context, _, _, html, plain string) error {
		fixture.sentBody = html + "\n" + plain
		return nil
	})
	return fixture
}

func TestAdminInviteActivatesThroughIssuedCapabilityWhenSignupsAreOff(t *testing.T) {
	fx := setupAdminService(t)

	invite, err := fx.service.InviteUser(context.Background(), fx.admin, "new-user@example.com")
	require.NoError(t, err)
	assert.Equal(t, "new-user@example.com", invite.Email)
	assert.NotContains(t, invite.TokenHash, invite.RawToken)
	assert.Contains(t, fx.sentBody, invite.RawToken, "the raw capability is delivered only by email")

	pending, err := fx.authStore.FindOrganizerByEmail(context.Background(), "new-user@example.com")
	require.NoError(t, err)
	require.NotNil(t, pending)
	assert.Equal(t, auth.UserStatusInvited, pending.Status)

	response, err := fx.service.AcceptInvite(context.Background(), invite.RawToken)
	require.NoError(t, err)
	assert.Equal(t, auth.UserStatusActive, response.Organizer.Status)
	assert.False(t, response.Organizer.IsAdmin)
	_, err = fx.authService.ValidateSession(context.Background(), response.Token)
	require.NoError(t, err)
}

func TestAccountInviteCapabilityIsOneTimeUnderConcurrency(t *testing.T) {
	fx := setupAdminService(t)
	invite, err := fx.service.InviteUser(context.Background(), fx.admin, "concurrent@example.com")
	require.NoError(t, err)

	var successes atomic.Int32
	var invalid atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, acceptErr := fx.service.AcceptInvite(context.Background(), invite.RawToken)
			switch {
			case acceptErr == nil:
				successes.Add(1)
			case errors.Is(acceptErr, ErrInvalidInvite):
				invalid.Add(1)
			default:
				t.Errorf("unexpected accept result: %v", acceptErr)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), successes.Load())
	assert.Equal(t, int32(1), invalid.Load())
}

func TestRevokedAccountInviteCannotBeAccepted(t *testing.T) {
	fx := setupAdminService(t)
	invite, err := fx.service.InviteUser(context.Background(), fx.admin, "revoked@example.com")
	require.NoError(t, err)
	require.NoError(t, fx.service.RevokeInvite(context.Background(), fx.admin, invite.ID))

	_, err = fx.service.AcceptInvite(context.Background(), invite.RawToken)
	assert.ErrorIs(t, err, ErrInvalidInvite)
	pending, err := fx.service.ListPendingInvites(context.Background())
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestOwnerCanSponsorCohostAccountWhenPublicSignupsAreOff(t *testing.T) {
	fx := setupAdminService(t)
	events := event.NewService(event.NewStore(fx.service.store.db), 30)
	ev, err := events.Create(context.Background(), fx.admin.ID, event.CreateEventRequest{
		Title: "Sponsored", EventDate: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), Timezone: "UTC",
	})
	require.NoError(t, err)

	pending, err := fx.service.InviteEventCohost(context.Background(), fx.admin, ev.ID, "new-cohost@example.com")
	require.NoError(t, err)
	assert.True(t, pending)
	invited, err := fx.authStore.FindOrganizerByEmail(context.Background(), "new-cohost@example.com")
	require.NoError(t, err)
	assert.Equal(t, auth.UserStatusInvited, invited.Status)

	match := regexp.MustCompile(`#token=([0-9a-f]{64})`).FindStringSubmatch(fx.sentBody)
	require.Len(t, match, 2)
	raw := match[1]
	response, err := fx.service.AcceptInvite(context.Background(), raw)
	require.NoError(t, err)
	canManage, err := events.CanManageEvent(context.Background(), ev.ID, response.Organizer.ID)
	require.NoError(t, err)
	assert.True(t, canManage)
}

func TestOwnerAddsExistingActiveCohostWithoutAccountInvite(t *testing.T) {
	fx := setupAdminService(t)
	events := event.NewService(event.NewStore(fx.service.store.db), 30)
	ev, err := events.Create(context.Background(), fx.admin.ID, event.CreateEventRequest{
		Title: "Existing", EventDate: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), Timezone: "UTC",
	})
	require.NoError(t, err)
	cohost, err := fx.authStore.CreateOrganizer(context.Background(), "existing-cohost@example.com")
	require.NoError(t, err)

	pending, err := fx.service.InviteEventCohost(context.Background(), fx.admin, ev.ID, cohost.Email)
	require.NoError(t, err)
	assert.False(t, pending)
	canManage, err := events.CanManageEvent(context.Background(), ev.ID, cohost.ID)
	require.NoError(t, err)
	assert.True(t, canManage)
}

func TestInstanceAdminCannotSponsorCohostWithoutOwnerMembership(t *testing.T) {
	fx := setupAdminService(t)
	owner, err := fx.authStore.CreateOrganizer(context.Background(), "different-owner@example.com")
	require.NoError(t, err)
	events := event.NewService(event.NewStore(fx.service.store.db), 30)
	ev, err := events.Create(context.Background(), owner.ID, event.CreateEventRequest{
		Title: "No implicit admin", EventDate: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), Timezone: "UTC",
	})
	require.NoError(t, err)

	_, err = fx.service.InviteEventCohost(context.Background(), fx.admin, ev.ID, "blocked@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestPendingSponsoredInvitesReserveCohostCapacity(t *testing.T) {
	fx := setupAdminService(t)
	fx.service.cfg.MaxCoHostsPerEvent = 1
	events := event.NewService(event.NewStore(fx.service.store.db), 30)
	ev, err := events.Create(context.Background(), fx.admin.ID, event.CreateEventRequest{
		Title: "Capacity", EventDate: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), Timezone: "UTC",
	})
	require.NoError(t, err)

	_, err = fx.service.InviteEventCohost(context.Background(), fx.admin, ev.ID, "reserved@example.com")
	require.NoError(t, err)
	_, err = fx.service.InviteEventCohost(context.Background(), fx.admin, ev.ID, "over-limit@example.com")
	assert.ErrorIs(t, err, ErrCohostLimit)
}

func TestAdminCannotDisableOrDemoteLastActiveAdmin(t *testing.T) {
	fx := setupAdminService(t)

	err := fx.service.SetUserStatus(context.Background(), fx.admin, fx.admin.ID, auth.UserStatusDisabled)
	assert.ErrorIs(t, err, ErrLastAdmin)
	err = fx.service.SetUserRole(context.Background(), fx.admin, fx.admin.ID, auth.InstanceRoleUser)
	assert.ErrorIs(t, err, ErrLastAdmin)
}

func TestSensitiveUserChangesAreAudited(t *testing.T) {
	fx := setupAdminService(t)
	user, err := fx.authStore.CreateOrganizer(context.Background(), "member@example.com")
	require.NoError(t, err)

	require.NoError(t, fx.service.SetUserStatus(context.Background(), fx.admin, user.ID, auth.UserStatusDisabled))
	require.NoError(t, fx.service.SetUserStatus(context.Background(), fx.admin, user.ID, auth.UserStatusActive))
	require.NoError(t, fx.service.SetUserRole(context.Background(), fx.admin, user.ID, auth.InstanceRoleAdmin))

	entries, err := fx.service.ListAudit(context.Background(), 20)
	require.NoError(t, err)
	actions := make([]string, 0, len(entries))
	for _, entry := range entries {
		actions = append(actions, entry.Action)
	}
	joined := strings.Join(actions, ",")
	assert.Contains(t, joined, "user_disabled")
	assert.Contains(t, joined, "user_enabled")
	assert.Contains(t, joined, "instance_role_changed")
}

func TestDisablingUserRevokesExistingSessions(t *testing.T) {
	fx := setupAdminService(t)
	user, err := fx.authStore.CreateOrganizer(context.Background(), "session-user@example.com")
	require.NoError(t, err)
	rawSession := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err = fx.authStore.CreateSession(context.Background(), tokenDigest(rawSession), user.ID, time.Now().UTC().Add(time.Hour))
	require.NoError(t, err)
	_, err = fx.authService.ValidateSession(context.Background(), rawSession)
	require.NoError(t, err)

	require.NoError(t, fx.service.SetUserStatus(context.Background(), fx.admin, user.ID, auth.UserStatusDisabled))
	_, err = fx.authService.ValidateSession(context.Background(), rawSession)
	assert.ErrorIs(t, err, auth.ErrSessionNotFound)
}

package useradmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/testutil"
)

func setupAdminHandler(t *testing.T) (*Handler, *Service, *auth.User) {
	t.Helper()
	db := testutil.NewTestDB(t)
	authStore := auth.NewStore(db)
	admin, err := authStore.CreateOrganizer(context.Background(), "handler-admin@example.com")
	require.NoError(t, err)
	require.NoError(t, authStore.SetAdminStatus(context.Background(), admin.ID, true))
	admin, err = authStore.FindOrganizerByID(context.Background(), admin.ID)
	require.NoError(t, err)
	cfg := testutil.TestConfig()
	service := NewService(NewStore(db), authStore, cfg, zerolog.Nop())
	service.SetEmailSender(func(context.Context, string, string, string, string) error { return nil })
	authMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.ContextWithOrganizer(r.Context(), admin)))
		})
	}
	return NewHandler(service, cfg, authMW, auth.RequireAdmin(), zerolog.Nop()), service, admin
}

func TestAdminInviteHandlerRejectsUnknownJSONAndNeverReturnsCapability(t *testing.T) {
	handler, _, _ := setupAdminHandler(t)

	bad := httptest.NewRecorder()
	handler.UserRoutes().ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/invites", strings.NewReader(`{"email":"new@example.com","role":"admin"}`)))
	assert.Equal(t, http.StatusBadRequest, bad.Code)

	good := httptest.NewRecorder()
	handler.UserRoutes().ServeHTTP(good, httptest.NewRequest(http.MethodPost, "/invites", strings.NewReader(`{"email":"new@example.com"}`)))
	require.Equal(t, http.StatusCreated, good.Code, good.Body.String())
	assert.NotContains(t, good.Body.String(), "token")
}

func TestAcceptInviteHandlerCreatesCookieWithoutReturningSessionToken(t *testing.T) {
	handler, service, admin := setupAdminHandler(t)
	invite, err := service.InviteUser(context.Background(), admin, "accept@example.com")
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	handler.PublicRoutes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/accept", strings.NewReader(`{"token":"`+invite.RawToken+`"}`)))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), invite.RawToken)
	assert.NotContains(t, recorder.Body.String(), `"token"`)

	foundSession := false
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "session" && cookie.Value != "" && cookie.HttpOnly {
			foundSession = true
		}
	}
	assert.True(t, foundSession)
}

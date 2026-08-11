package instanceconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/testutil"
)

func setupHandler(t *testing.T) (*Handler, *Service) {
	t.Helper()
	db := testutil.NewTestDB(t)
	store := NewStore(db)
	authStore := auth.NewStore(db)
	cfg := testutil.TestConfig()
	cfg.BootstrapToken = "bootstrap-only-secret"
	service := NewService(store)
	bootstrap := NewBootstrapService(store, authStore, cfg)
	handler := NewHandler(service, bootstrap, cfg, testutil.NoAuthMiddleware(), testutil.NoAuthMiddleware(), zerolog.Nop())
	return handler, service
}

func TestBootstrapHandlerUsesStrictJSONAndDoesNotExposeTokens(t *testing.T) {
	handler, service := setupHandler(t)
	body := `{
		"bootstrapToken":"bootstrap-only-secret",
		"adminEmail":"admin@example.com",
		"adminName":"Admin",
		"instanceName":"Owl Invites",
		"defaultTimezone":"UTC",
		"allowSignups":false,
		"supportEmail":"",
		"unexpected":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/bootstrap", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	configured, err := service.IsConfigured(context.Background())
	require.NoError(t, err)
	assert.False(t, configured)

	body = strings.ReplaceAll(body, ",\n\t\t\"unexpected\":true", "")
	req = httptest.NewRequest(http.MethodPost, "/bootstrap", strings.NewReader(body))
	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "bootstrap-only-secret")
	assert.NotContains(t, rec.Body.String(), `"token"`)

	var sessionCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "session" {
			sessionCookie = cookie
		}
	}
	require.NotNil(t, sessionCookie)
	assert.NotEmpty(t, sessionCookie.Value)
	assert.True(t, sessionCookie.HttpOnly)
}

func TestBootstrapHandlerClosesRouteAfterSetup(t *testing.T) {
	handler, _ := setupHandler(t)
	body := `{"bootstrapToken":"bootstrap-only-secret","adminEmail":"admin@example.com","adminName":"Admin","instanceName":"Owl Invites","defaultTimezone":"UTC","allowSignups":false,"supportEmail":""}`

	first := httptest.NewRecorder()
	handler.Routes().ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/bootstrap", strings.NewReader(body)))
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	second := httptest.NewRecorder()
	handler.Routes().ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/bootstrap", strings.NewReader(body)))
	assert.Equal(t, http.StatusNotFound, second.Code)
}

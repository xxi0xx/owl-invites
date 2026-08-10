package invitation

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func publicHandlerFixture(t *testing.T) (*serviceFixture, http.Handler, *bytes.Buffer) {
	t.Helper()
	f := newServiceFixture(t)
	logs := &bytes.Buffer{}
	handler := NewHandler(f.service, func(next http.Handler) http.Handler { return next },
		func(_ context.Context) (string, bool) { return "", false },
		func(_ context.Context, _, _ string) error { return nil }, false, zerolog.New(logs))
	router := chi.NewRouter()
	router.Mount("/invitations", handler.PublicRoutes())
	return f, router, logs
}

func postPublicJSON(t *testing.T, handler http.Handler, path string, value any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.25:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestCapabilityExchangeDoesNotLeakCapabilityAndSetsScopedCookie(t *testing.T) {
	f, handler, logs := publicHandlerFixture(t)
	created := f.create("Household", "household@example.com", 0, "Guest")
	capability := capabilityFromURL(created.AccessURL)

	response := postPublicJSON(t, handler, "/invitations/exchange", map[string]string{"capability": capability})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.NotContains(t, response.Body.String(), capability)
	assert.NotContains(t, response.Body.String(), "accessUrl")
	require.Empty(t, logs.String())

	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, invitationSessionCookie, cookies[0].Name)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)

	tampered := capability[:len(capability)-1] + "A"
	invalid := postPublicJSON(t, handler, "/invitations/exchange", map[string]string{"capability": tampered})
	assert.Equal(t, http.StatusUnauthorized, invalid.Code)
	assert.Equal(t, "no-store", invalid.Header().Get("Cache-Control"))
	assert.NotContains(t, invalid.Body.String(), tampered)
	assert.NotContains(t, logs.String(), tampered)
}

func TestRecoveryRequestIsEnumerationResistant(t *testing.T) {
	f, handler, logs := publicHandlerFixture(t)
	f.create("Recoverable", "stored@example.com", 0, "Guest")
	f.service.SetEmailSender(func(_ context.Context, _, _, _, _, _, _ string) error { return nil })

	existing := postPublicJSON(t, handler, "/invitations/recovery/request", RecoveryRequest{
		EventID: f.eventID, Contact: "stored@example.com",
	})
	missing := postPublicJSON(t, handler, "/invitations/recovery/request", RecoveryRequest{
		EventID: f.eventID, Contact: "missing@example.com",
	})

	assert.Equal(t, http.StatusOK, existing.Code)
	assert.Equal(t, existing.Code, missing.Code)
	assert.Equal(t, existing.Body.String(), missing.Body.String())
	assert.NotContains(t, existing.Body.String(), "stored@example.com")
	assert.NotContains(t, missing.Body.String(), "missing@example.com")
	assert.NotContains(t, logs.String(), "stored@example.com")
	assert.NotContains(t, logs.String(), "missing@example.com")
	assert.True(t, strings.Contains(existing.Body.String(), "If a matching invitation exists"))
}

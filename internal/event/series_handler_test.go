package event

import (
	"context"
	"net/http"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/database"
	"github.com/xxi0xx/owl-invites/internal/testutil"
)

// setupSeriesHandler returns a series handler with fake auth, plus the backing
// DB so a test can close it to force genuine internal failures.
func setupSeriesHandler(t *testing.T) (http.Handler, database.DB, *auth.Organizer) {
	t.Helper()
	db := testutil.NewTestDB(t)
	cfg := testutil.TestConfig()

	eventStore := NewStore(db)
	eventService := NewService(eventStore, cfg.DefaultRetentionDays)
	seriesStore := NewSeriesStore(db)
	seriesService := NewSeriesService(seriesStore, eventStore, eventService, cfg.DefaultRetentionDays, zerolog.Nop())

	authStore := auth.NewStore(db)
	org, err := authStore.CreateOrganizer(context.Background(), "organizer@example.com")
	require.NoError(t, err)

	authMW := testutil.FakeAuthMiddleware(func(ctx context.Context) context.Context {
		return auth.ContextWithOrganizer(ctx, org)
	})
	handler := NewSeriesHandler(seriesService, authMW, organizerFromCtx(), zerolog.Nop())
	return handler.Routes(), db, org
}

func validSeriesBody() map[string]any {
	return map[string]any{
		"title":          "Weekly Standup",
		"startDate":      "2027-06-01",
		"eventTime":      "14:00",
		"recurrenceRule": "weekly",
	}
}

// TestHandleCreateSeries_ValidationErrorReturns400 pins the correct half of the
// existing behaviour so the internal-error fix cannot regress it.
func TestHandleCreateSeries_ValidationErrorReturns400(t *testing.T) {
	h, _, _ := setupSeriesHandler(t)

	body := validSeriesBody()
	body["recurrenceRule"] = "hourly"
	rr := testutil.DoRequest(t, h, "POST", "/", body)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	parsed := testutil.ParseJSON(t, rr)
	assert.Equal(t, "bad_request", parsed["error"])
	assert.Contains(t, parsed["message"], "invalid recurrenceRule")
}

// TestHandleCreateSeries_InternalErrorReturns500 covers the inverse of the
// allowlist bug: this handler reported *every* failure as 400 bad_request, so a
// database outage was blamed on the caller and the raw driver error was echoed
// back to the client.
func TestHandleCreateSeries_InternalErrorReturns500(t *testing.T) {
	h, db, _ := setupSeriesHandler(t)
	require.NoError(t, db.Close()) // force genuine store failures

	rr := testutil.DoRequest(t, h, "POST", "/", validSeriesBody())

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	parsed := testutil.ParseJSON(t, rr)
	assert.Equal(t, "internal_error", parsed["error"])
	// The raw driver error must not reach the client.
	assert.NotContains(t, parsed["message"], "database")
	assert.NotContains(t, parsed["message"], "sql")
}

func TestHandleUpdateSeries_InternalErrorReturns500(t *testing.T) {
	h, db, _ := setupSeriesHandler(t)
	require.NoError(t, db.Close())

	rr := testutil.DoRequest(t, h, "PUT", "/some-series-id", map[string]any{"title": "Renamed"})

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	parsed := testutil.ParseJSON(t, rr)
	assert.Equal(t, "internal_error", parsed["error"])
	assert.NotContains(t, parsed["message"], "sql")
}

func TestHandleStopSeries_InternalErrorReturns500(t *testing.T) {
	h, db, _ := setupSeriesHandler(t)
	require.NoError(t, db.Close())

	rr := testutil.DoRequest(t, h, "POST", "/some-series-id/stop", nil)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	parsed := testutil.ParseJSON(t, rr)
	assert.Equal(t, "internal_error", parsed["error"])
	assert.NotContains(t, parsed["message"], "sql")
}

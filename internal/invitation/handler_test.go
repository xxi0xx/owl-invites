package invitation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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

func organizerHandlerFixture(t *testing.T) (*serviceFixture, http.Handler, *bytes.Buffer) {
	t.Helper()
	f := newServiceFixture(t)
	logs := &bytes.Buffer{}
	handler := NewHandler(f.service, func(next http.Handler) http.Handler { return next },
		func(_ context.Context) (string, bool) { return f.userID, true },
		func(_ context.Context, _, _ string) error { return nil }, false, zerolog.New(logs))
	router := chi.NewRouter()
	router.Route("/events/{eventId}", func(r chi.Router) {
		r.Mount("/invitations", handler.OrganizerInvitationRoutes())
	})
	return f, router, logs
}

func requestJSON(t *testing.T, handler http.Handler, method, path string, value any,
	cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if value != nil {
		require.NoError(t, json.NewEncoder(&body).Encode(value))
	}
	req := httptest.NewRequest(method, path, &body)
	if value != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	req.RemoteAddr = "192.0.2.25:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
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

func postCSV(t *testing.T, handler http.Handler, path, contents string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "households.csv")
	require.NoError(t, err)
	_, err = part.Write([]byte(contents))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestOrganizerImportPreviewCommitAndPublicIsolation(t *testing.T) {
	f, organizer, _ := organizerHandlerFixture(t)
	csvData := importHeader +
		"one,First Household,same@example.com,,email,1,Alex\n" +
		"one,First Household,same@example.com,,email,1,Bailey\n" +
		"two,Second Household,same@example.com,,email,0,Casey\n"
	previewResponse := postCSV(t, organizer,
		"/events/"+f.eventID+"/invitations/import/preview", csvData)
	require.Equal(t, http.StatusOK, previewResponse.Code, previewResponse.Body.String())
	var previewEnvelope struct {
		Data ImportPreview `json:"data"`
	}
	require.NoError(t, json.Unmarshal(previewResponse.Body.Bytes(), &previewEnvelope))
	require.Empty(t, previewEnvelope.Data.Errors)
	assert.Equal(t, 2, previewEnvelope.Data.HouseholdCount)
	assert.Equal(t, 3, previewEnvelope.Data.AssignedGuestCount)
	items, err := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, err)
	assert.Empty(t, items)

	commitResponse := requestJSON(t, organizer, http.MethodPost,
		"/events/"+f.eventID+"/invitations/import/commit",
		ImportCommitRequest{Households: previewEnvelope.Data.Households}, nil)
	require.Equal(t, http.StatusCreated, commitResponse.Code, commitResponse.Body.String())
	items, err = f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, err)
	assert.Len(t, items, 2)
	filtered := requestJSON(t, organizer, http.MethodGet,
		"/events/"+f.eventID+"/invitations?search=Second&response=not_submitted&attendance=pending", nil, nil)
	require.Equal(t, http.StatusOK, filtered.Code, filtered.Body.String())
	assert.Contains(t, filtered.Body.String(), "Second Household")
	assert.NotContains(t, filtered.Body.String(), "First Household")

	updateResponse := requestJSON(t, organizer, http.MethodPut,
		"/events/"+f.eventID+"/invitations/"+items[0].Invitation.ID,
		UpdateInvitationRequest{
			Label: "Updated First Household", ContactEmail: ptr("updated@example.com"),
			PreferredDeliveryMethod: "email", AdditionalGuestAllowance: 2,
			AssignedGuests: []AssignedGuestEdit{
				{ID: items[0].Guests[0].ID, Name: "Updated Alex"},
				{ID: items[0].Guests[1].ID, Name: "Bailey"},
			},
		}, nil)
	require.Equal(t, http.StatusOK, updateResponse.Code, updateResponse.Body.String())
	assert.Contains(t, updateResponse.Body.String(), "Updated First Household")
	assert.NotContains(t, updateResponse.Body.String(), "accessUrl",
		"ordinary organizer updates must not expose a raw household capability")
	exportResponse := requestJSON(t, organizer, http.MethodGet,
		"/events/"+f.eventID+"/invitations/export", nil, nil)
	require.Equal(t, http.StatusOK, exportResponse.Code, exportResponse.Body.String())
	assert.Equal(t, "text/csv; charset=utf-8", exportResponse.Header().Get("Content-Type"))
	assert.Contains(t, exportResponse.Body.String(), "invitation_id")
	assert.Contains(t, exportResponse.Body.String(), "First Household")

	_, public, _ := publicHandlerFixture(t)
	publicResponse := postCSV(t, public, "/invitations/import/preview", csvData)
	assert.Equal(t, http.StatusNotFound, publicResponse.Code,
		"public invitation routes must never expose organizer import")
	publicExport := requestJSON(t, public, http.MethodGet, "/invitations/export", nil, nil)
	assert.Equal(t, http.StatusNotFound, publicExport.Code,
		"public invitation routes must never expose organizer export")
	publicUpdate := requestJSON(t, public, http.MethodPut, "/invitations/not-public", UpdateInvitationRequest{}, nil)
	assert.Equal(t, http.StatusNotFound, publicUpdate.Code,
		"public invitation routes must never expose organizer household editing")
}

func TestOrganizerMessagePreviewAndAggregateDeliveryResult(t *testing.T) {
	f, organizer, _ := organizerHandlerFixture(t)
	f.create("First", "first@example.com", 0, "First")
	f.create("Second", "second@example.com", 0, "Second")
	f.service.SetEmailSender(func(_ context.Context, _, _, to, _, _, _ string) error {
		if to == "second@example.com" {
			return errors.New("deliberate delivery failure")
		}
		return nil
	})
	preview := requestJSON(t, organizer, http.MethodPost,
		"/events/"+f.eventID+"/invitations/messages/preview",
		MessagePreviewRequest{RecipientGroup: "all"}, nil)
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	assert.Contains(t, preview.Body.String(), `"recipientHouseholds":2`)
	assert.NotContains(t, preview.Body.String(), "first@example.com")
	assert.NotContains(t, preview.Body.String(), "second@example.com")

	result := requestJSON(t, organizer, http.MethodPost,
		"/events/"+f.eventID+"/invitations/messages",
		MessageRequest{RecipientGroup: "all", Subject: "Update", Body: "Details"}, nil)
	require.Equal(t, http.StatusCreated, result.Code, result.Body.String())
	assert.Contains(t, result.Body.String(), `"attempted":2`)
	assert.Contains(t, result.Body.String(), `"accepted":1`)
	assert.Contains(t, result.Body.String(), `"failed":1`)
	assert.NotContains(t, result.Body.String(), "second@example.com")

	_, public, _ := publicHandlerFixture(t)
	publicPreview := requestJSON(t, public, http.MethodPost, "/invitations/messages/preview",
		MessagePreviewRequest{RecipientGroup: "all"}, nil)
	assert.Equal(t, http.StatusNotFound, publicPreview.Code)
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

	tampered := tamperCapability(capability)
	invalid := postPublicJSON(t, handler, "/invitations/exchange", map[string]string{"capability": tampered})
	assert.Equal(t, http.StatusUnauthorized, invalid.Code)
	assert.Equal(t, "no-store", invalid.Header().Get("Cache-Control"))
	assert.NotContains(t, invalid.Body.String(), tampered)
	assert.NotContains(t, logs.String(), tampered)
}

func TestRecoveryRequestIsEnumerationResistant(t *testing.T) {
	f, handler, logs := publicHandlerFixture(t)
	f.create("Recoverable", "stored@example.com", 0, "Guest")
	deliveryStarted := make(chan struct{})
	deliveryFinished := make(chan struct{})
	var startedOnce sync.Once
	f.service.SetEmailSender(func(_ context.Context, _, _, _, _, _, _ string) error {
		startedOnce.Do(func() { close(deliveryStarted) })
		time.Sleep(500 * time.Millisecond)
		close(deliveryFinished)
		return nil
	})

	existingStarted := time.Now()
	existing := postPublicJSON(t, handler, "/invitations/recovery/request", RecoveryRequest{
		EventID: f.eventID, Contact: "stored@example.com",
	})
	existingDuration := time.Since(existingStarted)
	select {
	case <-deliveryStarted:
	case <-time.After(time.Second):
		t.Fatal("matching recovery delivery did not start")
	}
	missingStarted := time.Now()
	missing := postPublicJSON(t, handler, "/invitations/recovery/request", RecoveryRequest{
		EventID: f.eventID, Contact: "missing@example.com",
	})
	missingDuration := time.Since(missingStarted)
	select {
	case <-deliveryFinished:
	case <-time.After(time.Second):
		t.Fatal("matching recovery delivery did not finish")
	}

	assert.Equal(t, http.StatusOK, existing.Code)
	assert.Equal(t, existing.Code, missing.Code)
	assert.Equal(t, existing.Body.String(), missing.Body.String())
	assert.NotContains(t, existing.Body.String(), "stored@example.com")
	assert.NotContains(t, missing.Body.String(), "missing@example.com")
	assert.NotContains(t, logs.String(), "stored@example.com")
	assert.NotContains(t, logs.String(), "missing@example.com")
	assert.True(t, strings.Contains(existing.Body.String(), "If a matching invitation exists"))
	assert.GreaterOrEqual(t, existingDuration, recoveryResponseFloor)
	assert.GreaterOrEqual(t, missingDuration, recoveryResponseFloor)
	assert.Less(t, existingDuration, 300*time.Millisecond,
		"the deliberately delayed stored-destination delivery must not delay the public response")
	assert.Less(t, missingDuration, 300*time.Millisecond)
	delta := existingDuration - missingDuration
	if delta < 0 {
		delta = -delta
	}
	assert.Less(t, delta, 75*time.Millisecond,
		"matching and missing contacts should have no obvious public timing distinction")
}

func TestOpenEnrollmentDeliveryFailureKeepsCommittedSessionAndCapacity(t *testing.T) {
	f, handler, logs := publicHandlerFixture(t)
	_, accessURL, err := f.service.ConfigureOpen(context.Background(), f.eventID, f.userID,
		ConfigureOpenRequest{Enabled: true, MaxPartySize: 1, Capacity: intPtr(1)})
	require.NoError(t, err)
	capability := capabilityFromURL(accessURL)
	deliveryAttempts := 0
	f.service.SetEmailSender(func(_ context.Context, _, _, _, _, _, _ string) error {
		deliveryAttempts++
		return errors.New("deliberate SMTP failure")
	})

	response := postPublicJSON(t, handler, "/invitations/open/enroll", OpenEnrollmentRequest{
		Capability: capability, Label: "Committed household",
		ContactEmail: ptr("committed@example.com"), PreferredDeliveryMethod: "email",
		GuestNames: []string{"Committed Guest"},
	})
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	assert.Equal(t, 1, deliveryAttempts)
	assert.NotContains(t, response.Body.String(), capability)
	assert.NotContains(t, response.Body.String(), "accessUrl")

	var created struct {
		Data     Household      `json:"data"`
		Delivery DeliveryResult `json:"delivery"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	assert.Equal(t, DeliveryFailed, created.Delivery.Status)
	assert.NotEmpty(t, created.Delivery.Warning)
	require.NotNil(t, created.Data.Invitation)
	require.NotNil(t, created.Data.Response)
	require.Len(t, created.Data.Guests, 1)

	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	session := requestJSON(t, handler, http.MethodGet, "/invitations/session", nil, cookies[0])
	require.Equal(t, http.StatusOK, session.Code, session.Body.String())
	assert.Contains(t, session.Body.String(), created.Data.Invitation.ID)

	updated := requestJSON(t, handler, http.MethodPut, "/invitations/session/response", SubmitRequest{
		Version: created.Data.Response.Version,
		AssignedGuests: []GuestAttendanceInput{{
			GuestID: created.Data.Guests[0].ID, Attendance: AttendanceAttending,
		}},
		AdditionalGuests:  []AdditionalGuestInput{},
		InvitationAnswers: map[string]string{},
		GuestAnswers:      map[string]map[string]string{},
	}, cookies[0])
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Contains(t, updated.Body.String(), `"attendance":"attending"`)

	retry := postPublicJSON(t, handler, "/invitations/open/enroll", OpenEnrollmentRequest{
		Capability: capability, Label: "Retry household",
		ContactEmail: ptr("retry@example.com"), PreferredDeliveryMethod: "email",
		GuestNames: []string{"Retry Guest"},
	})
	assert.Equal(t, http.StatusConflict, retry.Code, retry.Body.String())
	assert.Equal(t, 1, deliveryAttempts, "a rolled-back capacity failure must not attempt delivery")
	items, err := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, created.Data.Invitation.ID, items[0].Invitation.ID)
	assert.Contains(t, logs.String(), "invitation persisted but delivery failed")
	assert.NotContains(t, logs.String(), capability)
}

func TestPrivateCreateDeliveryFailureReturnsCommittedResult(t *testing.T) {
	f, handler, logs := organizerHandlerFixture(t)
	f.service.SetEmailSender(func(_ context.Context, _, _, _, _, _, _ string) error {
		return errors.New("deliberate SMTP failure")
	})

	response := postPublicJSON(t, handler, "/events/"+f.eventID+"/invitations", CreateRequest{
		Label: "Committed private household", ContactEmail: ptr("private@example.com"),
		PreferredDeliveryMethod: "email", AssignedGuestNames: []string{"Private Guest"}, Send: true,
	})
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	var created struct {
		Data CreateResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	assert.Equal(t, DeliveryFailed, created.Data.Delivery.Status)
	assert.NotEmpty(t, created.Data.Delivery.Warning)
	assert.NotEmpty(t, created.Data.AccessURL)

	items, err := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	_, household, err := f.service.ExchangePrivate(context.Background(), capabilityFromURL(created.Data.AccessURL))
	require.NoError(t, err)
	assert.Equal(t, created.Data.Invitation.ID, household.Invitation.ID)
	assert.Contains(t, logs.String(), "invitation persisted but delivery failed")
	assert.NotContains(t, logs.String(), capabilityFromURL(created.Data.AccessURL))
}

func TestOpenEnrollmentRejectsPhoneOnlySMSBeforePersistence(t *testing.T) {
	f, handler, _ := publicHandlerFixture(t)
	_, accessURL, err := f.service.ConfigureOpen(context.Background(), f.eventID, f.userID,
		ConfigureOpenRequest{Enabled: true, MaxPartySize: 1})
	require.NoError(t, err)
	deliveryAttempts := 0
	f.service.SetEmailSender(func(_ context.Context, _, _, _, _, _, _ string) error {
		deliveryAttempts++
		return nil
	})

	response := postPublicJSON(t, handler, "/invitations/open/enroll", OpenEnrollmentRequest{
		Capability: capabilityFromURL(accessURL), Label: "Phone only",
		ContactPhone: ptr("+15551234567"), PreferredDeliveryMethod: "sms",
		GuestNames: []string{"Phone Guest"},
	})
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Equal(t, 0, deliveryAttempts)
	items, err := f.store.ListByEvent(context.Background(), f.eventID)
	require.NoError(t, err)
	assert.Empty(t, items)
}

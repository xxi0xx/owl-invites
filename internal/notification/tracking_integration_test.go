package notification

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yannkr/openrsvp/internal/testutil"
)

// captureProvider is a test email provider that records the last message it
// was asked to send and returns a fixed message ID.
type captureProvider struct {
	last      *Message
	messageID string
}

type failingProvider struct {
	attempts int
	err      error
}

func (p *failingProvider) Name() string     { return "failing" }
func (p *failingProvider) Channel() Channel { return ChannelEmail }
func (p *failingProvider) Send(context.Context, *Message) (*SendResult, error) {
	p.attempts++
	return nil, p.err
}
func (p *failingProvider) SendBatch(_ context.Context, msgs []*Message) ([]*SendResult, []error) {
	return make([]*SendResult, len(msgs)), make([]error, len(msgs))
}
func (p *failingProvider) HealthCheck(context.Context) error { return nil }

func (p *captureProvider) Name() string     { return "capture" }
func (p *captureProvider) Channel() Channel { return ChannelEmail }
func (p *captureProvider) Send(_ context.Context, msg *Message) (*SendResult, error) {
	// Copy so later mutation by the service does not race the assertion.
	cp := *msg
	p.last = &cp
	return &SendResult{MessageID: p.messageID}, nil
}
func (p *captureProvider) SendBatch(ctx context.Context, msgs []*Message) ([]*SendResult, []error) {
	results := make([]*SendResult, len(msgs))
	errs := make([]error, len(msgs))
	for i, m := range msgs {
		results[i], errs[i] = p.Send(ctx, m)
	}
	return results, errs
}
func (p *captureProvider) HealthCheck(context.Context) error { return nil }

// fakeSuppression is a test SuppressionChecker + Suppressor.
type fakeSuppression struct {
	suppressed  map[string]bool // email -> suppressed
	tokens      map[string]string
	suppressed2 map[string]string // email -> reason captured on Suppress
}

func newFakeSuppression() *fakeSuppression {
	return &fakeSuppression{
		suppressed:  map[string]bool{},
		tokens:      map[string]string{},
		suppressed2: map[string]string{},
	}
}

func (f *fakeSuppression) IsSuppressed(_ context.Context, email, _ string) bool {
	return f.suppressed[email]
}

func (f *fakeSuppression) GenerateUnsubscribeToken(_ context.Context, email, _ string) (string, error) {
	tok := "tok-" + email
	f.tokens[email] = tok
	return tok, nil
}

func (f *fakeSuppression) Suppress(_ context.Context, email, _, reason string) error {
	f.suppressed[email] = true
	f.suppressed2[email] = reason
	return nil
}

func newTestRegistry(p Provider) *Registry {
	r := NewRegistry()
	r.Register(p)
	return r
}

func TestService_OpenPixel_EmbeddedWhenEnabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	attendeeID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, attendeeID)

	prov := &captureProvider{messageID: "mid-1"}
	svc := NewServiceWithOptions(newTestRegistry(prov), db, zerolog.Nop(), Options{
		BaseURL:             "https://rsvp.example.com",
		OpenTrackingEnabled: true,
	})

	err := svc.Send(ctx, eventID, attendeeID, ChannelEmail, &Message{
		To:      "alice@example.com",
		Subject: "Hi",
		Body:    "<html><body><p>Hello</p></body></html>",
		Plain:   "Hello",
	})
	require.NoError(t, err)
	require.NotNil(t, prov.last)

	assert.Contains(t, prov.last.Body, "/api/v1/notifications/track/open/")
	assert.Contains(t, prov.last.Body, `width="1" height="1"`)
	// Plain text part must NOT get a pixel.
	assert.NotContains(t, prov.last.Plain, "track/open")

	// The pixel id must be the notification_log row id so RecordOpen works.
	var logID string
	err = db.QueryRowContext(ctx, "SELECT id FROM notification_log WHERE event_id = ?", eventID).Scan(&logID)
	require.NoError(t, err)
	assert.Contains(t, prov.last.Body, "/track/open/"+logID)
}

func TestServiceFailureRetriesAreBoundedAndTracked(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	invitationID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, invitationID)

	provider := &failingProvider{err: errors.New("deliberate SMTP outage")}
	var logs bytes.Buffer
	service := NewService(newTestRegistry(provider), db, zerolog.New(&logs))
	var delays []time.Duration
	service.retryWait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}

	err := service.Send(ctx, eventID, invitationID, ChannelEmail, &Message{To: "restore@example.com", Subject: "Retry", Body: "test"})
	require.Error(t, err)
	assert.Equal(t, maxSendAttempts, provider.attempts)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, delays)
	assert.Contains(t, logs.String(), "notification send failed, retrying")

	entries, err := service.GetLogsByEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "failed", entries[0].Status)
	assert.Contains(t, entries[0].Error, "deliberate SMTP outage")
}

func TestServiceCancellationStopsRetries(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	eventID := uuid.Must(uuid.NewV7()).String()
	invitationID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, invitationID)
	provider := &failingProvider{err: errors.New("provider unavailable")}
	service := NewService(newTestRegistry(provider), db, zerolog.Nop())
	service.retryWait = func(context.Context, time.Duration) error {
		t.Fatal("retry wait must not run after cancellation")
		return nil
	}
	cancel()

	err := service.Send(ctx, eventID, invitationID, ChannelEmail, &Message{To: "cancelled@example.com", Body: "test"})
	require.Error(t, err)
	assert.Equal(t, 1, provider.attempts)
}

func TestService_OpenPixel_AbsentWhenDisabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	attendeeID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, attendeeID)

	prov := &captureProvider{messageID: "mid-2"}
	svc := NewServiceWithOptions(newTestRegistry(prov), db, zerolog.Nop(), Options{
		BaseURL:             "https://rsvp.example.com",
		OpenTrackingEnabled: false, // disabled
	})

	err := svc.Send(ctx, eventID, attendeeID, ChannelEmail, &Message{
		To:      "alice@example.com",
		Subject: "Hi",
		Body:    "<html><body><p>Hello</p></body></html>",
	})
	require.NoError(t, err)
	require.NotNil(t, prov.last)
	assert.NotContains(t, prov.last.Body, "track/open")
}

func TestService_SuppressionGate_SkipsSuppressed(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	attendeeID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, attendeeID)

	fake := newFakeSuppression()
	fake.suppressed["blocked@example.com"] = true

	prov := &captureProvider{messageID: "mid-3"}
	svc := NewServiceWithOptions(newTestRegistry(prov), db, zerolog.Nop(), Options{
		BaseURL:     "https://rsvp.example.com",
		Suppression: fake,
	})

	err := svc.Send(ctx, eventID, attendeeID, ChannelEmail, &Message{
		To:      "blocked@example.com",
		Subject: "Hi",
		Body:    "<html><body>Hello</body></html>",
	})
	require.NoError(t, err)
	assert.Nil(t, prov.last, "suppressed recipient must not be sent to")

	// No notification_log row should have been written for the skipped send.
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_log WHERE event_id = ?", eventID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestService_UnsubscribeFooter_AddedWithSuppression(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	attendeeID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, attendeeID)

	fake := newFakeSuppression()
	prov := &captureProvider{messageID: "mid-4"}
	svc := NewServiceWithOptions(newTestRegistry(prov), db, zerolog.Nop(), Options{
		BaseURL:     "https://rsvp.example.com",
		Suppression: fake,
	})

	err := svc.Send(ctx, eventID, attendeeID, ChannelEmail, &Message{
		To:      "alice@example.com",
		Subject: "Hi",
		Body:    "<html><body><p>Hi</p></body></html>",
		Plain:   "Hi",
	})
	require.NoError(t, err)
	require.NotNil(t, prov.last)
	assert.Contains(t, prov.last.Body, "/unsubscribe?token=tok-alice@example.com")
	assert.Contains(t, prov.last.Plain, "/unsubscribe?token=tok-alice@example.com")
}

func TestService_UnsubscribeFooter_AbsentWithoutSuppression(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	attendeeID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, attendeeID)

	prov := &captureProvider{messageID: "mid-5"}
	svc := NewServiceWithOptions(newTestRegistry(prov), db, zerolog.Nop(), Options{
		BaseURL: "https://rsvp.example.com",
	})

	err := svc.Send(ctx, eventID, attendeeID, ChannelEmail, &Message{
		To:   "alice@example.com",
		Body: "<html><body>Hi</body></html>",
	})
	require.NoError(t, err)
	require.NotNil(t, prov.last)
	assert.NotContains(t, prov.last.Body, "unsubscribe")
}

// newTestHandler builds a Handler with stub auth/owner funcs for webhook tests.
func newTestHandler(tracking *TrackingService, svc *Service, suppressor Suppressor) *Handler {
	return NewHandler(
		tracking, svc, suppressor,
		func(next http.Handler) http.Handler { return next },
		func(context.Context) (string, bool) { return "", false },
		func(context.Context, string, string) error { return nil },
		zerolog.Nop(),
	)
}

func TestHandlerRoutesDoNotMountUnsignedProviderWebhooks(t *testing.T) {
	h := newTestHandler(nil, nil, nil).Routes()
	for _, path := range []string{"/webhooks/sendgrid", "/webhooks/ses"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}
}

func TestHandler_SendGridWebhook_BounceMarksLogAndSuppresses(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	attendeeID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, attendeeID)

	logID := uuid.Must(uuid.NewV7()).String()
	insertNotificationLog(t, ctx, db, logID, eventID, attendeeID, "sent", "delivered", "sg-msg-1")

	tracking := NewTrackingService(db, zerolog.Nop())
	fake := newFakeSuppression()
	h := newTestHandler(tracking, nil, fake)

	body := `[{"email":"bob@example.com","event":"bounce","type":"bounce","sg_message_id":"sg-msg-1","timestamp":1700000000}]`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sendgrid", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.handleSendGridWebhook(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Log row marked bounced.
	var status, bounceType string
	err := db.QueryRowContext(ctx, "SELECT delivery_status, bounce_type FROM notification_log WHERE id = ?", logID).Scan(&status, &bounceType)
	require.NoError(t, err)
	assert.Equal(t, "bounced", status)
	assert.Equal(t, "hard", bounceType)

	// Address suppressed with reason "bounce".
	assert.True(t, fake.suppressed["bob@example.com"])
	assert.Equal(t, "bounce", fake.suppressed2["bob@example.com"])
}

func TestHandler_SendGridWebhook_SpamReportSuppresses(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	attendeeID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, attendeeID)

	logID := uuid.Must(uuid.NewV7()).String()
	insertNotificationLog(t, ctx, db, logID, eventID, attendeeID, "sent", "delivered", "sg-msg-spam")

	tracking := NewTrackingService(db, zerolog.Nop())
	fake := newFakeSuppression()
	h := newTestHandler(tracking, nil, fake)

	body := `[{"email":"carol@example.com","event":"spamreport","sg_message_id":"sg-msg-spam","timestamp":1700000000}]`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sendgrid", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.handleSendGridWebhook(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var status string
	err := db.QueryRowContext(ctx, "SELECT delivery_status FROM notification_log WHERE id = ?", logID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "complained", status)
	assert.Equal(t, "complaint", fake.suppressed2["carol@example.com"])
}

func TestHandler_SendGridWebhook_InvalidPayload(t *testing.T) {
	db := testutil.NewTestDB(t)
	tracking := NewTrackingService(db, zerolog.Nop())
	h := newTestHandler(tracking, nil, newFakeSuppression())

	req := httptest.NewRequest(http.MethodPost, "/webhooks/sendgrid", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.handleSendGridWebhook(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_SESWebhook_PermanentBounceSuppresses(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	attendeeID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, attendeeID)

	logID := uuid.Must(uuid.NewV7()).String()
	insertNotificationLog(t, ctx, db, logID, eventID, attendeeID, "sent", "delivered", "ses-msg-1")

	tracking := NewTrackingService(db, zerolog.Nop())
	fake := newFakeSuppression()
	h := newTestHandler(tracking, nil, fake)

	// SES notification arrives wrapped in an SNS envelope; Message is a
	// JSON-encoded string.
	body := `{"Type":"Notification","Message":"{\"notificationType\":\"Bounce\",\"mail\":{\"messageId\":\"ses-msg-1\"},\"bounce\":{\"bounceType\":\"Permanent\",\"bouncedRecipients\":[{\"emailAddress\":\"dave@example.com\"}]}}"}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/ses", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.handleSESWebhook(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var status, bounceType string
	err := db.QueryRowContext(ctx, "SELECT delivery_status, bounce_type FROM notification_log WHERE id = ?", logID).Scan(&status, &bounceType)
	require.NoError(t, err)
	assert.Equal(t, "bounced", status)
	assert.Equal(t, "hard", bounceType)
	assert.Equal(t, "bounce", fake.suppressed2["dave@example.com"])
}

func TestHandler_Webhook_NilSuppressorDoesNotPanic(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	eventID := uuid.Must(uuid.NewV7()).String()
	attendeeID := uuid.Must(uuid.NewV7()).String()
	createParentRecordsForNotification(t, ctx, db, eventID, attendeeID)

	logID := uuid.Must(uuid.NewV7()).String()
	insertNotificationLog(t, ctx, db, logID, eventID, attendeeID, "sent", "delivered", "sg-nil")

	tracking := NewTrackingService(db, zerolog.Nop())
	h := newTestHandler(tracking, nil, nil) // nil suppressor

	body := `[{"email":"x@example.com","event":"bounce","type":"bounce","sg_message_id":"sg-nil","timestamp":1700000000}]`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sendgrid", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.handleSendGridWebhook(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Delivery event still recorded even without a suppressor.
	var status string
	err := db.QueryRowContext(ctx, "SELECT delivery_status FROM notification_log WHERE id = ?", logID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "bounced", status)
}

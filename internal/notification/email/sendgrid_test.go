package email

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xxi0xx/owl-invites/internal/notification"
)

func TestSendGridProvider_Send_RequestConstruction(t *testing.T) {
	var (
		gotPath   string
		gotAuth   string
		gotCType  string
		gotMethod string
		gotBody   sendGridRequest
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotCType = r.Header.Get("Content-Type")
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("X-Message-Id", "msg-123")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	p := NewSendGridProvider("SG.testkey.value", "sender@example.com")
	p.baseURL = srv.URL + "/v3/mail/send"

	res, err := p.Send(context.Background(), &notification.Message{
		To:      "rcpt@example.com",
		Subject: "Party Invite",
		Plain:   "plain version",
		Body:    "<html><body>rich</body></html>",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.MessageID != "msg-123" {
		t.Fatalf("MessageID = %q, want msg-123", res.MessageID)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotPath != "/v3/mail/send" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer SG.testkey.value" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotCType != "application/json" {
		t.Fatalf("content-type = %q", gotCType)
	}

	// Body assertions.
	if gotBody.From.Email != "sender@example.com" {
		t.Fatalf("from = %q", gotBody.From.Email)
	}
	if gotBody.Subject != "Party Invite" {
		t.Fatalf("subject = %q", gotBody.Subject)
	}
	if len(gotBody.Personalizations) != 1 || len(gotBody.Personalizations[0].To) != 1 ||
		gotBody.Personalizations[0].To[0].Email != "rcpt@example.com" {
		t.Fatalf("personalizations = %+v", gotBody.Personalizations)
	}
	// Both text/plain and text/html content blocks.
	var hasPlain, hasHTML bool
	for _, c := range gotBody.Content {
		if c.Type == "text/plain" && c.Value == "plain version" {
			hasPlain = true
		}
		if c.Type == "text/html" && c.Value == "<html><body>rich</body></html>" {
			hasHTML = true
		}
	}
	if !hasPlain || !hasHTML {
		t.Fatalf("content blocks missing plain/html: %+v", gotBody.Content)
	}
}

func TestSendGridProvider_Send_WithAttachment(t *testing.T) {
	var gotBody sendGridRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	p := NewSendGridProvider("SG.key.value", "s@x.com")
	p.baseURL = srv.URL

	_, err := p.Send(context.Background(), &notification.Message{
		To:   "r@x.com",
		Body: "<p>x</p>",
		Attachments: []notification.Attachment{
			{Filename: "a.ics", ContentType: "text/calendar", Data: []byte("DATA")},
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(gotBody.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(gotBody.Attachments))
	}
	a := gotBody.Attachments[0]
	if a.Filename != "a.ics" || a.Type != "text/calendar" || a.Disposition != "attachment" {
		t.Fatalf("attachment fields wrong: %+v", a)
	}
	if a.Content != "REFUQQ==" { // base64("DATA")
		t.Fatalf("attachment content = %q", a.Content)
	}
}

func TestSendGridProvider_Send_Non2xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"message":"bad key"}]}`))
	}))
	defer srv.Close()

	p := NewSendGridProvider("SG.key.value", "s@x.com")
	p.baseURL = srv.URL
	_, err := p.Send(context.Background(), &notification.Message{To: "r@x", Body: "x"})
	if err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestSendGridProvider_NameChannel(t *testing.T) {
	p := NewSendGridProvider("SG.key.value", "s@x")
	if p.Name() != "sendgrid" {
		t.Fatalf("Name = %q", p.Name())
	}
	if p.Channel() != notification.ChannelEmail {
		t.Fatalf("Channel = %q", p.Channel())
	}
}

func TestSendGridProvider_HealthCheck(t *testing.T) {
	if err := NewSendGridProvider("", "s@x").HealthCheck(context.Background()); err == nil {
		t.Fatal("empty key should fail")
	}
	if err := NewSendGridProvider("bad", "s@x").HealthCheck(context.Background()); err == nil {
		t.Fatal("non-SG prefix should fail")
	}
	if err := NewSendGridProvider("SG.short", "s@x").HealthCheck(context.Background()); err == nil {
		t.Fatal("short key should fail")
	}
	if err := NewSendGridProvider("SG.aaaaaaaaaaaaaaaaaaaa", "s@x").HealthCheck(context.Background()); err != nil {
		t.Fatalf("valid key should pass: %v", err)
	}
}

func TestSendGridProvider_SendBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	p := NewSendGridProvider("SG.key.value", "s@x")
	p.baseURL = srv.URL
	results, errs := p.SendBatch(context.Background(), []*notification.Message{
		{To: "a@x", Body: "1"},
		{To: "b@x", Body: "2"},
	})
	if len(results) != 2 || len(errs) != 2 {
		t.Fatalf("expected 2 results/errs, got %d/%d", len(results), len(errs))
	}
	for i, e := range errs {
		if e != nil {
			t.Fatalf("errs[%d] = %v", i, e)
		}
	}
}

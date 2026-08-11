package sms

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xxi0xx/owl-invites/internal/notification"
)

func TestVonageProvider_Send_RequestConstruction(t *testing.T) {
	var (
		gotPath   string
		gotMethod string
		gotCType  string
		gotBody   vonageRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotCType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message-count":"1","messages":[{"status":"0","message-id":"MID1"}]}`))
	}))
	defer srv.Close()

	p := NewVonageProvider("keyabc", "secretxyz", "Owl Invites")
	p.baseURL = srv.URL + "/sms/json"

	res, err := p.Send(context.Background(), &notification.Message{
		To:   "447700900000",
		Body: "hello",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.MessageID != "MID1" {
		t.Fatalf("MessageID = %q", res.MessageID)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotPath != "/sms/json" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotCType != "application/json" {
		t.Fatalf("content-type = %q", gotCType)
	}
	if gotBody.APIKey != "keyabc" || gotBody.APISecret != "secretxyz" {
		t.Fatalf("creds in body = %q/%q", gotBody.APIKey, gotBody.APISecret)
	}
	if gotBody.To != "447700900000" || gotBody.From != "Owl Invites" || gotBody.Text != "hello" {
		t.Fatalf("body = %+v", gotBody)
	}
}

func TestVonageProvider_Send_MessageLevelError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message-count":"1","messages":[{"status":"2","error-text":"Missing api_key"}]}`))
	}))
	defer srv.Close()
	p := NewVonageProvider("k", "s", "F")
	p.baseURL = srv.URL
	_, err := p.Send(context.Background(), &notification.Message{To: "1", Body: "x"})
	if err == nil {
		t.Fatal("expected message-level error")
	}
	if !strings.Contains(err.Error(), "Missing api_key") {
		t.Fatalf("error should include error-text: %v", err)
	}
}

func TestVonageProvider_Send_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()
	p := NewVonageProvider("k", "s", "F")
	p.baseURL = srv.URL
	_, err := p.Send(context.Background(), &notification.Message{To: "1", Body: "x"})
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestVonageProvider_Send_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()
	p := NewVonageProvider("k", "s", "F")
	p.baseURL = srv.URL
	_, err := p.Send(context.Background(), &notification.Message{To: "1", Body: "x"})
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestVonageProvider_NameChannel(t *testing.T) {
	p := NewVonageProvider("k", "s", "F")
	if p.Name() != "vonage" {
		t.Fatalf("Name = %q", p.Name())
	}
	if p.Channel() != notification.ChannelSMS {
		t.Fatalf("Channel = %q", p.Channel())
	}
}

func TestVonageProvider_HealthCheck(t *testing.T) {
	if err := NewVonageProvider("", "s", "F").HealthCheck(context.Background()); err == nil {
		t.Fatal("empty key should fail")
	}
	if err := NewVonageProvider("k", "", "F").HealthCheck(context.Background()); err == nil {
		t.Fatal("empty secret should fail")
	}
	if err := NewVonageProvider("k", "s", "F").HealthCheck(context.Background()); err != nil {
		t.Fatalf("valid should pass: %v", err)
	}
}

func TestVonageProvider_SendBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messages":[{"status":"0","message-id":"M"}]}`))
	}))
	defer srv.Close()
	p := NewVonageProvider("k", "s", "F")
	p.baseURL = srv.URL
	results, errs := p.SendBatch(context.Background(), []*notification.Message{
		{To: "1", Body: "a"}, {To: "2", Body: "b"},
	})
	if len(results) != 2 || len(errs) != 2 {
		t.Fatalf("got %d/%d", len(results), len(errs))
	}
}

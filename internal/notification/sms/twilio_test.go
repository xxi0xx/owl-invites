package sms

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/xxi0xx/owl-invites/internal/notification"
)

func TestTwilioProvider_Send_RequestConstruction(t *testing.T) {
	var (
		gotPath   string
		gotMethod string
		gotUser   string
		gotPass   string
		gotCType  string
		gotForm   url.Values
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotUser, gotPass, _ = r.BasicAuth()
		gotCType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(b))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sid":"SM123"}`))
	}))
	defer srv.Close()

	p := NewTwilioProvider("ACaccount", "secrettoken", "+15550000000")
	p.baseURL = srv.URL

	res, err := p.Send(context.Background(), &notification.Message{
		To:   "+15551234567",
		Body: "Your event is tomorrow",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.MessageID != "SM123" {
		t.Fatalf("MessageID = %q", res.MessageID)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotPath != "/2010-04-01/Accounts/ACaccount/Messages.json" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotUser != "ACaccount" || gotPass != "secrettoken" {
		t.Fatalf("basic auth = %q:%q", gotUser, gotPass)
	}
	if gotCType != "application/x-www-form-urlencoded" {
		t.Fatalf("content-type = %q", gotCType)
	}
	if gotForm.Get("To") != "+15551234567" {
		t.Fatalf("To = %q", gotForm.Get("To"))
	}
	if gotForm.Get("From") != "+15550000000" {
		t.Fatalf("From = %q", gotForm.Get("From"))
	}
	if gotForm.Get("Body") != "Your event is tomorrow" {
		t.Fatalf("Body = %q", gotForm.Get("Body"))
	}
}

func TestTwilioProvider_Send_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":21211,"message":"invalid To"}`))
	}))
	defer srv.Close()

	p := NewTwilioProvider("AC", "tok", "+1")
	p.baseURL = srv.URL
	_, err := p.Send(context.Background(), &notification.Message{To: "bad", Body: "x"})
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if !strings.Contains(err.Error(), "invalid To") {
		t.Fatalf("error should include body: %v", err)
	}
}

func TestTwilioProvider_Send_SuccessNoSID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()
	p := NewTwilioProvider("AC", "tok", "+1")
	p.baseURL = srv.URL
	res, err := p.Send(context.Background(), &notification.Message{To: "+1", Body: "x"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.MessageID != "" {
		t.Fatalf("expected empty MessageID, got %q", res.MessageID)
	}
}

func TestTwilioProvider_NameChannel(t *testing.T) {
	p := NewTwilioProvider("AC", "tok", "+1")
	if p.Name() != "twilio" {
		t.Fatalf("Name = %q", p.Name())
	}
	if p.Channel() != notification.ChannelSMS {
		t.Fatalf("Channel = %q", p.Channel())
	}
}

func TestTwilioProvider_HealthCheck(t *testing.T) {
	if err := NewTwilioProvider("", "", "+1").HealthCheck(context.Background()); err == nil {
		t.Fatal("empty creds should fail")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2010-04-01/Accounts/AC.json" {
			t.Errorf("health path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	p := NewTwilioProvider("AC", "tok", "+1")
	p.baseURL = srv.URL
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer bad.Close()
	pb := NewTwilioProvider("AC", "tok", "+1")
	pb.baseURL = bad.URL
	if err := pb.HealthCheck(context.Background()); err == nil {
		t.Fatal("403 health should fail")
	}
}

func TestTwilioProvider_SendBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sid":"SM"}`))
	}))
	defer srv.Close()
	p := NewTwilioProvider("AC", "tok", "+1")
	p.baseURL = srv.URL
	results, errs := p.SendBatch(context.Background(), []*notification.Message{
		{To: "+1", Body: "a"}, {To: "+2", Body: "b"},
	})
	if len(results) != 2 || len(errs) != 2 {
		t.Fatalf("got %d/%d", len(results), len(errs))
	}
}

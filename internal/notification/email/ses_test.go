package email

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/xxi0xx/owl-invites/internal/notification"
)

func TestSESProvider_NameChannelAndHost(t *testing.T) {
	p := NewSESProvider("us-east-1", "user", "pass", "from@example.com")
	if p.Name() != "ses" {
		t.Fatalf("Name = %q", p.Name())
	}
	if p.Channel() != notification.ChannelEmail {
		t.Fatalf("Channel = %q", p.Channel())
	}
	if p.smtp.host != "email-smtp.us-east-1.amazonaws.com" {
		t.Fatalf("host = %q", p.smtp.host)
	}
	if p.smtp.port != "587" {
		t.Fatalf("port = %q", p.smtp.port)
	}
}

func TestSESProvider_Send_ReachesSMTP(t *testing.T) {
	srv := newCaptureSMTP(t)
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.addr)

	p := NewSESProvider("us-east-1", "", "", "from@example.com")
	// Redirect the wrapped SMTP provider at the capture server.
	p.smtp.host = host
	p.smtp.port = port

	if _, err := p.Send(context.Background(), &notification.Message{
		To:      "to@example.com",
		Subject: "Subj",
		Plain:   "p",
		Body:    "<html><body>b</body></html>",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	data := srv.Data()
	if !strings.Contains(data, "multipart/alternative") {
		t.Fatalf("SES send did not produce MIME body:\n%s", data)
	}
}

func TestSESProvider_HealthCheck(t *testing.T) {
	srv := newCaptureSMTP(t)
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.addr)

	p := NewSESProvider("us-east-1", "", "", "from@example.com")
	p.smtp.host = host
	p.smtp.port = port
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestSESProvider_SendBatch(t *testing.T) {
	srv := newCaptureSMTP(t)
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.addr)

	p := NewSESProvider("eu-west-1", "", "", "from@example.com")
	p.smtp.host = host
	p.smtp.port = port

	results, errs := p.SendBatch(context.Background(), []*notification.Message{
		{To: "a@example.com", Body: "x"},
	})
	if len(results) != 1 || len(errs) != 1 {
		t.Fatalf("expected 1 result/err, got %d/%d", len(results), len(errs))
	}
}

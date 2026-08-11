package email

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/xxi0xx/owl-invites/internal/notification"
)

// --- stripCRLF: header-injection defense ------------------------------------

func TestStripCRLF(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"clean", "Hello World", "Hello World"},
		{"empty", "", ""},
		{"bcc injection crlf", "subject\r\nBcc: evil@x", "subjectBcc: evil@x"},
		{"bcc injection lf only", "subject\nBcc: evil@x", "subjectBcc: evil@x"},
		{"bcc injection cr only", "subject\rBcc: evil@x", "subjectBcc: evil@x"},
		{"multiple headers", "x\r\nTo: a@b\r\nCc: c@d", "xTo: a@bCc: c@d"},
		{"trailing crlf", "value\r\n", "value"},
		{"leading crlf", "\r\nInjected: yes", "Injected: yes"},
		{"only crlf", "\r\n", ""},
		{"smuggled body", "Subject\r\n\r\n<script>alert(1)</script>", "Subject<script>alert(1)</script>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripCRLF(tc.in)
			if got != tc.want {
				t.Fatalf("stripCRLF(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Fatalf("stripCRLF(%q) still contains CR/LF: %q", tc.in, got)
			}
		})
	}
}

// --- MIME assembly ----------------------------------------------------------

func TestWriteAlternativeParts_TextAndHTML(t *testing.T) {
	var buf bytes.Buffer
	msg := &notification.Message{
		Plain: "plain body text",
		Body:  "<html><body>html body</body></html>",
	}
	writeAlternativeParts(&buf, msg, "BND")
	out := buf.String()

	if c := strings.Count(out, "--BND\r\n"); c != 2 {
		t.Fatalf("expected 2 part boundaries, got %d in:\n%s", c, out)
	}
	if !strings.Contains(out, "Content-Type: text/plain; charset=\"utf-8\"") {
		t.Fatalf("missing text/plain part:\n%s", out)
	}
	if !strings.Contains(out, "Content-Type: text/html; charset=\"utf-8\"") {
		t.Fatalf("missing text/html part:\n%s", out)
	}
	if !strings.Contains(out, "Content-Transfer-Encoding: quoted-printable") {
		t.Fatalf("missing quoted-printable encoding:\n%s", out)
	}
	if !strings.Contains(out, "plain body text") {
		t.Fatalf("plain content missing:\n%s", out)
	}
	if !strings.Contains(out, "html body") {
		t.Fatalf("html content missing:\n%s", out)
	}
}

func TestWriteAlternativeParts_PlainFallsBackToBody(t *testing.T) {
	var buf bytes.Buffer
	// Plain empty -> falls back to Body. Since Plain==Body after fallback,
	// only one part (text/plain) is written.
	msg := &notification.Message{Body: "only body"}
	writeAlternativeParts(&buf, msg, "BND")
	out := buf.String()

	if !strings.Contains(out, "Content-Type: text/plain") {
		t.Fatalf("expected text/plain part:\n%s", out)
	}
	if strings.Contains(out, "text/html") {
		t.Fatalf("should not emit html part when plain==body:\n%s", out)
	}
	if !strings.Contains(out, "only body") {
		t.Fatalf("body content missing:\n%s", out)
	}
}

// --- Full Send via in-process SMTP capture server ---------------------------

func TestSMTPProvider_Send_MIMEAndHeaderInjection(t *testing.T) {
	srv := newCaptureSMTP(t)
	defer srv.Close()

	host, port, err := net.SplitHostPort(srv.addr)
	if err != nil {
		t.Fatal(err)
	}

	p := NewSMTPProvider(host, port, "", "", "from@example.com")
	msg := &notification.Message{
		// Clean envelope recipient (the raw envelope value is validated by
		// net/smtp itself — see TestSMTPProvider_Send_RejectsCRLFRecipient).
		// Injection payloads live in the header values that flow through the
		// buffer assembly: To-header, Subject, and From.
		To:      "victim@example.com",
		Subject: "Hello\r\nBcc: evil2@x.com",
		Plain:   "plain text",
		Body:    "<html><body>rich</body></html>",
	}

	if _, err := p.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	data := srv.Data()

	// Header injection defense: the CRLF must be stripped so the injected
	// "Bcc:" is flattened into the Subject value and never becomes its own
	// header line. Assert no line *starts* with "Bcc:".
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "Bcc:") {
			t.Fatalf("header injection succeeded; standalone Bcc header line:\n%s", data)
		}
	}
	// The injected text must survive only as part of the (safe) Subject value.
	if !strings.Contains(data, "Subject: HelloBcc: evil2@x.com") {
		t.Fatalf("expected injected CRLF stripped into Subject value:\n%s", data)
	}

	// Exactly one To, one Subject, one From header line.
	headerSection := data
	if idx := strings.Index(data, "\r\n\r\n"); idx != -1 {
		headerSection = data[:idx]
	}
	if c := strings.Count(headerSection, "\r\nTo: ") + boolToInt(strings.HasPrefix(headerSection, "To: ")); c != 1 {
		t.Fatalf("expected exactly 1 To header, found %d:\n%s", c, headerSection)
	}

	// MIME structure present.
	if !strings.Contains(data, "MIME-Version: 1.0") {
		t.Fatalf("missing MIME-Version:\n%s", data)
	}
	if !strings.Contains(data, "Content-Type: multipart/alternative") {
		t.Fatalf("missing multipart/alternative:\n%s", data)
	}
	if !strings.Contains(data, "text/plain") || !strings.Contains(data, "text/html") {
		t.Fatalf("missing alternative parts:\n%s", data)
	}
}

// A CR/LF in the envelope recipient must be rejected outright (net/smtp
// refuses to transmit it), not smuggled onto the wire.
func TestSMTPProvider_Send_RejectsCRLFRecipient(t *testing.T) {
	srv := newCaptureSMTP(t)
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.addr)

	p := NewSMTPProvider(host, port, "", "", "from@example.com")
	_, err := p.Send(context.Background(), &notification.Message{
		To:   "victim@example.com\r\nRCPT TO:<evil@x.com>",
		Body: "x",
	})
	if err == nil {
		t.Fatal("expected CRLF recipient to be rejected")
	}
	if !strings.Contains(err.Error(), "CR or LF") && !strings.Contains(err.Error(), "smtp send") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSMTPProvider_Send_WithAttachment(t *testing.T) {
	srv := newCaptureSMTP(t)
	defer srv.Close()

	host, port, _ := net.SplitHostPort(srv.addr)
	p := NewSMTPProvider(host, port, "", "", "from@example.com")
	msg := &notification.Message{
		To:      "to@example.com",
		Subject: "Subj",
		Plain:   "p",
		Body:    "<html><body>b</body></html>",
		Attachments: []notification.Attachment{
			{Filename: "cal.ics", ContentType: "text/calendar", Data: []byte("BEGIN:VCALENDAR")},
		},
	}
	if _, err := p.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	data := srv.Data()
	if !strings.Contains(data, "multipart/mixed") {
		t.Fatalf("expected multipart/mixed with attachment:\n%s", data)
	}
	if !strings.Contains(data, "Content-Disposition: attachment; filename=\"cal.ics\"") {
		t.Fatalf("missing attachment disposition:\n%s", data)
	}
	if !strings.Contains(data, "Content-Transfer-Encoding: base64") {
		t.Fatalf("missing base64 encoding for attachment:\n%s", data)
	}
}

func TestSMTPProvider_NameChannel(t *testing.T) {
	p := NewSMTPProvider("h", "25", "", "", "f@x")
	if p.Name() != "smtp" {
		t.Fatalf("Name = %q", p.Name())
	}
	if p.Channel() != notification.ChannelEmail {
		t.Fatalf("Channel = %q", p.Channel())
	}
}

func TestSMTPProvider_HealthCheck(t *testing.T) {
	srv := newCaptureSMTP(t)
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.addr)

	p := NewSMTPProvider(host, port, "", "", "f@x")
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestSMTPProvider_HealthCheck_DialError(t *testing.T) {
	p := NewSMTPProvider("127.0.0.1", "1", "", "", "f@x")
	if err := p.HealthCheck(context.Background()); err == nil {
		t.Fatal("expected health check dial error")
	}
}

func TestSMTPProvider_Send_DialError(t *testing.T) {
	// Nothing listening on this port.
	p := NewSMTPProvider("127.0.0.1", "1", "", "", "f@x")
	_, err := p.Send(context.Background(), &notification.Message{To: "a@b", Body: "x"})
	if err == nil {
		t.Fatal("expected error dialing dead SMTP port")
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// captureSMTP is a minimal SMTP server that accepts one message and records
// the DATA payload, sufficient to exercise net/smtp.SendMail.
type captureSMTP struct {
	ln   net.Listener
	addr string
	mu   sync.Mutex
	data string
	done chan struct{}
}

func newCaptureSMTP(t *testing.T) *captureSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &captureSMTP{ln: ln, addr: ln.Addr().String(), done: make(chan struct{})}
	go s.serve()
	return s
}

func (s *captureSMTP) Close() { _ = s.ln.Close() }

func (s *captureSMTP) Data() string {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data
}

func (s *captureSMTP) serve() {
	conn, err := s.ln.Accept()
	if err != nil {
		close(s.done)
		return
	}
	defer func() { _ = conn.Close() }()

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	write := func(s string) { _, _ = w.WriteString(s); _ = w.Flush() }
	write("220 capture ESMTP\r\n")

	var body bytes.Buffer
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			break
		}
		if inData {
			if line == ".\r\n" {
				inData = false
				write("250 OK\r\n")
				continue
			}
			body.WriteString(line)
			continue
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			write("250-capture\r\n250 OK\r\n")
		case strings.HasPrefix(cmd, "MAIL FROM"):
			write("250 OK\r\n")
		case strings.HasPrefix(cmd, "RCPT TO"):
			write("250 OK\r\n")
		case cmd == "DATA":
			write("354 End data with <CR><LF>.<CR><LF>\r\n")
			inData = true
		case cmd == "QUIT":
			write("221 Bye\r\n")
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()
			close(s.done)
			return
		default:
			write("250 OK\r\n")
		}
	}
	s.mu.Lock()
	s.data = body.String()
	s.mu.Unlock()
	close(s.done)
}

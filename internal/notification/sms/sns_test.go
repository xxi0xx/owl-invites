package sms

import (
	"testing"

	"github.com/xxi0xx/owl-invites/internal/notification"
)

func TestSNSProvider_ConstructAndIdentity(t *testing.T) {
	p, err := NewSNSProvider("us-east-1", "AKIAEXAMPLE", "secretexample")
	if err != nil {
		t.Fatalf("NewSNSProvider: %v", err)
	}
	if p.Name() != "sns" {
		t.Fatalf("Name = %q", p.Name())
	}
	if p.Channel() != notification.ChannelSMS {
		t.Fatalf("Channel = %q", p.Channel())
	}
	if p.snsClient == nil {
		t.Fatal("snsClient should be initialized")
	}
}

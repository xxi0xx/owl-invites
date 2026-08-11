package suppression

import (
	"context"
	"testing"

	"github.com/xxi0xx/owl-invites/internal/database"
	"github.com/xxi0xx/owl-invites/internal/testutil"
)

func newService(t *testing.T) (*Service, database.DB) {
	t.Helper()
	db := testutil.NewTestDB(t)
	return NewService(NewStore(db)), db
}

func TestService_IsSuppressed_GlobalVsEvent(t *testing.T) {
	svc, db := newService(t)
	ctx := context.Background()
	evt1 := seedEvent(t, ctx, db)
	evt2 := seedEvent(t, ctx, db)

	// Global suppression wins for every scope.
	if err := svc.Suppress(ctx, "Global@Example.com", "", ReasonUnsubscribe); err != nil {
		t.Fatalf("suppress global: %v", err)
	}
	if !svc.IsSuppressed(ctx, "global@example.com", "") {
		t.Fatal("expected global suppression (case-insensitive)")
	}
	if !svc.IsSuppressed(ctx, "global@example.com", evt1) {
		t.Fatal("expected global suppression to apply to event scope")
	}

	// Event-scoped suppression does not leak across events or to global.
	if err := svc.Suppress(ctx, "scoped@example.com", evt1, ReasonUnsubscribe); err != nil {
		t.Fatalf("suppress event: %v", err)
	}
	if !svc.IsSuppressed(ctx, "scoped@example.com", evt1) {
		t.Fatal("expected event suppression for evt1")
	}
	if svc.IsSuppressed(ctx, "scoped@example.com", evt2) {
		t.Fatal("event suppression must not leak to evt2")
	}
	if svc.IsSuppressed(ctx, "scoped@example.com", "") {
		t.Fatal("event suppression must not imply global")
	}
}

func TestService_Suppress_InvalidReason(t *testing.T) {
	svc, _ := newService(t)
	if err := svc.Suppress(context.Background(), "a@example.com", "", "spam"); err == nil {
		t.Fatal("expected error for invalid reason")
	}
}

func TestService_UnsubscribeToken_RoundTrip(t *testing.T) {
	svc, db := newService(t)
	ctx := context.Background()
	evt7 := seedEvent(t, ctx, db)

	token, err := svc.GenerateUnsubscribeToken(ctx, "User@Example.com", evt7)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	email, eventID, ok, err := svc.VerifyUnsubscribeToken(ctx, token)
	if err != nil || !ok {
		t.Fatalf("verify token: ok=%v err=%v", ok, err)
	}
	if email != "user@example.com" {
		t.Fatalf("expected normalized email, got %q", email)
	}
	if eventID == nil || *eventID != evt7 {
		t.Fatalf("expected eventID evt7, got %v", eventID)
	}

	// Wrong token does not verify.
	if _, _, ok, _ := svc.VerifyUnsubscribeToken(ctx, "deadbeef"); ok {
		t.Fatal("expected verification to fail for unknown token")
	}
	// Empty token does not verify.
	if _, _, ok, _ := svc.VerifyUnsubscribeToken(ctx, ""); ok {
		t.Fatal("expected verification to fail for empty token")
	}
}

func TestService_GlobalToken_RoundTrip(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	token, err := svc.GenerateUnsubscribeToken(ctx, "g@example.com", "")
	if err != nil {
		t.Fatalf("generate global token: %v", err)
	}

	email, eventID, ok, err := svc.VerifyUnsubscribeToken(ctx, token)
	if err != nil || !ok {
		t.Fatalf("verify global token: ok=%v err=%v", ok, err)
	}
	if email != "g@example.com" || eventID != nil {
		t.Fatalf("expected global token, got email=%q eventID=%v", email, eventID)
	}
}

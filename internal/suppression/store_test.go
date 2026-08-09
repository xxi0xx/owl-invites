package suppression

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/testutil"
)

func ptr(s string) *string { return &s }

// seedEvent inserts an organizer + event so suppressions/tokens can satisfy the
// event_id foreign key. It returns the new event id.
func seedEvent(t *testing.T, ctx context.Context, db database.DB) string {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	orgID := uuid.Must(uuid.NewV7()).String()
	eventID := uuid.Must(uuid.NewV7()).String()

	testutil.SeedUser(t, db, orgID, "test-"+orgID+"@example.com", "Test Organizer")
	if _, err := db.ExecContext(ctx,
		`INSERT INTO events (id, title, event_date, status, share_token, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		eventID, "Test Event", "2026-06-15T14:00:00Z", "published", "share-"+eventID, now, now); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	testutil.SeedEventOwner(t, db, eventID, orgID)
	return eventID
}

func TestStore_GlobalSuppression(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	if err := store.Suppress(ctx, "a@example.com", nil, ReasonUnsubscribe); err != nil {
		t.Fatalf("suppress global: %v", err)
	}

	// Global suppression applies to any event scope.
	for _, eid := range []*string{nil, ptr("evt-1"), ptr("evt-2")} {
		got, err := store.IsSuppressed(ctx, "a@example.com", eid)
		if err != nil {
			t.Fatalf("is suppressed: %v", err)
		}
		if !got {
			t.Fatalf("expected global suppression to apply for event %v", eid)
		}
	}

	// A different email is not suppressed.
	got, err := store.IsSuppressed(ctx, "b@example.com", nil)
	if err != nil {
		t.Fatalf("is suppressed: %v", err)
	}
	if got {
		t.Fatal("expected b@example.com not suppressed")
	}
}

func TestStore_EventScopedSuppression(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	evt1 := seedEvent(t, ctx, db)
	evt2 := seedEvent(t, ctx, db)

	if err := store.Suppress(ctx, "a@example.com", ptr(evt1), ReasonUnsubscribe); err != nil {
		t.Fatalf("suppress event: %v", err)
	}

	// Suppressed for evt1.
	got, err := store.IsSuppressed(ctx, "a@example.com", ptr(evt1))
	if err != nil {
		t.Fatalf("is suppressed: %v", err)
	}
	if !got {
		t.Fatal("expected suppression for evt1")
	}

	// NOT suppressed for a different event.
	got, err = store.IsSuppressed(ctx, "a@example.com", ptr(evt2))
	if err != nil {
		t.Fatalf("is suppressed: %v", err)
	}
	if got {
		t.Fatal("event-scoped suppression must not leak to other events")
	}

	// NOT globally suppressed.
	got, err = store.IsSuppressed(ctx, "a@example.com", nil)
	if err != nil {
		t.Fatalf("is suppressed: %v", err)
	}
	if got {
		t.Fatal("event-scoped suppression must not imply global suppression")
	}
}

func TestStore_SuppressIdempotent_Global(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := store.Suppress(ctx, "a@example.com", nil, ReasonUnsubscribe); err != nil {
			t.Fatalf("suppress iteration %d: %v", i, err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_suppressions WHERE email = ? AND event_id IS NULL`,
		"a@example.com",
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 global suppression row, got %d", count)
	}
}

func TestStore_SuppressIdempotent_Event(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	evt1 := seedEvent(t, ctx, db)

	for i := 0; i < 3; i++ {
		if err := store.Suppress(ctx, "a@example.com", ptr(evt1), ReasonBounce); err != nil {
			t.Fatalf("suppress iteration %d: %v", i, err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_suppressions WHERE email = ? AND event_id = ?`,
		"a@example.com", evt1,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 event suppression row, got %d", count)
	}
}

func TestStore_TokenRoundTrip(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	evt9 := seedEvent(t, ctx, db)

	if err := store.CreateToken(ctx, "hash-global", "a@example.com", nil); err != nil {
		t.Fatalf("create global token: %v", err)
	}
	if err := store.CreateToken(ctx, "hash-evt", "b@example.com", ptr(evt9)); err != nil {
		t.Fatalf("create event token: %v", err)
	}

	email, eventID, ok, err := store.FindTokenByHash(ctx, "hash-global")
	if err != nil || !ok {
		t.Fatalf("find global token: ok=%v err=%v", ok, err)
	}
	if email != "a@example.com" || eventID != nil {
		t.Fatalf("global token mismatch: email=%q eventID=%v", email, eventID)
	}

	email, eventID, ok, err = store.FindTokenByHash(ctx, "hash-evt")
	if err != nil || !ok {
		t.Fatalf("find event token: ok=%v err=%v", ok, err)
	}
	if email != "b@example.com" || eventID == nil || *eventID != evt9 {
		t.Fatalf("event token mismatch: email=%q eventID=%v", email, eventID)
	}

	_, _, ok, err = store.FindTokenByHash(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("find unknown token: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for unknown token hash")
	}
}

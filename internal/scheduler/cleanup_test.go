package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/database"
	"github.com/xxi0xx/owl-invites/internal/testutil"
)

func seedCleanupEvent(t *testing.T, db database.DB, userID, email, eventID, title, eventDate, shareToken string) {
	t.Helper()
	testutil.SeedUser(t, db, userID, email, "Test Owner")
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO events (id, title, event_date, retention_days, status, created_at, updated_at)
		 VALUES (?, ?, ?, 30, 'published', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		eventID, title, eventDate)
	require.NoError(t, err)
	testutil.SeedEventOwner(t, db, eventID, userID)
}

func TestCleanupJobWarnExpiringNotifiesOrganizer(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zerolog.Nop()

	orgID := "org-cleanup-test-1"
	orgEmail := "organizer@test.com"
	// Create an event that expires within 7 days (event_date 25 days ago, retention 30 days → expires in 5 days).
	eventID := "evt-cleanup-test-1"
	eventDate := time.Now().UTC().AddDate(0, 0, -25).Format(time.RFC3339)
	seedCleanupEvent(t, db, orgID, orgEmail, eventID, "Expiring Party", eventDate, "abc12345")

	// Track notification calls.
	var mu sync.Mutex
	var notifiedEmail, notifiedTitle string
	var notifiedExpiresAt time.Time

	job := NewCleanupJob(db, logger)
	job.SetRetentionNotify(func(ctx context.Context, email, title string, expiresAt time.Time) {
		mu.Lock()
		defer mu.Unlock()
		notifiedEmail = email
		notifiedTitle = title
		notifiedExpiresAt = expiresAt
	})

	err := job.Run(context.Background())
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, orgEmail, notifiedEmail, "organizer should be notified")
	assert.Equal(t, "Expiring Party", notifiedTitle)
	assert.False(t, notifiedExpiresAt.IsZero(), "expiry time should be set")

	// Verify event status remains published (warnExpiring no longer changes status).
	var status string
	err = db.QueryRowContext(context.Background(),
		`SELECT status FROM events WHERE id = ?`, eventID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "published", status)
}

func TestCleanupJobWarnExpiringSkipsAlreadyWarned(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zerolog.Nop()

	orgID := "org-cleanup-test-2"
	// Create an event within the warning window.
	eventDate := time.Now().UTC().AddDate(0, 0, -25).Format(time.RFC3339)
	seedCleanupEvent(t, db, orgID, "org2@test.com", "evt-already-warned", "Already Warned", eventDate, "xyz12345")

	notifyCount := 0
	job := NewCleanupJob(db, logger)
	job.SetRetentionNotify(func(ctx context.Context, email, title string, expiresAt time.Time) {
		notifyCount++
	})

	// First run should warn.
	err := job.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, notifyCount, "first run should trigger notification")

	// Second run on same job instance should skip (in-memory dedup).
	err = job.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, notifyCount, "second run should not trigger notification again")
}

func TestCleanupJobWarnExpiringSkipsNotYetExpiring(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zerolog.Nop()

	orgID := "org-cleanup-test-3"
	// Create an event that expires in 20 days (well beyond 7-day warning threshold).
	eventDate := time.Now().UTC().AddDate(0, 0, -10).Format(time.RFC3339)
	seedCleanupEvent(t, db, orgID, "org3@test.com", "evt-not-expiring", "Far Future", eventDate, "far12345")

	notifyCalled := false
	job := NewCleanupJob(db, logger)
	job.SetRetentionNotify(func(ctx context.Context, email, title string, expiresAt time.Time) {
		notifyCalled = true
	})

	err := job.Run(context.Background())
	require.NoError(t, err)
	assert.False(t, notifyCalled, "event far from expiry should not trigger notification")
}

func TestCleanupJobDeleteExpired(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zerolog.Nop()

	orgID := "org-cleanup-test-4"
	// Create an event that has already expired (event_date 40 days ago, retention 30 days).
	eventDate := time.Now().UTC().AddDate(0, 0, -40).Format(time.RFC3339)
	seedCleanupEvent(t, db, orgID, "org4@test.com", "evt-expired", "Expired Party", eventDate, "exp12345")

	job := NewCleanupJob(db, logger)
	err := job.Run(context.Background())
	require.NoError(t, err)

	// Verify event is deleted.
	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM events WHERE id = 'evt-expired'`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "expired event should be deleted")
}

func TestCleanupJobNoNotifyWithoutCallback(t *testing.T) {
	db := testutil.NewTestDB(t)
	logger := zerolog.Nop()

	orgID := "org-cleanup-test-5"
	// Create an event within the warning window.
	eventDate := time.Now().UTC().AddDate(0, 0, -25).Format(time.RFC3339)
	seedCleanupEvent(t, db, orgID, "org5@test.com", "evt-no-callback", "No Callback", eventDate, "ncb12345")

	job := NewCleanupJob(db, logger)
	// No SetRetentionNotify called — should not panic.
	err := job.Run(context.Background())
	require.NoError(t, err, "should not panic without notification callback")
}

package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/database"
	"github.com/xxi0xx/owl-invites/internal/event"
	"github.com/xxi0xx/owl-invites/internal/testutil"
)

type reminderTestEnv struct {
	db      database.DB
	store   *ReminderStore
	eventID string
}

func setupReminderJob(t *testing.T) *reminderTestEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	authStore := auth.NewStore(db)
	owner, err := authStore.CreateOrganizer(context.Background(), "reminder-owner@example.com")
	require.NoError(t, err)
	eventService := event.NewService(event.NewStore(db), 30)
	event, err := eventService.Create(context.Background(), owner.ID, event.CreateEventRequest{
		Title: "Reminder event", EventDate: "2027-06-15T14:00:00Z",
	})
	require.NoError(t, err)
	return &reminderTestEnv{db: db, store: NewReminderStore(db), eventID: event.ID}
}

func addReminder(t *testing.T, store *ReminderStore, eventID, group string, at time.Time, status string) *Reminder {
	t.Helper()
	reminder := &Reminder{
		ID: uuid.Must(uuid.NewV7()).String(), EventID: eventID, RemindAt: at,
		TargetGroup: group, Message: "Do not forget", Status: status,
	}
	require.NoError(t, store.Create(context.Background(), reminder))
	return reminder
}

func TestClaimForProcessingConcurrentOnlyOneWins(t *testing.T) {
	env := setupReminderJob(t)
	reminder := addReminder(t, env.store, env.eventID, "all", time.Now().Add(-time.Minute), "scheduled")

	const workers = 8
	var wins atomic.Int32
	var group sync.WaitGroup
	start := make(chan struct{})
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			claimed, err := env.store.ClaimForProcessing(context.Background(), reminder.ID)
			require.NoError(t, err)
			if claimed {
				wins.Add(1)
			}
		}()
	}
	close(start)
	group.Wait()
	assert.Equal(t, int32(1), wins.Load())
}

func TestReminderDeliversThroughInvitationBoundary(t *testing.T) {
	env := setupReminderJob(t)
	reminder := addReminder(t, env.store, env.eventID, "attending", time.Now().Add(-time.Minute), "scheduled")

	var gotEvent, gotGroup, gotSubject, gotBody string
	job := NewReminderJob(env.store, func(_ context.Context, eventID, group, subject, body string) (int, error) {
		gotEvent, gotGroup, gotSubject, gotBody = eventID, group, subject, body
		return 2, nil
	}, zerolog.Nop())
	require.NoError(t, job.Run(context.Background()))
	assert.Equal(t, env.eventID, gotEvent)
	assert.Equal(t, "attending", gotGroup)
	assert.Equal(t, "Event reminder", gotSubject)
	assert.Equal(t, "Do not forget", gotBody)

	stored, err := env.store.FindByID(context.Background(), reminder.ID)
	require.NoError(t, err)
	assert.Equal(t, "sent", stored.Status)
}

func TestReminderDeliveryFailureIsRecorded(t *testing.T) {
	env := setupReminderJob(t)
	reminder := addReminder(t, env.store, env.eventID, "all", time.Now().Add(-time.Minute), "scheduled")
	job := NewReminderJob(env.store, func(context.Context, string, string, string, string) (int, error) {
		return 0, errors.New("delivery failed")
	}, zerolog.Nop())

	require.NoError(t, job.Run(context.Background()), "one failed reminder must not stop the scheduler")
	stored, err := env.store.FindByID(context.Background(), reminder.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", stored.Status)
}

func TestFindDueOnlyReturnsScheduledPastRows(t *testing.T) {
	env := setupReminderJob(t)
	due := addReminder(t, env.store, env.eventID, "all", time.Now().Add(-time.Minute), "scheduled")
	addReminder(t, env.store, env.eventID, "all", time.Now().Add(time.Hour), "scheduled")
	addReminder(t, env.store, env.eventID, "all", time.Now().Add(-time.Minute), "cancelled")

	rows, err := env.store.FindDue(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, due.ID, rows[0].ID)
}

func TestReminderJobMetadata(t *testing.T) {
	env := setupReminderJob(t)
	job := NewReminderJob(env.store, nil, zerolog.Nop())
	assert.Equal(t, "reminder", job.Name())
	assert.Equal(t, 30*time.Second, job.Interval())
}

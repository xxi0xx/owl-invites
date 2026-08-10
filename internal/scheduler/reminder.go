package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

// ReminderDeliveryFunc sends an event reminder to invitation destinations.
// The recipient group is evaluated against per-guest attendance; no contact
// lookup or legacy RSVP token participates in delivery.
type ReminderDeliveryFunc func(ctx context.Context, eventID, recipientGroup, subject, body string) (int, error)

type ReminderJob struct {
	store   *ReminderStore
	deliver ReminderDeliveryFunc
	logger  zerolog.Logger
}

func NewReminderJob(store *ReminderStore, deliver ReminderDeliveryFunc, logger zerolog.Logger) *ReminderJob {
	return &ReminderJob{store: store, deliver: deliver, logger: logger}
}

func (j *ReminderJob) Name() string { return "reminder" }

func (j *ReminderJob) Interval() time.Duration { return 30 * time.Second }

func (j *ReminderJob) Run(ctx context.Context) error {
	due, err := j.store.FindDue(ctx)
	if err != nil {
		return fmt.Errorf("find due reminders: %w", err)
	}
	for _, reminder := range due {
		if err := j.processReminder(ctx, reminder); err != nil {
			j.logger.Error().Err(err).Str("reminder_id", reminder.ID).
				Str("event_id", reminder.EventID).Msg("failed to process invitation reminder")
		}
	}
	return nil
}

func (j *ReminderJob) processReminder(ctx context.Context, reminder *Reminder) error {
	claimed, err := j.store.ClaimForProcessing(ctx, reminder.ID)
	if err != nil {
		return fmt.Errorf("claim reminder: %w", err)
	}
	if !claimed {
		return nil
	}
	if j.deliver == nil {
		_ = j.store.SetStatus(ctx, reminder.ID, "failed")
		return fmt.Errorf("invitation reminder delivery is not configured")
	}
	body := reminder.Message
	if body == "" {
		body = "You have an upcoming event. Don't forget!"
	}
	sent, err := j.deliver(ctx, reminder.EventID, reminder.TargetGroup,
		"Event reminder", body)
	if err != nil {
		_ = j.store.SetStatus(ctx, reminder.ID, "failed")
		return err
	}
	j.logger.Info().Str("reminder_id", reminder.ID).Int("sent", sent).
		Msg("invitation reminder delivered")
	return j.store.SetStatus(ctx, reminder.ID, "sent")
}

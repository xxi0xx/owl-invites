// Command testfixture seeds the PostgreSQL backup/restore acceptance drill.
// It is not included in the production container.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/config"
	"github.com/yannkr/openrsvp/internal/database"
	"github.com/yannkr/openrsvp/internal/event"
	"github.com/yannkr/openrsvp/internal/invitation"
	"github.com/yannkr/openrsvp/internal/question"
)

type fixtureState struct {
	Capability   string `json:"capability"`
	InvitationID string `json:"invitationId"`
	GuestID      string `json:"guestId"`
	QuestionID   string `json:"questionId"`
	Answer       string `json:"answer"`
	Upload       string `json:"upload"`
}

func main() {
	statePath := flag.String("state", "", "restricted output file for acceptance state")
	flag.Parse()
	if *statePath == "" {
		fatalf("--state is required")
	}
	if err := seed(*statePath); err != nil {
		fatalf("seed recovery fixture: %v", err)
	}
}

func seed(statePath string) error {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.New(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := database.RunMigrations(db); err != nil {
		return err
	}

	owner, err := auth.NewStore(db).CreateOrganizer(ctx, "restore-owner@example.com")
	if err != nil {
		return err
	}
	events := event.NewService(event.NewStore(db), cfg.DefaultRetentionDays)
	events.SetCoHostStore(event.NewCoHostStore(db))
	record, err := events.Create(ctx, owner.ID, event.CreateEventRequest{
		Title: "PostgreSQL Restore Drill", EventDate: time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339), Timezone: "UTC",
	})
	if err != nil {
		return err
	}
	if _, err := events.Publish(ctx, record.ID, owner.ID); err != nil {
		return err
	}
	required := true
	questionRecord, err := question.NewService(question.NewStore(db)).Create(ctx, record.ID, question.CreateQuestionRequest{
		Label: "Restore proof", Type: "text", Required: &required, Scope: "invitation",
	})
	if err != nil {
		return err
	}

	invitations, err := invitation.NewService(invitation.NewStore(db), cfg.InvitationSecretKey, cfg.BaseURL, 24*time.Hour, 15*time.Minute)
	if err != nil {
		return err
	}
	email := "restore-household@example.com"
	created, err := invitations.CreatePrivate(ctx, record.ID, owner.ID, invitation.CreateRequest{
		Label: "Restore Household", ContactEmail: &email, PreferredDeliveryMethod: "email", AssignedGuestNames: []string{"Recovery Guest"},
	})
	if err != nil {
		return err
	}
	parts := strings.SplitN(created.AccessURL, "#", 2)
	if len(parts) != 2 || parts[1] == "" {
		return fmt.Errorf("fixture did not create a capability")
	}
	session, household, err := invitations.ExchangePrivate(ctx, parts[1])
	if err != nil {
		return err
	}
	const answer = "state survived pg_restore"
	if _, err := invitations.SubmitForSession(ctx, session, invitation.SubmitRequest{
		Version:           household.Response.Version,
		AssignedGuests:    []invitation.GuestAttendanceInput{{GuestID: household.Guests[0].ID, Attendance: invitation.AttendanceAttending}},
		InvitationAnswers: map[string]string{questionRecord.ID: answer},
	}); err != nil {
		return err
	}

	if err := os.MkdirAll(cfg.UploadsDir, 0o700); err != nil {
		return err
	}
	const upload = "restore-proof.txt"
	if err := os.WriteFile(filepath.Join(cfg.UploadsDir, upload), []byte("upload survived restore\n"), 0o600); err != nil {
		return err
	}
	state := fixtureState{
		Capability: parts[1], InvitationID: created.Invitation.ID, GuestID: household.Guests[0].ID,
		QuestionID: questionRecord.ID, Answer: answer, Upload: upload,
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, append(data, '\n'), 0o600)
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

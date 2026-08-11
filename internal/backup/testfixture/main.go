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

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/config"
	"github.com/xxi0xx/owl-invites/internal/database"
	"github.com/xxi0xx/owl-invites/internal/event"
	"github.com/xxi0xx/owl-invites/internal/invitation"
	invitecard "github.com/xxi0xx/owl-invites/internal/invite"
	"github.com/xxi0xx/owl-invites/internal/question"
)

type fixtureState struct {
	Capability            string `json:"capability"`
	InvitationID          string `json:"invitationId"`
	ImportedInvitationID  string `json:"importedInvitationId"`
	GuestID               string `json:"guestId"`
	AdditionalGuestID     string `json:"additionalGuestId"`
	InvitationQuestionID  string `json:"invitationQuestionId"`
	GuestQuestionID       string `json:"guestQuestionId"`
	InvitationAnswer      string `json:"invitationAnswer"`
	AdditionalGuestAnswer string `json:"additionalGuestAnswer"`
	CardHeading           string `json:"cardHeading"`
	Upload                string `json:"upload"`
}

func main() {
	statePath := flag.String("state", "", "restricted output file for acceptance state")
	verify := flag.Bool("verify", false, "verify restored Gate 5 product state instead of seeding")
	flag.Parse()
	if *statePath == "" {
		fatalf("--state is required")
	}
	if *verify {
		if err := verifyRestore(*statePath); err != nil {
			fatalf("verify recovery fixture: %v", err)
		}
		return
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
	questions := question.NewService(question.NewStore(db))
	invitationQuestion, err := questions.Create(ctx, record.ID, question.CreateQuestionRequest{
		Label: "Restore proof", Type: "text", Required: &required, Scope: "invitation",
	})
	if err != nil {
		return err
	}
	guestQuestion, err := questions.Create(ctx, record.ID, question.CreateQuestionRequest{
		Label: "Guest restore proof", Type: "text", Scope: "guest",
	})
	if err != nil {
		return err
	}
	const upload = "restore-proof.txt"
	const cardHeading = "PostgreSQL Recovery Garden"
	if _, err := invitecard.NewService(invitecard.NewStore(db), cfg.UploadsDir).Save(ctx, record.ID, invitecard.SaveInviteRequest{
		TemplateID: "garden-picnic", Heading: cardHeading, Body: "Presentation survived pg_restore.",
		Footer: "Recovery complete", PrimaryColor: "#225522", SecondaryColor: "#eef8ee", Font: "Inter",
		CustomData: `{"backgroundImage":"/api/v1/uploads/restore-proof.txt"}`,
	}); err != nil {
		return err
	}

	invitations, err := invitation.NewService(invitation.NewStore(db), cfg.InvitationSecretKey, cfg.BaseURL, 24*time.Hour, 15*time.Minute)
	if err != nil {
		return err
	}
	email := "restore-household@example.com"
	created, err := invitations.CreatePrivate(ctx, record.ID, owner.ID, invitation.CreateRequest{
		Label: "Restore Household", ContactEmail: &email, PreferredDeliveryMethod: "email",
		AdditionalGuestAllowance: 1, AssignedGuestNames: []string{"Recovery Guest"},
	})
	if err != nil {
		return err
	}
	parts := strings.SplitN(created.AccessURL, "#", 2)
	if len(parts) != 2 || parts[1] == "" {
		return fmt.Errorf("fixture did not create a capability")
	}
	imported, err := invitations.CommitImport(ctx, record.ID, owner.ID, invitation.ImportCommitRequest{Households: []invitation.ImportHousehold{{
		HouseholdKey: "separate-same-contact", HouseholdLabel: "Imported Restore Household",
		ContactEmail: &email, PreferredDelivery: "email", AssignedGuestNames: []string{"Zoë Imported"},
	}}})
	if err != nil {
		return err
	}
	session, household, err := invitations.ExchangePrivate(ctx, parts[1])
	if err != nil {
		return err
	}
	const invitationAnswer = "state survived pg_restore"
	updated, err := invitations.SubmitForSession(ctx, session, invitation.SubmitRequest{
		Version:           household.Response.Version,
		AssignedGuests:    []invitation.GuestAttendanceInput{{GuestID: household.Guests[0].ID, Attendance: invitation.AttendanceAttending}},
		AdditionalGuests:  []invitation.AdditionalGuestInput{{Name: "Recovery Plus One", Attendance: invitation.AttendanceMaybe}},
		InvitationAnswers: map[string]string{invitationQuestion.ID: invitationAnswer},
		GuestAnswers:      map[string]map[string]string{household.Guests[0].ID: {guestQuestion.ID: "assigned guest answer"}},
	})
	if err != nil {
		return err
	}
	if len(updated.Guests) != 2 {
		return fmt.Errorf("expected additional guest to be persisted")
	}
	additionalGuestID := updated.Guests[1].ID
	const additionalGuestAnswer = "additional guest answer survived"
	updated, err = invitations.SubmitForSession(ctx, session, invitation.SubmitRequest{
		Version:           updated.Response.Version,
		AssignedGuests:    []invitation.GuestAttendanceInput{{GuestID: household.Guests[0].ID, Attendance: invitation.AttendanceAttending}},
		AdditionalGuests:  []invitation.AdditionalGuestInput{{ID: additionalGuestID, Name: "Recovery Plus One", Attendance: invitation.AttendanceMaybe}},
		InvitationAnswers: map[string]string{invitationQuestion.ID: invitationAnswer},
		GuestAnswers: map[string]map[string]string{
			household.Guests[0].ID: {guestQuestion.ID: "assigned guest answer"},
			additionalGuestID:      {guestQuestion.ID: additionalGuestAnswer},
		},
	})
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO notification_log (
		id, event_id, invitation_id, channel, provider, status, delivery_status, error,
		recipient, subject, sent_at, created_at
	) VALUES ('gate5-postgres-restore-delivery', ?, ?, 'email', 'smtp', 'sent', 'sent', '', ?,
		'Restore delivery', ?, ?)`, record.ID, created.Invitation.ID, email, now, now); err != nil {
		return err
	}

	if err := os.MkdirAll(cfg.UploadsDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.UploadsDir, upload), []byte("upload survived restore\n"), 0o600); err != nil {
		return err
	}
	state := fixtureState{
		Capability: parts[1], InvitationID: created.Invitation.ID, ImportedInvitationID: imported.InvitationIDs[0],
		GuestID: household.Guests[0].ID, AdditionalGuestID: additionalGuestID,
		InvitationQuestionID: invitationQuestion.ID, GuestQuestionID: guestQuestion.ID,
		InvitationAnswer: invitationAnswer, AdditionalGuestAnswer: additionalGuestAnswer,
		CardHeading: cardHeading, Upload: upload,
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, append(data, '\n'), 0o600)
}

func verifyRestore(statePath string) error {
	ctx := context.Background()
	data, err := os.ReadFile(statePath)
	if err != nil {
		return err
	}
	var state fixtureState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.New(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	invitations, err := invitation.NewService(invitation.NewStore(db), cfg.InvitationSecretKey, cfg.BaseURL, 24*time.Hour, 15*time.Minute)
	if err != nil {
		return err
	}
	_, household, err := invitations.ExchangePrivate(ctx, state.Capability)
	if err != nil {
		return fmt.Errorf("old household capability: %w", err)
	}
	if household.Invitation.ID != state.InvitationID || household.Presentation == nil ||
		household.Presentation.Heading != state.CardHeading ||
		household.Presentation.BackgroundImage != "/api/v1/uploads/"+state.Upload {
		return fmt.Errorf("invitation presentation did not survive restore")
	}
	if !hasGuest(household, state.GuestID, invitation.GuestOriginAssigned, invitation.AttendanceAttending) ||
		!hasGuest(household, state.AdditionalGuestID, invitation.GuestOriginAdditional, invitation.AttendanceMaybe) {
		return fmt.Errorf("assigned/additional guest state did not survive restore")
	}
	if !hasInvitationAnswer(household, state.InvitationQuestionID, state.InvitationAnswer) ||
		!hasGuestAnswer(household, state.AdditionalGuestID, state.GuestQuestionID, state.AdditionalGuestAnswer) {
		return fmt.Errorf("scoped answers did not survive restore")
	}
	households, err := invitations.ListOrganizerHouseholds(ctx, household.Invitation.EventID, invitation.InvitationListFilter{})
	if err != nil {
		return err
	}
	if len(households) != 2 {
		return fmt.Errorf("expected two contact-equal isolated households, got %d", len(households))
	}
	var deliveryFound bool
	for _, item := range households {
		if item.Invitation.ID == state.InvitationID && item.LatestDelivery != nil && item.LatestDelivery.Status == "sent" {
			deliveryFound = true
		}
	}
	if !deliveryFound {
		return fmt.Errorf("delivery status did not survive restore")
	}
	exported, err := invitations.ExportEventCSV(ctx, household.Invitation.EventID)
	if err != nil {
		return err
	}
	exportText := string(exported)
	for _, expected := range []string{state.InvitationID, state.ImportedInvitationID, state.InvitationAnswer, state.AdditionalGuestAnswer} {
		if !strings.Contains(exportText, expected) {
			return fmt.Errorf("restored export is missing %q", expected)
		}
	}
	upload, err := os.ReadFile(filepath.Join(cfg.UploadsDir, state.Upload))
	if err != nil {
		return err
	}
	if string(upload) != "upload survived restore\n" {
		return fmt.Errorf("restored upload content changed")
	}
	return nil
}

func hasGuest(household *invitation.Household, id, origin, attendance string) bool {
	for _, guest := range household.Guests {
		if guest.ID == id && guest.Origin == origin && guest.Attendance == attendance {
			return true
		}
	}
	return false
}

func hasInvitationAnswer(household *invitation.Household, questionID, answer string) bool {
	for _, item := range household.InvitationAnswers {
		if item.QuestionID == questionID && item.Answer == answer {
			return true
		}
	}
	return false
}

func hasGuestAnswer(household *invitation.Household, guestID, questionID, answer string) bool {
	for _, item := range household.GuestAnswers {
		if item.GuestID == guestID && item.QuestionID == questionID && item.Answer == answer {
			return true
		}
	}
	return false
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

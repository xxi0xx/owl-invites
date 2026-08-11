package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backupops "github.com/xxi0xx/owl-invites/internal/backup"
	"github.com/xxi0xx/owl-invites/internal/config"
	"github.com/xxi0xx/owl-invites/internal/database"
	"github.com/xxi0xx/owl-invites/internal/invitation"
	"github.com/xxi0xx/owl-invites/internal/question"
	"github.com/xxi0xx/owl-invites/internal/testutil"
)

const restoreSecret = "4c64fb646f28c3cf57d320675a546e290173f4ab91786764"

func TestSQLiteBackupVerifyRestorePreservesCapabilityAndState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sourceDatabase := filepath.Join(root, "source.db")
	sourceUploads := filepath.Join(root, "source-uploads")
	require.NoError(t, os.Mkdir(sourceUploads, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(sourceUploads, "hero.png"), []byte("representative-upload"), 0o600))

	db := openMigratedSQLite(t, sourceDatabase)
	eventID, ownerID := seedInvitationEvent(t, db)
	required := true
	questionRecord, err := question.NewService(question.NewStore(db)).Create(ctx, eventID, question.CreateQuestionRequest{
		Label: "Meal preference", Type: "text", Required: &required, Scope: "invitation",
	})
	require.NoError(t, err)

	service, err := invitation.NewService(invitation.NewStore(db), restoreSecret, "https://invites.example", 24*time.Hour, 15*time.Minute)
	require.NoError(t, err)
	email := "household@example.com"
	created, err := service.CreatePrivate(ctx, eventID, ownerID, invitation.CreateRequest{
		Label: "Restore household", ContactEmail: &email, PreferredDeliveryMethod: "email", AssignedGuestNames: []string{"Alex"},
	})
	require.NoError(t, err)
	capability := strings.SplitN(created.AccessURL, "#", 2)[1]
	session, household, err := service.ExchangePrivate(ctx, capability)
	require.NoError(t, err)
	_, err = service.SubmitForSession(ctx, session, invitation.SubmitRequest{
		Version:           household.Response.Version,
		AssignedGuests:    []invitation.GuestAttendanceInput{{GuestID: household.Guests[0].ID, Attendance: invitation.AttendanceAttending}},
		InvitationAnswers: map[string]string{questionRecord.ID: "Vegetarian"},
	})
	require.NoError(t, err)

	bundle := filepath.Join(root, "backup-bundle")
	manifest, err := backupops.CreateSQLite(ctx, db, sourceUploads, bundle, restoreSecret)
	require.NoError(t, err)
	assert.Equal(t, uint(36), manifest.Source.SchemaVersion)
	assert.False(t, manifest.Source.SchemaDirty)
	assert.Equal(t, 1, manifest.Uploads.FileCount)
	assert.Contains(t, manifest.Uploads.Consistency, "not transactionally atomic")

	manifestBytes, err := os.ReadFile(filepath.Join(bundle, "manifest.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(manifestBytes), restoreSecret)
	verified, err := backupops.VerifySQLite(ctx, bundle)
	require.NoError(t, err)
	assert.Equal(t, manifest.Database.SHA256, verified.Database.SHA256)

	wrongDatabase := filepath.Join(root, "wrong", "restored.db")
	wrongUploads := filepath.Join(root, "wrong", "uploads")
	wrongSecret := "f84a173ee20738df4867a3d18439aa9f63a81cf46c996d9f"
	_, err = backupops.RestoreSQLite(ctx, bundle, wrongDatabase, wrongUploads, wrongSecret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret fingerprint mismatch")
	assert.NotContains(t, err.Error(), restoreSecret)
	assert.NotContains(t, err.Error(), wrongSecret)
	assert.NoFileExists(t, wrongDatabase)
	assert.NoDirExists(t, wrongUploads)

	restoredDatabase := filepath.Join(root, "restored", "openrsvp.db")
	restoredUploads := filepath.Join(root, "restored", "uploads")
	_, err = backupops.RestoreSQLite(ctx, bundle, restoredDatabase, restoredUploads, restoreSecret)
	require.NoError(t, err)
	assert.FileExists(t, restoredDatabase)
	upload, err := os.ReadFile(filepath.Join(restoredUploads, "hero.png"))
	require.NoError(t, err)
	assert.Equal(t, "representative-upload", string(upload))

	restoredDB := openSQLite(t, restoredDatabase)
	restoredService, err := invitation.NewService(invitation.NewStore(restoredDB), restoreSecret, "https://invites.example", 24*time.Hour, 15*time.Minute)
	require.NoError(t, err)
	_, restoredHousehold, err := restoredService.ExchangePrivate(ctx, capability)
	require.NoError(t, err)
	assert.Equal(t, invitation.AttendanceAttending, restoredHousehold.Guests[0].Attendance)
	require.Len(t, restoredHousehold.InvitationAnswers, 1)
	assert.Equal(t, "Vegetarian", restoredHousehold.InvitationAnswers[0].Answer)

	wrongService, err := invitation.NewService(invitation.NewStore(restoredDB), wrongSecret, "https://invites.example", 24*time.Hour, 15*time.Minute)
	require.NoError(t, err)
	_, _, err = wrongService.ExchangePrivate(ctx, capability)
	assert.ErrorIs(t, err, invitation.ErrInvalidCapability)

	require.NoError(t, os.WriteFile(filepath.Join(bundle, "uploads", "hero.png"), []byte("tampered"), 0o600))
	_, err = backupops.VerifySQLite(ctx, bundle)
	assert.ErrorContains(t, err, "uploads checksum")

	if runtime.GOOS != "windows" {
		for _, path := range []string{bundle, filepath.Join(bundle, "manifest.json"), filepath.Join(bundle, "database.sqlite")} {
			info, statErr := os.Stat(path)
			require.NoError(t, statErr)
			assert.Zero(t, info.Mode().Perm()&0o077, "%s should not be group/world accessible", path)
		}
	}
}

func TestSQLiteBackupRefusesExistingDestination(t *testing.T) {
	root := t.TempDir()
	uploads := filepath.Join(root, "uploads")
	require.NoError(t, os.Mkdir(uploads, 0o700))
	db := openMigratedSQLite(t, filepath.Join(root, "source.db"))
	destination := filepath.Join(root, "existing")
	require.NoError(t, os.Mkdir(destination, 0o700))

	_, err := backupops.CreateSQLite(context.Background(), db, uploads, destination, restoreSecret)
	assert.ErrorContains(t, err, "refusing to overwrite")
}

func openMigratedSQLite(t *testing.T, path string) database.DB {
	t.Helper()
	db := openSQLite(t, path)
	require.NoError(t, database.RunMigrations(db))
	return db
}

func openSQLite(t *testing.T, path string) database.DB {
	t.Helper()
	db, err := database.New(&config.Config{DBDriver: "sqlite", DBDSN: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedInvitationEvent(t *testing.T, db database.DB) (string, string) {
	t.Helper()
	const ownerID = "backup-owner"
	const eventID = "backup-event"
	testutil.SeedUser(t, db, ownerID, "backup-owner@example.com", "Backup Owner")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(context.Background(), `INSERT INTO events (
		id, title, description, event_date, location, timezone,
		retention_days, status, created_at, updated_at
	) VALUES (?, 'Backup Test', '', ?, '', 'UTC', 30, 'published', ?, ?)`,
		eventID, time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339), now, now)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `INSERT INTO event_memberships (
		id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
	) VALUES ('backup-membership', ?, ?, 'owner', ?, ?, ?)`, eventID, ownerID, ownerID, now, now)
	require.NoError(t, err)
	return eventID, ownerID
}

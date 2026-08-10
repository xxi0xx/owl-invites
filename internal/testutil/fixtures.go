package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yannkr/openrsvp/internal/database"
)

// SeedUser inserts a canonical active user for tests that exercise a real DB.
func SeedUser(t *testing.T, db database.DB, id, email, displayName string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(context.Background(), `INSERT INTO users (
		id, email, normalized_email, display_name, timezone, instance_role,
		status, activated_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, 'UTC', 'user', 'active', ?, ?, ?)`,
		id, email, email, displayName, now, now, now)
	require.NoError(t, err)
}

// SeedEventOwner inserts the authoritative owner membership for an event.
func SeedEventOwner(t *testing.T, db database.DB, eventID, userID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(context.Background(), `INSERT INTO event_memberships (
		id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
	) VALUES (?, ?, ?, 'owner', ?, ?, ?)`, "owner:"+eventID, eventID, userID, userID, now, now)
	require.NoError(t, err)
}

package question

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yannkr/openrsvp/internal/testutil"
)

func TestQuestionScopesAndCRUD(t *testing.T) {
	db := testutil.NewTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO events (id, title, event_date, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"event-question", "Question event", now, "draft", now, now)
	require.NoError(t, err)

	service := NewService(NewStore(db))
	required := true
	created, err := service.Create(context.Background(), "event-question", CreateQuestionRequest{
		Label: "Household note", Type: "text", Scope: "invitation", Required: &required,
	})
	require.NoError(t, err)
	assert.Equal(t, "invitation", created.Scope)

	guestScope := "guest"
	updated, err := service.Update(context.Background(), created.ID, UpdateQuestionRequest{Scope: &guestScope})
	require.NoError(t, err)
	assert.Equal(t, "guest", updated.Scope)

	questions, err := service.ListByEvent(context.Background(), "event-question")
	require.NoError(t, err)
	require.Len(t, questions, 1)
	assert.Equal(t, created.ID, questions[0].ID)

	require.NoError(t, service.Delete(context.Background(), created.ID))
	questions, err = service.ListByEvent(context.Background(), "event-question")
	require.NoError(t, err)
	assert.Empty(t, questions)
}

func TestQuestionRejectsUnknownScope(t *testing.T) {
	db := testutil.NewTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO events (id, title, event_date, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"event-question-scope", "Question event", now, "draft", now, now)
	require.NoError(t, err)

	_, err = NewService(NewStore(db)).Create(context.Background(), "event-question-scope", CreateQuestionRequest{
		Label: "Invalid", Type: "text", Scope: "attendee",
	})
	require.ErrorContains(t, err, "invalid question scope")
}

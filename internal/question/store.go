package question

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xxi0xx/owl-invites/internal/database"
)

// Store handles database operations for event questions. Invitation- and
// guest-scoped answers are owned by the invitation domain.
type Store struct {
	db database.DB
}

// NewStore creates a new question Store.
func NewStore(db database.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new question into the database.
func (s *Store) Create(ctx context.Context, q *Question) error {
	now := time.Now().UTC().Format(time.RFC3339)

	optionsJSON, err := json.Marshal(q.Options)
	if err != nil {
		return fmt.Errorf("marshal options: %w", err)
	}

	requiredInt := 0
	if q.Required {
		requiredInt = 1
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO event_questions (id, event_id, label, type, options, required, scope, sort_order, deleted, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		q.ID, q.EventID, q.Label, q.Type, string(optionsJSON),
		requiredInt, q.Scope, q.SortOrder, now, now,
	)
	if err != nil {
		return fmt.Errorf("create question: %w", err)
	}

	created, _ := time.Parse(time.RFC3339, now)
	q.CreatedAt = created
	q.UpdatedAt = created

	return nil
}

// FindByEventID retrieves all non-deleted questions for a given event, ordered
// by sort_order ascending.
func (s *Store) FindByEventID(ctx context.Context, eventID string) ([]*Question, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event_id, label, type, options, required, scope, sort_order, deleted, created_at, updated_at
		 FROM event_questions WHERE event_id = ? AND deleted = 0 ORDER BY sort_order ASC`,
		eventID,
	)
	if err != nil {
		return nil, fmt.Errorf("find questions by event: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var questions []*Question
	for rows.Next() {
		q, err := scanQuestionRow(rows)
		if err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate questions: %w", err)
	}

	return questions, nil
}

// FindByID retrieves a single question by ID.
func (s *Store) FindByID(ctx context.Context, id string) (*Question, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, label, type, options, required, scope, sort_order, deleted, created_at, updated_at
		 FROM event_questions WHERE id = ?`, id,
	)
	return scanQuestion(row)
}

// Update persists changes to an existing question.
func (s *Store) Update(ctx context.Context, q *Question) error {
	now := time.Now().UTC().Format(time.RFC3339)

	optionsJSON, err := json.Marshal(q.Options)
	if err != nil {
		return fmt.Errorf("marshal options: %w", err)
	}

	requiredInt := 0
	if q.Required {
		requiredInt = 1
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE event_questions SET label = ?, type = ?, options = ?, required = ?, scope = ?, sort_order = ?, updated_at = ?
		 WHERE id = ?`,
		q.Label, q.Type, string(optionsJSON), requiredInt, q.Scope, q.SortOrder, now, q.ID,
	)
	if err != nil {
		return fmt.Errorf("update question: %w", err)
	}

	q.UpdatedAt, _ = time.Parse(time.RFC3339, now)
	return nil
}

// SoftDelete marks a question as deleted.
func (s *Store) SoftDelete(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`UPDATE event_questions SET deleted = 1, updated_at = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("soft delete question: %w", err)
	}
	return nil
}

// CountByEventID returns the number of non-deleted questions for an event.
func (s *Store) CountByEventID(ctx context.Context, eventID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM event_questions WHERE event_id = ? AND deleted = 0`,
		eventID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count questions by event: %w", err)
	}
	return count, nil
}

// UpdateSortOrders batch-updates sort_order based on position in the orderedIDs
// slice.
func (s *Store) UpdateSortOrders(ctx context.Context, eventID string, orderedIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)

	for i, id := range orderedIDs {
		_, err := tx.ExecContext(ctx,
			`UPDATE event_questions SET sort_order = ?, updated_at = ? WHERE id = ? AND event_id = ? AND deleted = 0`,
			i, now, id, eventID,
		)
		if err != nil {
			return fmt.Errorf("update sort order for %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sort order update: %w", err)
	}
	return nil
}

// scanQuestion scans a single sql.Row into a Question.
func scanQuestion(row *sql.Row) (*Question, error) {
	var q Question
	var optionsStr string
	var requiredInt, deletedInt int
	var createdAt, updatedAt string

	err := row.Scan(
		&q.ID, &q.EventID, &q.Label, &q.Type, &optionsStr,
		&requiredInt, &q.Scope, &q.SortOrder, &deletedInt, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan question: %w", err)
	}

	return parseQuestion(&q, optionsStr, requiredInt, deletedInt, createdAt, updatedAt)
}

// scanQuestionRow scans a single row from sql.Rows into a Question.
func scanQuestionRow(rows *sql.Rows) (*Question, error) {
	var q Question
	var optionsStr string
	var requiredInt, deletedInt int
	var createdAt, updatedAt string

	err := rows.Scan(
		&q.ID, &q.EventID, &q.Label, &q.Type, &optionsStr,
		&requiredInt, &q.Scope, &q.SortOrder, &deletedInt, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan question row: %w", err)
	}

	return parseQuestion(&q, optionsStr, requiredInt, deletedInt, createdAt, updatedAt)
}

// parseQuestion populates derived fields (Options, Required, Deleted, timestamps).
func parseQuestion(q *Question, optionsStr string, requiredInt, deletedInt int, createdAt, updatedAt string) (*Question, error) {
	if optionsStr != "" {
		if err := json.Unmarshal([]byte(optionsStr), &q.Options); err != nil {
			return nil, fmt.Errorf("parse options: %w", err)
		}
	}
	if q.Options == nil {
		q.Options = []string{}
	}

	q.Required = requiredInt != 0
	q.Deleted = deletedInt != 0

	var err error
	q.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	q.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return q, nil
}

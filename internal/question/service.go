package question

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// maxQuestionsPerEvent is the maximum number of questions allowed per event.
const maxQuestionsPerEvent = 10

// maxOptionsPerQuestion is the maximum number of options per select/checkbox question.
const maxOptionsPerQuestion = 20

// maxLabelLength is the maximum length for a question label.
const maxLabelLength = 500

// maxOptionLength is the maximum length for a single option value.
const maxOptionLength = 200

// validTypes lists the allowed question types.
var validTypes = map[string]bool{
	"text":     true,
	"select":   true,
	"checkbox": true,
}

var validScopes = map[string]bool{
	"invitation": true,
	"guest":      true,
}

// Service contains the business logic for event questions.
type Service struct {
	store *Store
}

// NewService creates a new question Service.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// Create creates a new question for an event.
func (s *Service) Create(ctx context.Context, eventID string, req CreateQuestionRequest) (*Question, error) {
	label := strings.TrimSpace(req.Label)
	if label == "" {
		return nil, fmt.Errorf("label is required")
	}
	if len(label) > maxLabelLength {
		return nil, fmt.Errorf("label must be %d characters or fewer", maxLabelLength)
	}

	if !validTypes[req.Type] {
		return nil, fmt.Errorf("invalid question type: must be text, select, or checkbox")
	}

	options := req.Options
	if options == nil {
		options = []string{}
	}

	// select and checkbox require at least 2 options.
	if (req.Type == "select" || req.Type == "checkbox") && len(options) < 2 {
		return nil, fmt.Errorf("select and checkbox questions require at least 2 options")
	}

	if len(options) > maxOptionsPerQuestion {
		return nil, fmt.Errorf("maximum %d options per question", maxOptionsPerQuestion)
	}

	for _, opt := range options {
		if strings.TrimSpace(opt) == "" {
			return nil, fmt.Errorf("option values cannot be empty")
		}
		if len(opt) > maxOptionLength {
			return nil, fmt.Errorf("option values must be %d characters or fewer", maxOptionLength)
		}
	}

	// Check question limit per event.
	count, err := s.store.CountByEventID(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("count questions: %w", err)
	}
	if count >= maxQuestionsPerEvent {
		return nil, fmt.Errorf("maximum %d questions per event", maxQuestionsPerEvent)
	}

	required := false
	if req.Required != nil {
		required = *req.Required
	}
	scope := req.Scope
	if scope == "" {
		scope = "guest"
	}
	if !validScopes[scope] {
		return nil, fmt.Errorf("invalid question scope: must be invitation or guest")
	}

	sortOrder := count // default to appending at the end
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}

	q := &Question{
		ID:        uuid.Must(uuid.NewV7()).String(),
		EventID:   eventID,
		Label:     label,
		Type:      req.Type,
		Options:   options,
		Required:  required,
		Scope:     scope,
		SortOrder: sortOrder,
	}

	if err := s.store.Create(ctx, q); err != nil {
		return nil, err
	}

	return q, nil
}

// Update applies partial updates to an existing question.
func (s *Service) Update(ctx context.Context, questionID string, req UpdateQuestionRequest) (*Question, error) {
	q, err := s.store.FindByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q == nil || q.Deleted {
		return nil, fmt.Errorf("question not found")
	}

	if req.Label != nil {
		label := strings.TrimSpace(*req.Label)
		if label == "" {
			return nil, fmt.Errorf("label is required")
		}
		if len(label) > maxLabelLength {
			return nil, fmt.Errorf("label must be %d characters or fewer", maxLabelLength)
		}
		q.Label = label
	}

	if req.Type != nil {
		if !validTypes[*req.Type] {
			return nil, fmt.Errorf("invalid question type: must be text, select, or checkbox")
		}
		q.Type = *req.Type
	}

	if req.Options != nil {
		for _, opt := range req.Options {
			if strings.TrimSpace(opt) == "" {
				return nil, fmt.Errorf("option values cannot be empty")
			}
			if len(opt) > maxOptionLength {
				return nil, fmt.Errorf("option values must be %d characters or fewer", maxOptionLength)
			}
		}
		q.Options = req.Options
	}

	// Validate options for select/checkbox after applying updates.
	if (q.Type == "select" || q.Type == "checkbox") && len(q.Options) < 2 {
		return nil, fmt.Errorf("select and checkbox questions require at least 2 options")
	}

	if len(q.Options) > maxOptionsPerQuestion {
		return nil, fmt.Errorf("maximum %d options per question", maxOptionsPerQuestion)
	}

	if req.Required != nil {
		q.Required = *req.Required
	}
	if req.Scope != nil {
		if !validScopes[*req.Scope] {
			return nil, fmt.Errorf("invalid question scope: must be invitation or guest")
		}
		q.Scope = *req.Scope
	}

	if req.SortOrder != nil {
		q.SortOrder = *req.SortOrder
	}

	if err := s.store.Update(ctx, q); err != nil {
		return nil, err
	}

	return q, nil
}

// Delete soft-deletes a question.
func (s *Service) Delete(ctx context.Context, questionID string) error {
	q, err := s.store.FindByID(ctx, questionID)
	if err != nil {
		return err
	}
	if q == nil || q.Deleted {
		return fmt.Errorf("question not found")
	}
	return s.store.SoftDelete(ctx, questionID)
}

// ListByEvent returns all non-deleted questions for an event.
func (s *Service) ListByEvent(ctx context.Context, eventID string) ([]*Question, error) {
	questions, err := s.store.FindByEventID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if questions == nil {
		questions = []*Question{}
	}
	return questions, nil
}

// Reorder updates the sort order of questions for an event.
func (s *Service) Reorder(ctx context.Context, eventID string, orderedIDs []string) error {
	return s.store.UpdateSortOrders(ctx, eventID, orderedIDs)
}

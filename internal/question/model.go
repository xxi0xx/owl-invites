package question

import "time"

// Question represents a custom question attached to an event.
type Question struct {
	ID        string    `json:"id"`
	EventID   string    `json:"eventId"`
	Label     string    `json:"label"`
	Type      string    `json:"type"`    // text, select, checkbox
	Options   []string  `json:"options"` // For select/checkbox
	Required  bool      `json:"required"`
	Scope     string    `json:"scope"` // invitation or guest
	SortOrder int       `json:"sortOrder"`
	Deleted   bool      `json:"-"` // Hidden from API
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CreateQuestionRequest is the request body for creating a new question.
type CreateQuestionRequest struct {
	Label     string   `json:"label"`
	Type      string   `json:"type"`
	Options   []string `json:"options,omitempty"`
	Required  *bool    `json:"required,omitempty"`
	Scope     string   `json:"scope"`
	SortOrder *int     `json:"sortOrder,omitempty"`
}

// UpdateQuestionRequest is the request body for updating an existing question.
type UpdateQuestionRequest struct {
	Label     *string  `json:"label,omitempty"`
	Type      *string  `json:"type,omitempty"`
	Options   []string `json:"options,omitempty"`
	Required  *bool    `json:"required,omitempty"`
	Scope     *string  `json:"scope,omitempty"`
	SortOrder *int     `json:"sortOrder,omitempty"`
}

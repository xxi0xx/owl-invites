package event

import "time"

// CoHost is the co-host view of a canonical event membership. OrganizerID and
// AddedBy remain internal compatibility fields while callers migrate to user
// terminology.
type CoHost struct {
	ID              string    `json:"id"`
	EventID         string    `json:"eventId"`
	UserID          string    `json:"userId"`
	OrganizerID     string    `json:"-"`
	Role            string    `json:"role"`
	GrantedByUserID string    `json:"grantedByUserId"`
	AddedBy         string    `json:"-"`
	CreatedAt       time.Time `json:"createdAt"`
	Email           string    `json:"email,omitempty"`
	Name            string    `json:"name,omitempty"`
	OrganizerEmail  string    `json:"organizerEmail,omitempty"`
	OrganizerName   string    `json:"organizerName,omitempty"`
}

// AddCoHostRequest is the request body for adding a co-host to an event.
type AddCoHostRequest struct {
	Email string `json:"email"`
}

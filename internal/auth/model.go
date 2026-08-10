package auth

import "time"

// User is the sole persistent Owl Invites identity. Event ownership is held by
// event_memberships; no organizer identity table or ownership mirror exists.
type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	NormalizedEmail string     `json:"-"`
	Name            string     `json:"name"`
	Timezone        string     `json:"timezone"`
	InstanceRole    string     `json:"instanceRole"`
	Status          string     `json:"status"`
	IsAdmin         bool       `json:"isAdmin"`
	InvitedByUserID *string    `json:"invitedByUserId,omitempty"`
	ActivatedAt     *time.Time `json:"activatedAt,omitempty"`
	LastLoginAt     *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// Organizer is a source-level vocabulary alias for User, not a second model or
// persistence boundary. New identity and administration code should use User.
type Organizer = User

const (
	InstanceRoleAdmin = "admin"
	InstanceRoleUser  = "user"

	UserStatusInvited  = "invited"
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// UpdateProfileRequest is the request body for updating an organizer's profile.
type UpdateProfileRequest struct {
	Name     *string `json:"name,omitempty"`
	Timezone *string `json:"timezone,omitempty"`
}

// MagicLink is a one-time-use token sent via email for passwordless login.
type MagicLink struct {
	ID          string
	TokenHash   string
	OrganizerID string
	ExpiresAt   time.Time
	UsedAt      *time.Time
	CreatedAt   time.Time
}

// Session represents an authenticated session for an organizer.
type Session struct {
	ID          string
	TokenHash   string
	OrganizerID string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

// MagicLinkRequest is the request body for requesting a magic link.
type MagicLinkRequest struct {
	Email string `json:"email"`
}

// MagicLinkResponse is the response body after requesting a magic link.
type MagicLinkResponse struct {
	Message string `json:"message"`
}

// VerifyRequest is the request body for verifying a magic link token.
type VerifyRequest struct {
	Token string `json:"token"`
}

// AuthResponse is returned after successful authentication.
type AuthResponse struct {
	Token     string     `json:"token"`
	Organizer *Organizer `json:"organizer"`
}

// ExportDocument is the full GDPR-style data export for a single organizer.
// Rows from per-domain tables are returned as generic maps keyed by column
// name to keep the export decoupled from each domain's model structs.
type ExportDocument struct {
	ExportedAt        string           `json:"exportedAt"`
	Organizer         *Organizer       `json:"organizer"`
	Events            []map[string]any `json:"events"`
	Series            []map[string]any `json:"series"`
	Invitations       []map[string]any `json:"invitations"`
	Guests            []map[string]any `json:"guests"`
	Responses         []map[string]any `json:"responses"`
	GuestResponses    []map[string]any `json:"guestResponses"`
	Questions         []map[string]any `json:"questions"`
	InvitationAnswers []map[string]any `json:"invitationAnswers"`
	GuestAnswers      []map[string]any `json:"guestAnswers"`
	Messages          []map[string]any `json:"invitationMessages"`
	Webhooks          []map[string]any `json:"webhooks"`
	Reminders         []map[string]any `json:"reminders"`
	InviteCards       []map[string]any `json:"inviteCards"`
	NotificationLog   []map[string]any `json:"notificationLog"`
}

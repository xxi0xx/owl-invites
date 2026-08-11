package invitation

import "time"

const (
	SourcePrivate = "private"
	SourceOpen    = "open"

	GuestOriginAssigned   = "assigned"
	GuestOriginAdditional = "additional"

	AttendancePending   = "pending"
	AttendanceAttending = "attending"
	AttendanceMaybe     = "maybe"
	AttendanceDeclined  = "declined"

	QuestionScopeInvitation = "invitation"
	QuestionScopeGuest      = "guest"

	DeliveryNotRequested = "not_requested"
	DeliverySent         = "sent"
	DeliveryFailed       = "failed"
)

// Invitation is the household and capability boundary for an event. Contact
// fields are delivery/recovery data and are deliberately non-unique.
type Invitation struct {
	ID                       string     `json:"id"`
	EventID                  string     `json:"eventId"`
	Label                    string     `json:"label"`
	ContactEmail             *string    `json:"contactEmail,omitempty"`
	ContactPhone             *string    `json:"contactPhone,omitempty"`
	PreferredDeliveryMethod  string     `json:"preferredDeliveryMethod"`
	AdditionalGuestAllowance int        `json:"additionalGuestAllowance"`
	Source                   string     `json:"source"`
	OpenEnrollmentID         *string    `json:"openEnrollmentId,omitempty"`
	AccessID                 string     `json:"-"`
	TokenVersion             int        `json:"tokenVersion"`
	CreatedByUserID          *string    `json:"createdByUserId,omitempty"`
	RevokedAt                *time.Time `json:"revokedAt,omitempty"`
	RevocationReason         *string    `json:"revocationReason,omitempty"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

type Guest struct {
	ID           string     `json:"id"`
	InvitationID string     `json:"invitationId"`
	Name         string     `json:"name"`
	Origin       string     `json:"origin"`
	SortOrder    int        `json:"sortOrder"`
	Attendance   string     `json:"attendance"`
	RemovedAt    *time.Time `json:"removedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type Response struct {
	ID           string     `json:"id"`
	InvitationID string     `json:"invitationId"`
	Version      int        `json:"version"`
	SubmittedAt  *time.Time `json:"submittedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type Question struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Type      string   `json:"type"`
	Options   []string `json:"options"`
	Required  bool     `json:"required"`
	Scope     string   `json:"scope"`
	SortOrder int      `json:"sortOrder"`
}

type Answer struct {
	QuestionID string `json:"questionId"`
	Answer     string `json:"answer"`
}

type GuestAnswer struct {
	GuestID    string `json:"guestId"`
	QuestionID string `json:"questionId"`
	Answer     string `json:"answer"`
}

type EventSummary struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	EventDate   time.Time  `json:"eventDate"`
	EndDate     *time.Time `json:"endDate,omitempty"`
	Location    string     `json:"location"`
	Timezone    string     `json:"timezone"`
	Status      string     `json:"status"`
}

// GuestPresentation is the safe, read-only subset of invite-card data exposed
// through an already-authorized household session. Arbitrary custom data,
// storage paths, organizer metadata, and card administration fields are absent.
type GuestPresentation struct {
	TemplateID      string `json:"templateId"`
	Heading         string `json:"heading"`
	Body            string `json:"body"`
	Footer          string `json:"footer"`
	PrimaryColor    string `json:"primaryColor"`
	SecondaryColor  string `json:"secondaryColor"`
	Font            string `json:"font"`
	BackgroundImage string `json:"backgroundImage,omitempty"`
}

// Household is the complete invitation-scoped view returned to an organizer
// or to a browser holding a valid invitation session.
type Household struct {
	Invitation        *Invitation        `json:"invitation"`
	Event             *EventSummary      `json:"event"`
	Response          *Response          `json:"response"`
	Guests            []*Guest           `json:"guests"`
	Questions         []*Question        `json:"questions"`
	InvitationAnswers []Answer           `json:"invitationAnswers"`
	GuestAnswers      []GuestAnswer      `json:"guestAnswers"`
	Presentation      *GuestPresentation `json:"presentation"`
	LatestDelivery    *DeliverySummary   `json:"latestDelivery,omitempty"`
}

type CreateRequest struct {
	Label                    string   `json:"label"`
	ContactEmail             *string  `json:"contactEmail,omitempty"`
	ContactPhone             *string  `json:"contactPhone,omitempty"`
	PreferredDeliveryMethod  string   `json:"preferredDeliveryMethod"`
	AdditionalGuestAllowance int      `json:"additionalGuestAllowance"`
	AssignedGuestNames       []string `json:"assignedGuestNames"`
	Send                     bool     `json:"send"`
}

type AssignedGuestEdit struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

type UpdateInvitationRequest struct {
	Label                    string              `json:"label"`
	ContactEmail             *string             `json:"contactEmail,omitempty"`
	ContactPhone             *string             `json:"contactPhone,omitempty"`
	PreferredDeliveryMethod  string              `json:"preferredDeliveryMethod"`
	AdditionalGuestAllowance int                 `json:"additionalGuestAllowance"`
	AssignedGuests           []AssignedGuestEdit `json:"assignedGuests"`
}

type DeliverySummary struct {
	Status         string     `json:"status"`
	DeliveryStatus string     `json:"deliveryStatus"`
	Provider       string     `json:"provider"`
	Error          string     `json:"error,omitempty"`
	AttemptedAt    time.Time  `json:"attemptedAt"`
	SentAt         *time.Time `json:"sentAt,omitempty"`
}

type CreateResult struct {
	Invitation *Invitation    `json:"invitation"`
	Guests     []*Guest       `json:"guests"`
	AccessURL  string         `json:"accessUrl,omitempty"`
	Delivery   DeliveryResult `json:"delivery"`
}

// DeliveryResult reports the delivery attempt separately from the resource
// creation that preceded it. A failed post-commit attempt is nonfatal and the
// underlying provider error is available only to the server log.
type DeliveryResult struct {
	Status  string `json:"status"`
	Warning string `json:"warning,omitempty"`
	err     error
}

type AdditionalGuestInput struct {
	ID string `json:"id,omitempty"`
	// ClientKey correlates answers for a not-yet-persisted additional guest
	// during one atomic submission. It is validated, consumed, and never stored.
	ClientKey  string `json:"clientKey,omitempty"`
	Name       string `json:"name"`
	Attendance string `json:"attendance"`
}

type GuestAttendanceInput struct {
	GuestID    string `json:"guestId"`
	Attendance string `json:"attendance"`
}

type SubmitRequest struct {
	Version           int                          `json:"version"`
	AssignedGuests    []GuestAttendanceInput       `json:"assignedGuests"`
	AdditionalGuests  []AdditionalGuestInput       `json:"additionalGuests"`
	InvitationAnswers map[string]string            `json:"invitationAnswers"`
	GuestAnswers      map[string]map[string]string `json:"guestAnswers"`
}

type RecoveryRequest struct {
	EventID string `json:"eventId"`
	Contact string `json:"contact"`
}

type OpenEnrollmentConfig struct {
	ID           string     `json:"id"`
	EventID      string     `json:"eventId"`
	Enabled      bool       `json:"enabled"`
	OpensAt      *time.Time `json:"opensAt,omitempty"`
	ClosesAt     *time.Time `json:"closesAt,omitempty"`
	MaxPartySize int        `json:"maxPartySize"`
	Capacity     *int       `json:"capacity,omitempty"`
	TokenVersion int        `json:"tokenVersion"`
	RevokedAt    *time.Time `json:"revokedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	AccessID     string     `json:"-"`
}

type ConfigureOpenRequest struct {
	Enabled      bool    `json:"enabled"`
	OpensAt      *string `json:"opensAt,omitempty"`
	ClosesAt     *string `json:"closesAt,omitempty"`
	MaxPartySize int     `json:"maxPartySize"`
	Capacity     *int    `json:"capacity,omitempty"`
}

type OpenEnrollmentRequest struct {
	Capability              string   `json:"capability"`
	Label                   string   `json:"label"`
	ContactEmail            *string  `json:"contactEmail,omitempty"`
	ContactPhone            *string  `json:"contactPhone,omitempty"`
	PreferredDeliveryMethod string   `json:"preferredDeliveryMethod"`
	GuestNames              []string `json:"guestNames"`
}

type MessageRequest struct {
	RecipientGroup string `json:"recipientGroup"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
}

type MessagePreviewRequest struct {
	RecipientGroup string `json:"recipientGroup"`
}

type MessagePreview struct {
	RecipientGroup      string `json:"recipientGroup"`
	RecipientHouseholds int    `json:"recipientHouseholds"`
}

type MessageResult struct {
	Attempted int `json:"attempted"`
	Accepted  int `json:"accepted"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

type InvitationMessage struct {
	ID             string    `json:"id"`
	EventID        string    `json:"eventId"`
	SenderUserID   *string   `json:"senderUserId,omitempty"`
	RecipientGroup string    `json:"recipientGroup"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ImportIssue is a row-addressable validation result shown during CSV preview.
// Row zero identifies a file/header-level issue.
type ImportIssue struct {
	Row     int    `json:"row,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// ImportHousehold is normalized, organizer-reviewed import data. HouseholdKey
// groups rows only for this import and is deliberately never persisted.
type ImportHousehold struct {
	HouseholdKey             string   `json:"householdKey"`
	HouseholdLabel           string   `json:"householdLabel"`
	ContactEmail             *string  `json:"contactEmail,omitempty"`
	ContactPhone             *string  `json:"contactPhone,omitempty"`
	PreferredDelivery        string   `json:"preferredDelivery"`
	AdditionalGuestAllowance int      `json:"additionalGuestAllowance"`
	AssignedGuestNames       []string `json:"assignedGuestNames"`
}

type ImportPreview struct {
	HouseholdCount     int               `json:"householdCount"`
	AssignedGuestCount int               `json:"assignedGuestCount"`
	Households         []ImportHousehold `json:"households"`
	Errors             []ImportIssue     `json:"errors"`
	Warnings           []ImportIssue     `json:"warnings"`
}

type ImportCommitRequest struct {
	Households []ImportHousehold `json:"households"`
}

type ImportCommitResult struct {
	HouseholdCount     int      `json:"householdCount"`
	AssignedGuestCount int      `json:"assignedGuestCount"`
	InvitationIDs      []string `json:"invitationIds"`
}

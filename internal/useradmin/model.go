package useradmin

import (
	"encoding/json"
	"time"

	"github.com/xxi0xx/owl-invites/internal/auth"
)

type AccountInvite struct {
	ID              string     `json:"id"`
	TargetUserID    string     `json:"targetUserId"`
	Email           string     `json:"email"`
	InvitedByUserID string     `json:"invitedByUserId"`
	EventID         *string    `json:"eventId,omitempty"`
	EventRole       *string    `json:"eventRole,omitempty"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	AcceptedAt      *time.Time `json:"acceptedAt,omitempty"`
	RevokedAt       *time.Time `json:"revokedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// IssuedInvite contains the delivery-only capability. JSON deliberately
// excludes both raw and hashed token material.
type IssuedInvite struct {
	*AccountInvite
	RawToken  string `json:"-"`
	TokenHash string `json:"-"`
}

type AuditEntry struct {
	ID           string          `json:"id"`
	ActorUserID  *string         `json:"actorUserId,omitempty"`
	ActorKind    string          `json:"actorKind"`
	Action       string          `json:"action"`
	TargetUserID *string         `json:"targetUserId,omitempty"`
	EventID      *string         `json:"eventId,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"createdAt"`
}

type CreateInviteRequest struct {
	Email string `json:"email"`
}

type AcceptInviteRequest struct {
	Token string `json:"token"`
}

type UpdateUserStatusRequest struct {
	Status string `json:"status"`
}

type UpdateUserRoleRequest struct {
	InstanceRole string `json:"instanceRole"`
}

type UserList struct {
	Users []*auth.User `json:"users"`
}

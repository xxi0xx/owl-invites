package useradmin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/mail"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/config"
	"github.com/xxi0xx/owl-invites/internal/database"
)

var (
	ErrInvalidInvite     = errors.New("invalid or expired account invitation")
	ErrUserExists        = errors.New("a user with that email already exists")
	ErrUserNotFound      = errors.New("user not found")
	ErrLastAdmin         = errors.New("the last active administrator cannot be changed")
	ErrInvalidTransition = errors.New("invalid user status transition")
	ErrEmailUnavailable  = errors.New("account invitation email is unavailable")
	ErrCohostLimit       = errors.New("event cohost limit reached")
)

type EmailSender func(ctx context.Context, to, subject, htmlBody, plainBody string) error

type Service struct {
	store     *Store
	authStore *auth.Store
	cfg       *config.Config
	logger    zerolog.Logger
	sendEmail EmailSender
}

func NewService(store *Store, authStore *auth.Store, cfg *config.Config, logger zerolog.Logger) *Service {
	return &Service{store: store, authStore: authStore, cfg: cfg, logger: logger}
}

func (s *Service) SetEmailSender(sender EmailSender) { s.sendEmail = sender }

func (s *Service) ListUsers(ctx context.Context) ([]*auth.User, error) {
	return s.store.ListUsers(ctx)
}

func (s *Service) ListPendingInvites(ctx context.Context) ([]AccountInvite, error) {
	return s.store.ListPendingInvites(ctx, time.Now().UTC())
}

func (s *Service) InviteUser(ctx context.Context, actor *auth.User, email string) (*IssuedInvite, error) {
	if actor == nil || !actor.IsAdmin || actor.Status != auth.UserStatusActive {
		return nil, ErrUserNotFound
	}
	if s.sendEmail == nil {
		return nil, ErrEmailUnavailable
	}
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 320 {
		return nil, auth.ErrInvalidEmail
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin account invite: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	persistedActor, err := s.authStore.FindOrganizerByIDTx(ctx, tx, actor.ID)
	if err != nil {
		return nil, err
	}
	if persistedActor == nil || !persistedActor.IsAdmin || persistedActor.Status != auth.UserStatusActive {
		return nil, ErrUserNotFound
	}
	actor = persistedActor

	target, err := s.authStore.FindUserByEmailTx(ctx, tx, email)
	if err != nil {
		return nil, err
	}
	if target == nil {
		target, err = s.authStore.CreateUserTx(ctx, tx, email, "", s.cfg.DefaultTimezone,
			auth.InstanceRoleUser, auth.UserStatusInvited, &actor.ID)
		if err != nil {
			return nil, err
		}
	} else if target.Status != auth.UserStatusInvited {
		return nil, ErrUserExists
	}

	now := time.Now().UTC()
	if err := s.store.RevokePendingInvitesTx(ctx, tx, target.ID, now); err != nil {
		return nil, err
	}
	rawToken, tokenHash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate account invite: %w", err)
	}
	invite, err := s.store.CreateInviteTx(ctx, tx, target, actor.ID, tokenHash, nil, nil,
		now.Add(s.cfg.AccountInviteExpiry), now)
	if err != nil {
		return nil, err
	}
	if err := s.store.InsertAuditTx(ctx, tx, actor.ID, "account_invite_issued", target.ID,
		map[string]any{"email": target.NormalizedEmail}, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit account invite: %w", err)
	}

	htmlBody, plainBody := s.renderInviteEmail(rawToken, invite.ExpiresAt)
	if err := s.sendEmail(ctx, invite.Email, "You're invited to Owl Invites", htmlBody, plainBody); err != nil {
		return nil, fmt.Errorf("deliver account invite: %w", err)
	}
	return &IssuedInvite{AccountInvite: invite, RawToken: rawToken, TokenHash: tokenHash}, nil
}

// InviteEventCohost lets an explicit event owner sponsor an account even when
// public signups are disabled. Existing active users receive a membership;
// new/invited users receive an account capability that grants membership only
// when it is accepted.
func (s *Service) InviteEventCohost(ctx context.Context, actor *auth.User, eventID, email string) (accountInvitePending bool, err error) {
	if actor == nil || actor.Status != auth.UserStatusActive || s.sendEmail == nil {
		return false, ErrUserNotFound
	}
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 320 {
		return false, auth.ErrInvalidEmail
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.store.LockEventTx(ctx, tx, eventID); err != nil {
		return false, err
	}
	persistedActor, err := s.authStore.FindOrganizerByIDTx(ctx, tx, actor.ID)
	if err != nil {
		return false, err
	}
	if persistedActor == nil || persistedActor.Status != auth.UserStatusActive {
		return false, ErrUserNotFound
	}
	isOwner, err := s.store.IsEventOwnerTx(ctx, tx, eventID, persistedActor.ID)
	if err != nil {
		return false, err
	}
	if !isOwner {
		return false, ErrUserNotFound
	}

	target, err := s.authStore.FindUserByEmailTx(ctx, tx, email)
	if err != nil {
		return false, err
	}
	if target != nil && target.Status == auth.UserStatusInvited {
		if err := s.store.RevokePendingInvitesTx(ctx, tx, target.ID, time.Now().UTC()); err != nil {
			return false, err
		}
	}
	now := time.Now().UTC()
	commitments, err := s.store.CountCohostCommitmentsTx(ctx, tx, eventID, now)
	if err != nil {
		return false, err
	}
	if commitments >= s.cfg.MaxCoHostsPerEvent {
		return false, ErrCohostLimit
	}

	if target == nil {
		target, err = s.authStore.CreateUserTx(ctx, tx, email, "", s.cfg.DefaultTimezone,
			auth.InstanceRoleUser, auth.UserStatusInvited, &persistedActor.ID)
		if err != nil {
			return false, err
		}
	}
	if target.Status == auth.UserStatusActive {
		hasMembership, err := s.store.HasEventMembershipTx(ctx, tx, eventID, target.ID)
		if err != nil {
			return false, err
		}
		if hasMembership {
			return false, ErrUserExists
		}
		if err := s.store.CreateCohostMembershipTx(ctx, tx, eventID, target.ID, persistedActor.ID, now); err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit cohost membership: %w", err)
		}
		htmlBody := `<p>You have been added as a cohost on Owl Invites.</p>`
		plainBody := "You have been added as a cohost on Owl Invites. Sign in to view the event."
		if err := s.sendEmail(ctx, target.Email, "You've been added as an Owl Invites cohost", htmlBody, plainBody); err != nil {
			return false, fmt.Errorf("deliver cohost notification: %w", err)
		}
		return false, nil
	}
	if target.Status != auth.UserStatusInvited {
		return false, ErrUserExists
	}
	rawToken, tokenHash, err := generateToken()
	if err != nil {
		return false, err
	}
	role := "cohost"
	invite, err := s.store.CreateInviteTx(ctx, tx, target, persistedActor.ID, tokenHash, &eventID, &role,
		now.Add(s.cfg.AccountInviteExpiry), now)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit sponsored cohost invite: %w", err)
	}
	htmlBody, plainBody := s.renderInviteEmail(rawToken, invite.ExpiresAt)
	if err := s.sendEmail(ctx, invite.Email, "You're invited to cohost on Owl Invites", htmlBody, plainBody); err != nil {
		return false, fmt.Errorf("deliver sponsored cohost invite: %w", err)
	}
	return true, nil
}

func (s *Service) AcceptInvite(ctx context.Context, rawToken string) (*auth.AuthResponse, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, ErrInvalidInvite
	}
	now := time.Now().UTC()
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin accept account invite: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	invite, err := s.store.FindLiveInviteByHashTx(ctx, tx, tokenDigest(rawToken), now)
	if err != nil {
		return nil, err
	}
	if invite == nil {
		return nil, ErrInvalidInvite
	}
	if err := s.store.ConsumeInviteTx(ctx, tx, invite.ID, now); err != nil {
		return nil, ErrInvalidInvite
	}
	user, err := s.authStore.ActivateInvitedUserTx(ctx, tx, invite.TargetUserID, now)
	if err != nil {
		return nil, ErrInvalidInvite
	}
	if invite.EventID != nil && invite.EventRole != nil && *invite.EventRole == "cohost" {
		if err := s.store.LockEventTx(ctx, tx, *invite.EventID); err != nil {
			return nil, ErrInvalidInvite
		}
		if err := s.store.CreateCohostMembershipTx(ctx, tx, *invite.EventID, user.ID, invite.InvitedByUserID, now); err != nil {
			return nil, ErrInvalidInvite
		}
	}

	sessionToken, sessionHash, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate account invite session: %w", err)
	}
	if _, err := s.authStore.CreateSessionTx(ctx, tx, sessionHash, user.ID, now.Add(s.cfg.SessionExpiry)); err != nil {
		return nil, err
	}
	if err := s.authStore.TouchLastLoginTx(ctx, tx, user.ID, now); err != nil {
		return nil, err
	}
	user.LastLoginAt = &now
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit account invite acceptance: %w", err)
	}
	return &auth.AuthResponse{Token: sessionToken, Organizer: user}, nil
}

func (s *Service) RevokeInvite(ctx context.Context, actor *auth.User, inviteID string) error {
	if actor == nil || !actor.IsAdmin || actor.Status != auth.UserStatusActive {
		return ErrUserNotFound
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	persistedActor, err := s.authStore.FindOrganizerByIDTx(ctx, tx, actor.ID)
	if err != nil {
		return err
	}
	if persistedActor == nil || !persistedActor.IsAdmin || persistedActor.Status != auth.UserStatusActive {
		return ErrUserNotFound
	}
	now := time.Now().UTC()
	invite, err := s.store.RevokeInviteByIDTx(ctx, tx, inviteID, now)
	if err != nil {
		return err
	}
	if err := s.store.InsertAuditTx(ctx, tx, persistedActor.ID, "account_invite_revoked", invite.TargetUserID,
		map[string]any{"inviteId": invite.ID}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account invite revocation: %w", err)
	}
	return nil
}

func (s *Service) SetUserStatus(ctx context.Context, actor *auth.User, userID, status string) error {
	if status != auth.UserStatusActive && status != auth.UserStatusDisabled {
		return ErrInvalidTransition
	}
	return s.changeUser(ctx, actor, userID, func(txCtx *userChange) error {
		if txCtx.target.Status == status {
			return nil
		}
		if txCtx.target.Status == auth.UserStatusInvited && status == auth.UserStatusActive {
			return ErrInvalidTransition
		}
		if txCtx.target.IsAdmin && txCtx.target.Status == auth.UserStatusActive && status == auth.UserStatusDisabled && txCtx.activeAdmins <= 1 {
			return ErrLastAdmin
		}
		if err := s.store.UpdateStatusTx(ctx, txCtx.tx, userID, status, txCtx.now); err != nil {
			return err
		}
		action := "user_enabled"
		if status == auth.UserStatusDisabled {
			action = "user_disabled"
		}
		return s.store.InsertAuditTx(ctx, txCtx.tx, actor.ID, action, userID,
			map[string]any{"from": txCtx.target.Status, "to": status}, txCtx.now)
	})
}

func (s *Service) SetUserRole(ctx context.Context, actor *auth.User, userID, role string) error {
	if role != auth.InstanceRoleAdmin && role != auth.InstanceRoleUser {
		return ErrInvalidTransition
	}
	return s.changeUser(ctx, actor, userID, func(txCtx *userChange) error {
		if txCtx.target.InstanceRole == role {
			return nil
		}
		if txCtx.target.IsAdmin && txCtx.target.Status == auth.UserStatusActive && role != auth.InstanceRoleAdmin && txCtx.activeAdmins <= 1 {
			return ErrLastAdmin
		}
		if err := s.store.UpdateRoleTx(ctx, txCtx.tx, userID, role, txCtx.now); err != nil {
			return err
		}
		return s.store.InsertAuditTx(ctx, txCtx.tx, actor.ID, "instance_role_changed", userID,
			map[string]any{"from": txCtx.target.InstanceRole, "to": role}, txCtx.now)
	})
}

type userChange struct {
	tx           database.Tx
	target       *auth.User
	activeAdmins int
	now          time.Time
}

func (s *Service) changeUser(ctx context.Context, actor *auth.User, userID string, apply func(*userChange) error) error {
	if actor == nil || !actor.IsAdmin || actor.Status != auth.UserStatusActive {
		return ErrUserNotFound
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.store.LockAdminMutationTx(ctx, tx); err != nil {
		return err
	}
	persistedActor, err := s.authStore.FindOrganizerByIDTx(ctx, tx, actor.ID)
	if err != nil {
		return err
	}
	if persistedActor == nil || !persistedActor.IsAdmin || persistedActor.Status != auth.UserStatusActive {
		return ErrUserNotFound
	}
	target, err := s.authStore.FindOrganizerByIDTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if target == nil {
		return ErrUserNotFound
	}
	count, err := s.store.CountActiveAdminsTx(ctx, tx)
	if err != nil {
		return err
	}
	change := &userChange{tx: tx, target: target, activeAdmins: count, now: time.Now().UTC()}
	if err := apply(change); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user change: %w", err)
	}
	return nil
}

func (s *Service) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListAudit(ctx, limit)
}

func (s *Service) renderInviteEmail(rawToken string, expiresAt time.Time) (string, string) {
	// Put the capability in the fragment so it is not sent in the initial HTTP
	// request or access logs. The SPA exchanges it via the strict JSON endpoint.
	url := strings.TrimRight(s.cfg.BaseURL, "/") + "/auth/accept-invite#token=" + rawToken
	instanceName := strings.TrimSpace(s.cfg.InstanceName)
	const legacyDefaultInstanceName = "OpenRSVP"
	if instanceName == "" || instanceName == legacyDefaultInstanceName {
		instanceName = "Owl Invites"
	}
	htmlBody := fmt.Sprintf(`<p>You have been invited to manage events on %s.</p><p><a href="%s">Accept invitation</a></p><p>This invitation expires at %s.</p>`,
		html.EscapeString(instanceName), html.EscapeString(url), expiresAt.UTC().Format(time.RFC3339))
	plainBody := fmt.Sprintf("You have been invited to manage events on %s.\n\nAccept invitation:\n%s\n\nThis invitation expires at %s.",
		instanceName, url, expiresAt.UTC().Format(time.RFC3339))
	return htmlBody, plainBody
}

func generateToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	return raw, tokenDigest(raw), nil
}

func tokenDigest(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

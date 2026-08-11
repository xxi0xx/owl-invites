package instanceconfig

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/config"
)

var (
	ErrBootstrapUnauthorized = errors.New("bootstrap authorization failed")
	ErrSetupComplete         = errors.New("setup is already complete")
)

type BootstrapResult struct {
	SessionToken string     `json:"-"`
	User         *auth.User `json:"user"`
}

// BootstrapService owns the one-time transition from setup-required mode to
// a configured instance with its first persistent administrator and session.
type BootstrapService struct {
	store     *Store
	authStore *auth.Store
	cfg       *config.Config
}

func NewBootstrapService(store *Store, authStore *auth.Store, cfg *config.Config) *BootstrapService {
	return &BootstrapService{store: store, authStore: authStore, cfg: cfg}
}

func (s *BootstrapService) Bootstrap(ctx context.Context, in *BootstrapRequest) (*BootstrapResult, error) {
	if !constantTimeTokenEqual(in.BootstrapToken, s.cfg.BootstrapToken) || s.cfg.BootstrapToken == "" {
		return nil, ErrBootstrapUnauthorized
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	claimed, err := s.store.ClaimBootstrapTx(ctx, tx, in, now)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrSetupComplete
	}

	email := strings.ToLower(strings.TrimSpace(in.AdminEmail))
	user, err := s.authStore.FindUserByEmailTx(ctx, tx, email)
	if err != nil {
		return nil, fmt.Errorf("find bootstrap user: %w", err)
	}
	if user == nil {
		user, err = s.authStore.CreateUserTx(ctx, tx, email, in.AdminName,
			in.DefaultTimezone, auth.InstanceRoleAdmin, auth.UserStatusActive, nil)
	} else {
		user, err = s.authStore.PromoteBootstrapAdminTx(ctx, tx, user.ID,
			in.AdminName, in.DefaultTimezone, now)
	}
	if err != nil {
		return nil, err
	}

	rawSession, sessionHash, err := newToken()
	if err != nil {
		return nil, fmt.Errorf("generate bootstrap session: %w", err)
	}
	if _, err := s.authStore.CreateSessionTx(ctx, tx, sessionHash, user.ID, now.Add(s.cfg.SessionExpiry)); err != nil {
		return nil, err
	}
	if err := s.authStore.TouchLastLoginTx(ctx, tx, user.ID, now); err != nil {
		return nil, err
	}
	user.LastLoginAt = &now

	if err := s.store.InsertAuditTx(ctx, tx, &user.ID, "user", "bootstrap_completed", &user.ID, nil,
		map[string]any{"instanceRole": auth.InstanceRoleAdmin}, now); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit bootstrap: %w", err)
	}
	return &BootstrapResult{SessionToken: rawSession, User: user}, nil
}

func constantTimeTokenEqual(provided, expected string) bool {
	providedHash := sha256.Sum256([]byte(provided))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
}

func newToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	digest := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(digest[:]), nil
}

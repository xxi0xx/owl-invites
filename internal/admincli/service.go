// Package admincli implements host-side break-glass administration.
package admincli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/instanceconfig"
)

var ErrUserNotFound = errors.New("user not found")

type Service struct {
	authStore     *auth.Store
	instanceStore *instanceconfig.Store
}

func NewService(authStore *auth.Store, instanceStore *instanceconfig.Store) *Service {
	return &Service{authStore: authStore, instanceStore: instanceStore}
}

// PromoteAdmin activates an existing user and grants the persistent instance
// admin role. It does not reopen or reuse the one-time bootstrap endpoint.
func (s *Service) PromoteAdmin(ctx context.Context, email string) (*auth.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrUserNotFound
	}
	tx, err := s.instanceStore.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin emergency role recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize with API-driven role changes through the singleton row.
	if _, err := tx.ExecContext(ctx, "UPDATE instances SET updated_at = updated_at WHERE id = 'default'"); err != nil {
		return nil, fmt.Errorf("lock emergency role recovery: %w", err)
	}
	user, err := s.authStore.FindUserByEmailTx(ctx, tx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	now := time.Now().UTC()
	previousRole, previousStatus := user.InstanceRole, user.Status
	user, err = s.authStore.PromoteBootstrapAdminTx(ctx, tx, user.ID, user.Name, user.Timezone, now)
	if err != nil {
		return nil, err
	}
	if err := s.instanceStore.InsertAuditTx(ctx, tx, nil, "cli", "emergency_role_recovery", &user.ID, nil,
		map[string]any{"fromRole": previousRole, "fromStatus": previousStatus, "toRole": auth.InstanceRoleAdmin}, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit emergency role recovery: %w", err)
	}
	return user, nil
}

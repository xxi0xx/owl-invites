// Package eventadmin implements explicit, audited event recovery operations.
package eventadmin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/xxi0xx/owl-invites/internal/auth"
	"github.com/xxi0xx/owl-invites/internal/database"
	"github.com/xxi0xx/owl-invites/internal/instanceconfig"
)

var (
	ErrEventNotFound = errors.New("event not found")
	ErrUserNotFound  = errors.New("eligible user not found")
	ErrForbidden     = errors.New("administrator authorization required")
)

type Service struct {
	db            database.DB
	authStore     *auth.Store
	instanceStore *instanceconfig.Store
}

func NewService(db database.DB, authStore *auth.Store, instanceStore *instanceconfig.Store) *Service {
	return &Service{db: db, authStore: authStore, instanceStore: instanceStore}
}

func (s *Service) AddSelf(ctx context.Context, actor *auth.User, eventID string) error {
	tx, persistedActor, now, err := s.beginAuthorized(ctx, actor, eventID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var existingRole string
	err = tx.QueryRowContext(ctx, "SELECT role FROM event_memberships WHERE event_id = ? AND user_id = ?", eventID, persistedActor.ID).Scan(&existingRole)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("find administrator event membership: %w", err)
	}
	if err == sql.ErrNoRows {
		_, err = tx.ExecContext(ctx, `INSERT INTO event_memberships (
			id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, 'cohost', ?, ?, ?)`, uuid.Must(uuid.NewV7()).String(),
			eventID, persistedActor.ID, persistedActor.ID, now.Format(time.RFC3339), now.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("add administrator event membership: %w", err)
		}
		if err := s.instanceStore.InsertAuditTx(ctx, tx, &persistedActor.ID, "user", "administrator_joined_event", &persistedActor.ID, &eventID,
			map[string]any{"role": "cohost"}, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit administrator event membership: %w", err)
	}
	return nil
}

func (s *Service) TransferOwnership(ctx context.Context, actor *auth.User, eventID, newOwnerUserID string) error {
	tx, persistedActor, now, err := s.beginAuthorized(ctx, actor, eventID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	newOwner, err := s.authStore.FindOrganizerByIDTx(ctx, tx, newOwnerUserID)
	if err != nil {
		return err
	}
	if newOwner == nil || newOwner.Status != auth.UserStatusActive {
		return ErrUserNotFound
	}
	var oldOwnerUserID string
	if err := tx.QueryRowContext(ctx, "SELECT user_id FROM event_memberships WHERE event_id = ? AND role = 'owner'", eventID).Scan(&oldOwnerUserID); err != nil {
		if err == sql.ErrNoRows {
			return ErrEventNotFound
		}
		return fmt.Errorf("find current event owner: %w", err)
	}
	if oldOwnerUserID == newOwnerUserID {
		return tx.Commit()
	}

	nowText := now.Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE event_memberships SET role = 'cohost',
		granted_by_user_id = ?, updated_at = ? WHERE event_id = ? AND user_id = ? AND role = 'owner'`,
		persistedActor.ID, nowText, eventID, oldOwnerUserID); err != nil {
		return fmt.Errorf("demote previous event owner: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE event_memberships SET role = 'owner',
		granted_by_user_id = ?, updated_at = ? WHERE event_id = ? AND user_id = ?`,
		persistedActor.ID, nowText, eventID, newOwnerUserID)
	if err != nil {
		return fmt.Errorf("promote event owner membership: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO event_memberships (
			id, event_id, user_id, role, granted_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, 'owner', ?, ?, ?)`, uuid.Must(uuid.NewV7()).String(), eventID,
			newOwnerUserID, persistedActor.ID, nowText, nowText); err != nil {
			return fmt.Errorf("create new owner membership: %w", err)
		}
	}
	if err := s.instanceStore.InsertAuditTx(ctx, tx, &persistedActor.ID, "user", "event_ownership_transferred", &newOwnerUserID, &eventID,
		map[string]any{"fromUserId": oldOwnerUserID, "toUserId": newOwnerUserID}, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit event ownership transfer: %w", err)
	}
	return nil
}

func (s *Service) beginAuthorized(ctx context.Context, actor *auth.User, eventID string) (database.Tx, *auth.User, time.Time, error) {
	if actor == nil {
		return nil, nil, time.Time{}, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	if result, lockErr := tx.ExecContext(ctx, "UPDATE events SET updated_at = updated_at WHERE id = ?", eventID); lockErr != nil {
		_ = tx.Rollback()
		return nil, nil, time.Time{}, lockErr
	} else if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		_ = tx.Rollback()
		return nil, nil, time.Time{}, ErrEventNotFound
	}
	persistedActor, err := s.authStore.FindOrganizerByIDTx(ctx, tx, actor.ID)
	if err != nil {
		_ = tx.Rollback()
		return nil, nil, time.Time{}, err
	}
	if persistedActor == nil || !persistedActor.IsAdmin || persistedActor.Status != auth.UserStatusActive {
		_ = tx.Rollback()
		return nil, nil, time.Time{}, ErrForbidden
	}
	return tx, persistedActor, time.Now().UTC(), nil
}

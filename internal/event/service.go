package event

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/yannkr/openrsvp/internal/errcode"
)

// Field length limits.
const (
	maxTitleLen       = 200
	maxDescriptionLen = 5000
	maxLocationLen    = 500
)

// Service contains the business logic for event management.
type Service struct {
	store            *Store
	cohostStore      *CoHostStore
	defaultRetention int
	onPublish        func(ctx context.Context, e *Event)
	onDuplicate      func(ctx context.Context, srcEventID, newEventID string)
	onCancel         func(ctx context.Context, e *Event)
}

// NewService creates a new event Service.
func NewService(store *Store, defaultRetentionDays int) *Service {
	return &Service{
		store:            store,
		defaultRetention: defaultRetentionDays,
	}
}

// SetOnPublish registers a callback that is invoked after an event is
// successfully published. This is used to create default reminders.
func (s *Service) SetOnPublish(fn func(ctx context.Context, e *Event)) {
	s.onPublish = fn
}

// SetOnDuplicate registers a callback that is invoked after an event is
// successfully duplicated. This is used to copy the invite card design.
func (s *Service) SetOnDuplicate(fn func(ctx context.Context, srcEventID, newEventID string)) {
	s.onDuplicate = fn
}

// SetOnCancel registers a callback that is invoked after an event is
// cancelled when the organizer requests invitation-household notification.
func (s *Service) SetOnCancel(fn func(ctx context.Context, e *Event)) {
	s.onCancel = fn
}

// SetCoHostStore sets the co-host store on the service, enabling co-host
// authorization checks.
func (s *Service) SetCoHostStore(cs *CoHostStore) {
	s.cohostStore = cs
}

// CanManageEvent checks whether the given organizer can manage the event.
// Returns true if the organizer is the owner or a co-host.
func (s *Service) CanManageEvent(ctx context.Context, eventID, organizerID string) (bool, error) {
	ev, err := s.store.FindByID(ctx, eventID)
	if err != nil || ev == nil {
		return false, err
	}
	role, err := s.store.FindMembershipRole(ctx, eventID, organizerID)
	if err != nil {
		return false, err
	}
	return role == "owner" || role == "cohost", nil
}

// IsEventOwner checks whether the given organizer is the owner (not co-host) of
// the event.
func (s *Service) IsEventOwner(ctx context.Context, eventID, organizerID string) (bool, error) {
	ev, err := s.store.FindByID(ctx, eventID)
	if err != nil || ev == nil {
		return false, err
	}
	role, err := s.store.FindMembershipRole(ctx, eventID, organizerID)
	if err != nil {
		return false, err
	}
	return role == "owner", nil
}

// Create validates the request and creates a new event for the given organizer.
func (s *Service) Create(ctx context.Context, organizerID string, req CreateEventRequest) (*Event, error) {
	if req.Title == "" {
		return nil, errcode.Validationf("title is required")
	}
	if len(req.Title) > maxTitleLen {
		return nil, errcode.Validationf("title must be %d characters or less", maxTitleLen)
	}
	if len(req.Description) > maxDescriptionLen {
		return nil, errcode.Validationf("description must be %d characters or less", maxDescriptionLen)
	}
	if len(req.Location) > maxLocationLen {
		return nil, errcode.Validationf("location must be %d characters or less", maxLocationLen)
	}
	if req.EventDate == "" {
		return nil, errcode.Validationf("eventDate is required")
	}

	eventDate, err := parseFlexibleTime(req.EventDate)
	if err != nil {
		return nil, fmt.Errorf("invalid eventDate format: %w", err)
	}

	var endDate *time.Time
	if req.EndDate != nil && *req.EndDate != "" {
		t, err := parseFlexibleTime(*req.EndDate)
		if err != nil {
			return nil, fmt.Errorf("invalid endDate format: %w", err)
		}
		endDate = &t
	}

	if req.Timezone == "" {
		req.Timezone = "America/New_York"
	}

	retentionDays := s.defaultRetention
	if req.RetentionDays != nil && *req.RetentionDays > 0 {
		retentionDays = *req.RetentionDays
	}

	showHeadcount := false
	if req.ShowHeadcount != nil {
		showHeadcount = *req.ShowHeadcount
	}
	showGuestList := false
	if req.ShowGuestList != nil {
		showGuestList = *req.ShowGuestList
	}

	var rsvpDeadline *time.Time
	if req.RSVPDeadline != nil && *req.RSVPDeadline != "" {
		deadline, err := parseFlexibleTime(*req.RSVPDeadline)
		if err != nil {
			return nil, fmt.Errorf("invalid rsvpDeadline format: %w", err)
		}
		if deadline.After(eventDate) {
			return nil, errcode.Validationf("RSVP deadline must be on or before the event date")
		}
		rsvpDeadline = &deadline
	}

	e := &Event{
		ID:            uuid.Must(uuid.NewV7()).String(),
		OrganizerID:   organizerID,
		Title:         req.Title,
		Description:   req.Description,
		EventDate:     eventDate,
		EndDate:       endDate,
		Location:      req.Location,
		Timezone:      req.Timezone,
		RetentionDays: retentionDays,
		ShowHeadcount: showHeadcount,
		ShowGuestList: showGuestList,
		RSVPDeadline:  rsvpDeadline,
		Status:        "draft",
	}

	if err := s.store.Create(ctx, e); err != nil {
		return nil, err
	}

	return e, nil
}

// GetByID retrieves an event by its ID.
func (s *Service) GetByID(ctx context.Context, id string) (*Event, error) {
	e, err := s.store.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("event not found")
	}
	return e, nil
}

// ListByOrganizer retrieves all events belonging to the given organizer,
// including events where the organizer is a co-host.
func (s *Service) ListByOrganizer(ctx context.Context, organizerID string) ([]*Event, error) {
	events, err := s.store.FindByOrganizerID(ctx, organizerID)
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []*Event{}
	}
	return events, nil
}

// Update applies partial updates to an event. The event owner or a co-host can
// update.
func (s *Service) Update(ctx context.Context, eventID, organizerID string, req UpdateEventRequest) (*Event, error) {
	e, err := s.store.FindByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("event not found")
	}

	canManage, err := s.CanManageEvent(ctx, eventID, organizerID)
	if err != nil {
		return nil, err
	}
	if !canManage {
		return nil, fmt.Errorf("forbidden: you do not own this event")
	}

	if req.Title != nil {
		if len(*req.Title) > maxTitleLen {
			return nil, errcode.Validationf("title must be %d characters or less", maxTitleLen)
		}
		e.Title = *req.Title
	}
	if req.Description != nil {
		if len(*req.Description) > maxDescriptionLen {
			return nil, errcode.Validationf("description must be %d characters or less", maxDescriptionLen)
		}
		e.Description = *req.Description
	}
	if req.EventDate != nil {
		t, err := parseFlexibleTime(*req.EventDate)
		if err != nil {
			return nil, fmt.Errorf("invalid eventDate format: %w", err)
		}
		e.EventDate = t
	}
	if req.EndDate != nil {
		if *req.EndDate == "" {
			e.EndDate = nil
		} else {
			t, err := parseFlexibleTime(*req.EndDate)
			if err != nil {
				return nil, fmt.Errorf("invalid endDate format: %w", err)
			}
			e.EndDate = &t
		}
	}
	if req.Location != nil {
		if len(*req.Location) > maxLocationLen {
			return nil, errcode.Validationf("location must be %d characters or less", maxLocationLen)
		}
		e.Location = *req.Location
	}
	if req.Timezone != nil {
		e.Timezone = *req.Timezone
	}
	if req.RetentionDays != nil {
		e.RetentionDays = *req.RetentionDays
	}
	if req.ShowHeadcount != nil {
		e.ShowHeadcount = *req.ShowHeadcount
	}
	if req.ShowGuestList != nil {
		e.ShowGuestList = *req.ShowGuestList
	}
	if req.RSVPDeadline != nil {
		if *req.RSVPDeadline == "" {
			e.RSVPDeadline = nil
		} else {
			deadline, err := parseFlexibleTime(*req.RSVPDeadline)
			if err != nil {
				return nil, fmt.Errorf("invalid rsvpDeadline format: %w", err)
			}
			if deadline.After(e.EventDate) {
				return nil, errcode.Validationf("RSVP deadline must be on or before the event date")
			}
			e.RSVPDeadline = &deadline
		}
	}
	if err := s.store.Update(ctx, e); err != nil {
		return nil, err
	}

	return e, nil
}

// Publish transitions an event from draft to published status. The event owner
// or a co-host can publish.
func (s *Service) Publish(ctx context.Context, eventID, organizerID string) (*Event, error) {
	e, err := s.store.FindByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("event not found")
	}

	canManage, err := s.CanManageEvent(ctx, eventID, organizerID)
	if err != nil {
		return nil, err
	}
	if !canManage {
		return nil, fmt.Errorf("forbidden: you do not own this event")
	}
	if e.Status != "draft" {
		return nil, errcode.Validationf("event can only be published from draft status, current status: %s", e.Status)
	}

	e.Status = "published"
	if err := s.store.Update(ctx, e); err != nil {
		return nil, err
	}

	if s.onPublish != nil {
		s.onPublish(ctx, e)
	}

	return e, nil
}

// Cancel transitions an event from published to cancelled status. The event
// owner or a co-host can cancel.
// When notifyInvitees is true, the onCancel callback is invoked to
// send cancellation notifications.
func (s *Service) Cancel(ctx context.Context, eventID, organizerID string, notifyInvitees bool) (*Event, error) {
	e, err := s.store.FindByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("event not found")
	}

	canManage, err := s.CanManageEvent(ctx, eventID, organizerID)
	if err != nil {
		return nil, err
	}
	if !canManage {
		return nil, fmt.Errorf("forbidden: you do not own this event")
	}
	if e.Status != "published" {
		return nil, errcode.Validationf("event can only be cancelled from published status, current status: %s", e.Status)
	}

	e.Status = "cancelled"
	if err := s.store.Update(ctx, e); err != nil {
		return nil, err
	}

	if notifyInvitees && s.onCancel != nil {
		s.onCancel(ctx, e)
	}

	return e, nil
}

// Reopen transitions a cancelled event back to draft status. The event owner
// or a co-host can reopen.
func (s *Service) Reopen(ctx context.Context, eventID, organizerID string) (*Event, error) {
	e, err := s.store.FindByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("event not found")
	}

	canManage, err := s.CanManageEvent(ctx, eventID, organizerID)
	if err != nil {
		return nil, err
	}
	if !canManage {
		return nil, fmt.Errorf("forbidden: you do not own this event")
	}
	if e.Status != "cancelled" {
		return nil, errcode.Validationf("event can only be reopened from cancelled status, current status: %s", e.Status)
	}

	e.Status = "draft"
	if err := s.store.Update(ctx, e); err != nil {
		return nil, err
	}

	return e, nil
}

// Duplicate creates a draft copy. Invitations and reminders are not copied.
func (s *Service) Duplicate(ctx context.Context, eventID, organizerID string) (*Event, error) {
	e, err := s.store.FindByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("event not found")
	}
	isOwner, err := s.IsEventOwner(ctx, eventID, organizerID)
	if err != nil {
		return nil, err
	}
	if !isOwner {
		return nil, fmt.Errorf("forbidden: you do not own this event")
	}

	newEvent := &Event{
		ID:            uuid.Must(uuid.NewV7()).String(),
		OrganizerID:   organizerID,
		Title:         "Copy of " + e.Title,
		Description:   e.Description,
		EventDate:     e.EventDate,
		EndDate:       e.EndDate,
		Location:      e.Location,
		Timezone:      e.Timezone,
		RetentionDays: e.RetentionDays,
		ShowHeadcount: e.ShowHeadcount,
		ShowGuestList: e.ShowGuestList,
		RSVPDeadline:  e.RSVPDeadline,
		Status:        "draft",
	}

	if err := s.store.Create(ctx, newEvent); err != nil {
		return nil, err
	}

	if s.onDuplicate != nil {
		s.onDuplicate(ctx, eventID, newEvent.ID)
	}

	return newEvent, nil
}

// Delete performs a soft delete by setting the event status to archived.
// Only the event owner can delete.
func (s *Service) Delete(ctx context.Context, eventID, organizerID string) error {
	e, err := s.store.FindByID(ctx, eventID)
	if err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("event not found")
	}
	isOwner, err := s.IsEventOwner(ctx, eventID, organizerID)
	if err != nil {
		return err
	}
	if !isOwner {
		return fmt.Errorf("forbidden: you do not own this event")
	}

	e.Status = "archived"
	return s.store.Update(ctx, e)
}

// CreateFromSeries persists an event that was generated by a series template.
// It bypasses the normal Create() validation flow since the series service
// constructs a fully-formed Event struct.
func (s *Service) CreateFromSeries(ctx context.Context, event *Event) error {
	return s.store.Create(ctx, event)
}

// parseFlexibleTime tries RFC3339 first, then falls back to common datetime
// formats produced by HTML datetime-local inputs (e.g. "2026-03-15T14:00").
func parseFlexibleTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, errcode.Validationf("unrecognized datetime format: %s", s)
}

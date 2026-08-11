package invitation

import (
	"context"
	"strings"

	"github.com/xxi0xx/owl-invites/internal/errcode"
)

type InvitationListFilter struct {
	Search     string
	Response   string
	Attendance string
}

func (s *Service) ListOrganizerHouseholds(ctx context.Context, eventID string, filter InvitationListFilter) ([]*Household, error) {
	filter.Search = strings.ToLower(strings.TrimSpace(filter.Search))
	if len(filter.Search) > 200 {
		return nil, errcode.Validationf("search must be 200 characters or fewer")
	}
	if filter.Response != "" && filter.Response != "submitted" && filter.Response != "not_submitted" {
		return nil, errcode.Validationf("invalid response filter")
	}
	if filter.Attendance != "" && !validAttendance(filter.Attendance) {
		return nil, errcode.Validationf("invalid attendance filter")
	}
	items, err := s.store.ListByEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	result := make([]*Household, 0, len(items))
	for _, household := range items {
		if !matchesOrganizerFilter(household, filter) {
			continue
		}
		result = append(result, household)
	}
	return result, nil
}

func matchesOrganizerFilter(household *Household, filter InvitationListFilter) bool {
	if filter.Response == "submitted" && household.Response.SubmittedAt == nil {
		return false
	}
	if filter.Response == "not_submitted" && household.Response.SubmittedAt != nil {
		return false
	}
	if filter.Attendance != "" {
		matched := false
		for _, guest := range household.Guests {
			if guest.Attendance == filter.Attendance {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if filter.Search == "" {
		return true
	}
	values := []string{household.Invitation.Label, stringValue(household.Invitation.ContactEmail), stringValue(household.Invitation.ContactPhone)}
	for _, guest := range household.Guests {
		values = append(values, guest.Name)
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), filter.Search) {
			return true
		}
	}
	return false
}

func (s *Service) UpdateInvitation(ctx context.Context, eventID, invitationID string, req UpdateInvitationRequest) (*Household, error) {
	req.Label = strings.TrimSpace(req.Label)
	if req.Label == "" || len(req.Label) > 200 {
		return nil, errcode.Validationf("label is required and must be 200 characters or fewer")
	}
	if req.AdditionalGuestAllowance < 0 || req.AdditionalGuestAllowance > 50 {
		return nil, errcode.Validationf("additional guest allowance must be between 0 and 50")
	}
	if len(req.AssignedGuests) == 0 || len(req.AssignedGuests) > MaxHouseholdGuests {
		return nil, errcode.Validationf("between 1 and %d assigned guests are required", MaxHouseholdGuests)
	}
	email, phone, method, err := validateContact(req.ContactEmail, req.ContactPhone, req.PreferredDeliveryMethod)
	if err != nil {
		return nil, err
	}
	req.ContactEmail, req.ContactPhone, req.PreferredDeliveryMethod = email, phone, method
	seenIDs := make(map[string]bool)
	for index := range req.AssignedGuests {
		guest := &req.AssignedGuests[index]
		guest.ID = strings.TrimSpace(guest.ID)
		guest.Name = strings.TrimSpace(guest.Name)
		if guest.Name == "" || len(guest.Name) > 200 {
			return nil, errcode.Validationf("assigned guest %d requires a name of 200 characters or fewer", index+1)
		}
		if guest.ID != "" {
			if seenIDs[guest.ID] {
				return nil, errcode.Validationf("assigned guest appears more than once")
			}
			seenIDs[guest.ID] = true
		}
	}
	if err := s.store.UpdateInvitation(ctx, eventID, invitationID, req); err != nil {
		return nil, err
	}
	return s.store.LoadOrganizerHousehold(ctx, invitationID)
}

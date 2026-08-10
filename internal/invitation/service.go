package invitation

import (
	"context"
	"fmt"
	"html"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yannkr/openrsvp/internal/errcode"
)

type EmailSender func(ctx context.Context, eventID, invitationID, to, subject, htmlBody, plainBody string) error

type Service struct {
	store          *Store
	signer         *CapabilitySigner
	baseURL        string
	sessionExpiry  time.Duration
	recoveryExpiry time.Duration
	sendEmail      EmailSender
}

func NewService(store *Store, secretKey, baseURL string, sessionExpiry, recoveryExpiry time.Duration) (*Service, error) {
	signer, err := NewCapabilitySigner(secretKey)
	if err != nil {
		return nil, err
	}
	if sessionExpiry <= 0 {
		return nil, fmt.Errorf("invitation session expiry must be positive")
	}
	if recoveryExpiry <= 0 {
		return nil, fmt.Errorf("invitation recovery expiry must be positive")
	}
	return &Service{
		store: store, signer: signer, baseURL: strings.TrimRight(baseURL, "/"),
		sessionExpiry: sessionExpiry, recoveryExpiry: recoveryExpiry,
	}, nil
}

func (s *Service) SetEmailSender(sender EmailSender) { s.sendEmail = sender }

func (s *Service) CreatePrivate(ctx context.Context, eventID, creatorUserID string, req CreateRequest) (*CreateResult, error) {
	label := strings.TrimSpace(req.Label)
	if label == "" {
		return nil, errcode.Validationf("label is required")
	}
	if len(label) > 200 {
		return nil, errcode.Validationf("label must be 200 characters or fewer")
	}
	if req.AdditionalGuestAllowance < 0 || req.AdditionalGuestAllowance > 50 {
		return nil, errcode.Validationf("additional guest allowance must be between 0 and 50")
	}
	if len(req.AssignedGuestNames) == 0 || len(req.AssignedGuestNames) > 100 {
		return nil, errcode.Validationf("at least one assigned guest is required")
	}
	email, phone, method, err := validateContact(req.ContactEmail, req.ContactPhone, req.PreferredDeliveryMethod)
	if err != nil {
		return nil, err
	}
	if req.Send && (method != "email" || email == nil) {
		return nil, errcode.Validationf("send requires email delivery and a valid contact email")
	}
	accessID, err := randomToken(18)
	if err != nil {
		return nil, err
	}
	creator := creatorUserID
	inv := &Invitation{
		ID: uuid.Must(uuid.NewV7()).String(), EventID: eventID, Label: label,
		ContactEmail: email, ContactPhone: phone, PreferredDeliveryMethod: method,
		AdditionalGuestAllowance: req.AdditionalGuestAllowance, Source: SourcePrivate,
		AccessID: accessID, TokenVersion: 1, CreatedByUserID: &creator,
	}
	guests := make([]*Guest, 0, len(req.AssignedGuestNames))
	for i, rawName := range req.AssignedGuestNames {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, errcode.Validationf("assigned guest names cannot be empty")
		}
		if len(name) > 200 {
			return nil, errcode.Validationf("assigned guest names must be 200 characters or fewer")
		}
		guests = append(guests, &Guest{ID: uuid.Must(uuid.NewV7()).String(), Name: name,
			Origin: GuestOriginAssigned, SortOrder: i})
	}
	if err := s.store.Create(ctx, inv, guests); err != nil {
		return nil, err
	}
	result := &CreateResult{Invitation: inv, Guests: guests, AccessURL: s.privateAccessURL(inv),
		Delivery: DeliveryResult{Status: DeliveryNotRequested}}
	if req.Send {
		result.Delivery = s.attemptDelivery(ctx, inv.ID,
			"Invitation created, but email delivery failed. Use the private link or retry delivery.")
	}
	return result, nil
}

func (s *Service) Deliver(ctx context.Context, invitationID string) error {
	inv, err := s.store.FindByID(ctx, invitationID)
	if err != nil {
		return err
	}
	if inv == nil || inv.RevokedAt != nil {
		return ErrNotFound
	}
	if inv.PreferredDeliveryMethod != "email" || inv.ContactEmail == nil {
		return errcode.Validationf("email delivery is not available for this invitation")
	}
	if s.sendEmail == nil {
		return fmt.Errorf("invitation email delivery is not configured")
	}
	household, err := s.store.LoadHousehold(ctx, inv.ID)
	if err != nil {
		return err
	}
	url := s.privateAccessURL(inv)
	subject := "You're invited — " + household.Event.Title
	plain := fmt.Sprintf("You have been invited to %s.\n\nOpen your private invitation:\n%s\n\nDo not forward this private link.", household.Event.Title, url)
	htmlBody := fmt.Sprintf(`<p>You have been invited to <strong>%s</strong>.</p><p><a href="%s">Open your private invitation</a></p><p>Do not forward this private link.</p>`, html.EscapeString(household.Event.Title), html.EscapeString(url))
	return s.sendEmail(ctx, inv.EventID, inv.ID, *inv.ContactEmail, subject, htmlBody, plain)
}

func (s *Service) attemptDelivery(ctx context.Context, invitationID, warning string) DeliveryResult {
	if err := s.Deliver(ctx, invitationID); err != nil {
		return DeliveryResult{Status: DeliveryFailed, Warning: warning, err: err}
	}
	return DeliveryResult{Status: DeliverySent}
}

func (s *Service) Broadcast(ctx context.Context, eventID string, senderUserID *string, req MessageRequest) (int, error) {
	req.RecipientGroup = strings.TrimSpace(req.RecipientGroup)
	if req.RecipientGroup == "" {
		req.RecipientGroup = "all"
	}
	if !validAttendance(req.RecipientGroup) && req.RecipientGroup != "all" {
		return 0, errcode.Validationf("invalid recipient group")
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Body = strings.TrimSpace(req.Body)
	if req.Subject == "" || len(req.Subject) > 200 {
		return 0, errcode.Validationf("subject is required and must be 200 characters or fewer")
	}
	if req.Body == "" || len(req.Body) > 10000 {
		return 0, errcode.Validationf("body is required and must be 10000 characters or fewer")
	}
	if s.sendEmail == nil {
		return 0, fmt.Errorf("invitation email delivery is not configured")
	}
	targets, err := s.store.ListDeliveryTargets(ctx, eventID, req.RecipientGroup)
	if err != nil {
		return 0, err
	}
	message := &InvitationMessage{EventID: eventID, SenderUserID: senderUserID,
		RecipientGroup: req.RecipientGroup, Subject: req.Subject, Body: req.Body}
	if err := s.store.CreateMessage(ctx, message); err != nil {
		return 0, err
	}
	sent := 0
	for _, inv := range targets {
		if inv.PreferredDeliveryMethod != "email" || inv.ContactEmail == nil {
			continue
		}
		url := s.privateAccessURL(inv)
		plain := req.Body + "\n\nManage your private invitation:\n" + url
		htmlBody := fmt.Sprintf(`<p>%s</p><p><a href="%s">Manage your private invitation</a></p>`,
			html.EscapeString(req.Body), html.EscapeString(url))
		if err := s.sendEmail(ctx, eventID, inv.ID, *inv.ContactEmail, req.Subject, htmlBody, plain); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func (s *Service) Rotate(ctx context.Context, eventID, invitationID string) (*CreateResult, error) {
	inv, err := s.store.Rotate(ctx, invitationID, eventID)
	if err != nil {
		return nil, err
	}
	household, err := s.store.LoadHousehold(ctx, invitationID)
	if err != nil {
		return nil, err
	}
	return &CreateResult{Invitation: inv, Guests: household.Guests, AccessURL: s.privateAccessURL(inv),
		Delivery: DeliveryResult{Status: DeliveryNotRequested}}, nil
}

func (s *Service) Revoke(ctx context.Context, eventID, invitationID, reason string) error {
	reason = strings.TrimSpace(reason)
	if len(reason) > 500 {
		return errcode.Validationf("revocation reason must be 500 characters or fewer")
	}
	return s.store.Revoke(ctx, invitationID, eventID, reason)
}

func (s *Service) ExchangePrivate(ctx context.Context, rawCapability string) (string, *Household, error) {
	accessID, version, err := s.signer.ParsePrivate(rawCapability)
	if err != nil {
		return "", nil, ErrInvalidCapability
	}
	inv, err := s.store.FindByAccessID(ctx, accessID)
	if err != nil {
		return "", nil, err
	}
	// Source records how the household was allocated. Every invitation has the
	// same durable household capability; the separate open-enrollment HMAC
	// domain can only create a new invitation and cannot pass ParsePrivate.
	if inv == nil || inv.RevokedAt != nil || inv.TokenVersion != version {
		return "", nil, ErrInvalidCapability
	}
	rawSession, err := randomToken(32)
	if err != nil {
		return "", nil, err
	}
	if err := s.store.CreateSession(ctx, inv.ID, hashToken(rawSession), version, time.Now().UTC().Add(s.sessionExpiry)); err != nil {
		return "", nil, err
	}
	household, err := s.store.LoadHousehold(ctx, inv.ID)
	if err != nil {
		return "", nil, err
	}
	return rawSession, household, nil
}

func (s *Service) HouseholdForSession(ctx context.Context, rawSession string) (*Household, error) {
	if rawSession == "" {
		return nil, ErrInvalidCapability
	}
	inv, err := s.store.InvitationForSession(ctx, hashToken(rawSession))
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, ErrInvalidCapability
	}
	return s.store.LoadHousehold(ctx, inv.ID)
}

func (s *Service) SubmitForSession(ctx context.Context, rawSession string, req SubmitRequest) (*Household, error) {
	inv, err := s.store.InvitationForSession(ctx, hashToken(rawSession))
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, ErrInvalidCapability
	}
	if req.Version < 1 {
		return nil, errcode.Validationf("response version is required")
	}
	if err := s.store.SubmitResponse(ctx, inv.ID, req); err != nil {
		return nil, err
	}
	return s.store.LoadHousehold(ctx, inv.ID)
}

// RequestRecovery deliberately has no existence-bearing return value. Callers
// receive the same public response even when this method reports an internal
// delivery failure to the handler's private log.
func (s *Service) RequestRecovery(ctx context.Context, eventID, contact, sourceIdentity string) error {
	eventID = strings.TrimSpace(eventID)
	contact = strings.TrimSpace(contact)
	if eventID == "" || contact == "" {
		return nil
	}
	normalized := normalizeEmail(contact)
	destinationKind := "email"
	if !strings.Contains(contact, "@") {
		normalized = normalizePhone(contact)
		destinationKind = "sms"
	}
	if normalized == "" {
		return nil
	}
	exists, err := s.store.EventExists(ctx, eventID)
	if err != nil || !exists {
		return err
	}
	allowed, err := s.store.AllowRecovery(ctx, eventID,
		s.signer.fingerprint("source", sourceIdentity),
		s.signer.fingerprint("destination", eventID, normalized))
	if err != nil || !allowed {
		return err
	}
	matches, err := s.store.FindRecoveryMatches(ctx, eventID, normalized)
	if err != nil {
		return err
	}
	for _, inv := range matches {
		if destinationKind != "email" || inv.PreferredDeliveryMethod != "email" ||
			inv.ContactEmail == nil || s.sendEmail == nil {
			continue
		}
		raw, err := randomToken(32)
		if err != nil {
			return err
		}
		if err := s.store.CreateRecoveryToken(ctx, inv.ID, hashToken(raw), "email",
			time.Now().UTC().Add(s.recoveryExpiry)); err != nil {
			return err
		}
		household, err := s.store.LoadHousehold(ctx, inv.ID)
		if err != nil {
			return err
		}
		url := s.baseURL + "/invitation/recover#" + raw
		subject := "Recover your invitation — " + household.Event.Title
		plain := fmt.Sprintf("Use this one-time link to recover your invitation:\n%s\n\nIt expires in %d minutes.", url, int(s.recoveryExpiry.Minutes()))
		htmlBody := fmt.Sprintf(`<p>Use this one-time link to recover your invitation:</p><p><a href="%s">Recover invitation</a></p><p>It expires in %d minutes.</p>`, html.EscapeString(url), int(s.recoveryExpiry.Minutes()))
		if err := s.sendEmail(ctx, inv.EventID, inv.ID, *inv.ContactEmail, subject, htmlBody, plain); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ExchangeRecovery(ctx context.Context, rawRecovery string) (string, *Household, error) {
	if len(rawRecovery) < 32 {
		return "", nil, ErrInvalidCapability
	}
	rawSession, err := randomToken(32)
	if err != nil {
		return "", nil, err
	}
	inv, err := s.store.ConsumeRecoveryAndCreateSession(ctx, hashToken(rawRecovery),
		hashToken(rawSession), time.Now().UTC().Add(s.sessionExpiry))
	if err != nil {
		return "", nil, err
	}
	household, err := s.store.LoadHousehold(ctx, inv.ID)
	if err != nil {
		return "", nil, err
	}
	return rawSession, household, nil
}

func (s *Service) ConfigureOpen(ctx context.Context, eventID, creatorUserID string, req ConfigureOpenRequest) (*OpenEnrollmentConfig, string, error) {
	if req.MaxPartySize < 1 || req.MaxPartySize > 50 {
		return nil, "", errcode.Validationf("max party size must be between 1 and 50")
	}
	if req.Capacity != nil && *req.Capacity < 1 {
		return nil, "", errcode.Validationf("capacity must be positive")
	}
	opensAt, err := parseOptionalTime(req.OpensAt)
	if err != nil {
		return nil, "", errcode.Validationf("opensAt must be RFC3339")
	}
	closesAt, err := parseOptionalTime(req.ClosesAt)
	if err != nil {
		return nil, "", errcode.Validationf("closesAt must be RFC3339")
	}
	if opensAt != nil && closesAt != nil && !opensAt.Before(*closesAt) {
		return nil, "", errcode.Validationf("opensAt must be before closesAt")
	}
	config, err := s.store.FindOpenByEvent(ctx, eventID)
	if err != nil {
		return nil, "", err
	}
	if config == nil {
		accessID, err := randomToken(18)
		if err != nil {
			return nil, "", err
		}
		config = &OpenEnrollmentConfig{ID: uuid.Must(uuid.NewV7()).String(),
			EventID: eventID, AccessID: accessID, TokenVersion: 1}
	}
	config.Enabled, config.OpensAt, config.ClosesAt = req.Enabled, opensAt, closesAt
	config.MaxPartySize, config.Capacity = req.MaxPartySize, req.Capacity
	if err := s.store.ConfigureOpen(ctx, config, creatorUserID); err != nil {
		return nil, "", err
	}
	config, err = s.store.FindOpenByEvent(ctx, eventID)
	if err != nil {
		return nil, "", err
	}
	return config, s.openAccessURL(config), nil
}

func (s *Service) RotateOpen(ctx context.Context, eventID string) (*OpenEnrollmentConfig, string, error) {
	config, err := s.store.RotateOpen(ctx, eventID)
	if err != nil {
		return nil, "", err
	}
	return config, s.openAccessURL(config), nil
}

func (s *Service) InspectOpen(ctx context.Context, rawCapability string) (*OpenEnrollmentConfig, *EventSummary, error) {
	config, err := s.validOpen(ctx, rawCapability)
	if err != nil {
		return nil, nil, err
	}
	event, err := s.store.EventSummary(ctx, config.EventID)
	if err != nil {
		return nil, nil, err
	}
	return config, event, nil
}

func (s *Service) EnrollOpen(ctx context.Context, req OpenEnrollmentRequest) (string, *Household, DeliveryResult, error) {
	config, err := s.validOpen(ctx, req.Capability)
	if err != nil {
		return "", nil, DeliveryResult{}, err
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		return "", nil, DeliveryResult{}, errcode.Validationf("label is required")
	}
	if len(req.GuestNames) < 1 || len(req.GuestNames) > config.MaxPartySize {
		return "", nil, DeliveryResult{}, errcode.Validationf("party size is outside the open invitation limit")
	}
	eventSummary, err := s.store.EventSummary(ctx, config.EventID)
	if err != nil {
		return "", nil, DeliveryResult{}, err
	}
	questions, err := s.store.listQuestions(ctx, config.EventID)
	if err != nil {
		return "", nil, DeliveryResult{}, err
	}
	if strings.TrimSpace(req.PreferredDeliveryMethod) != "email" {
		return "", nil, DeliveryResult{}, errcode.Validationf("open enrollment requires email delivery")
	}
	email, phone, method, err := validateContact(req.ContactEmail, req.ContactPhone, "email")
	if err != nil {
		return "", nil, DeliveryResult{}, err
	}
	accessID, err := randomToken(18)
	if err != nil {
		return "", nil, DeliveryResult{}, err
	}
	inv := &Invitation{ID: uuid.Must(uuid.NewV7()).String(), EventID: config.EventID,
		Label: label, ContactEmail: email, ContactPhone: phone,
		PreferredDeliveryMethod: method, Source: SourceOpen,
		OpenEnrollmentID: &config.ID, AccessID: accessID, TokenVersion: 1}
	guests := make([]*Guest, 0, len(req.GuestNames))
	for i, rawName := range req.GuestNames {
		name := strings.TrimSpace(rawName)
		if name == "" || len(name) > 200 {
			return "", nil, DeliveryResult{}, errcode.Validationf("guest names must be between 1 and 200 characters")
		}
		guests = append(guests, &Guest{ID: uuid.Must(uuid.NewV7()).String(),
			Name: name, Origin: GuestOriginAssigned, SortOrder: i})
	}
	rawSession, err := randomToken(32)
	if err != nil {
		return "", nil, DeliveryResult{}, err
	}
	response, err := s.store.EnrollOpen(ctx, config, inv, guests, hashToken(rawSession),
		time.Now().UTC().Add(s.sessionExpiry))
	if err != nil {
		return "", nil, DeliveryResult{}, err
	}
	household := &Household{Invitation: inv, Event: eventSummary, Response: response,
		Guests: guests, Questions: questions, InvitationAnswers: []Answer{}, GuestAnswers: []GuestAnswer{}}
	if household.Questions == nil {
		household.Questions = []*Question{}
	}
	delivery := s.attemptDelivery(ctx, inv.ID,
		"Enrollment succeeded, but the management email could not be sent. Keep this browser session open or request recovery later.")
	return rawSession, household, delivery, nil
}

func (s *Service) validOpen(ctx context.Context, raw string) (*OpenEnrollmentConfig, error) {
	accessID, version, err := s.signer.ParseOpen(raw)
	if err != nil {
		return nil, ErrInvalidCapability
	}
	config, err := s.store.FindOpenByAccessID(ctx, accessID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if config == nil || config.TokenVersion != version || !config.Enabled || config.RevokedAt != nil ||
		(config.OpensAt != nil && now.Before(*config.OpensAt)) ||
		(config.ClosesAt != nil && !now.Before(*config.ClosesAt)) {
		return nil, ErrInvalidCapability
	}
	return config, nil
}

func (s *Service) privateAccessURL(inv *Invitation) string {
	return s.baseURL + "/invitation/accept#" + s.signer.Private(inv.AccessID, inv.TokenVersion)
}

func (s *Service) openAccessURL(config *OpenEnrollmentConfig) string {
	return s.baseURL + "/enroll#" + s.signer.Open(config.AccessID, config.TokenVersion)
}

func validateContact(emailInput, phoneInput *string, method string) (*string, *string, string, error) {
	method = strings.TrimSpace(method)
	if method == "" {
		method = "email"
	}
	if method != "email" && method != "sms" && method != "none" {
		return nil, nil, "", errcode.Validationf("invalid preferred delivery method")
	}
	var emailValue, phoneValue *string
	if emailInput != nil && strings.TrimSpace(*emailInput) != "" {
		value := normalizeEmail(*emailInput)
		if _, err := mail.ParseAddress(value); err != nil {
			return nil, nil, "", errcode.Validationf("invalid contact email")
		}
		emailValue = &value
	}
	if phoneInput != nil && strings.TrimSpace(*phoneInput) != "" {
		value := normalizePhone(*phoneInput)
		if len(strings.TrimPrefix(value, "+")) < 7 {
			return nil, nil, "", errcode.Validationf("invalid contact phone")
		}
		phoneValue = &value
	}
	if method == "email" && emailValue == nil {
		return nil, nil, "", errcode.Validationf("contact email is required for email delivery")
	}
	if method == "sms" && phoneValue == nil {
		return nil, nil, "", errcode.Validationf("contact phone is required for SMS delivery")
	}
	return emailValue, phoneValue, method, nil
}

func parseOptionalTime(raw *string) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil, err
	}
	value = value.UTC()
	return &value, nil
}

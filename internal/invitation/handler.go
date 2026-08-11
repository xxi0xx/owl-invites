package invitation

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/xxi0xx/owl-invites/internal/errcode"
	"github.com/xxi0xx/owl-invites/internal/httpx"
)

const invitationSessionCookie = "owl_invitation_session"

const recoveryResponseFloor = 100 * time.Millisecond

type UserFromCtx func(ctx context.Context) (id string, ok bool)
type EventAccessChecker func(ctx context.Context, eventID, userID string) error

type Handler struct {
	service          *Service
	authMiddleware   func(http.Handler) http.Handler
	userFrom         UserFromCtx
	checkEventAccess EventAccessChecker
	secureCookies    bool
	logger           zerolog.Logger
}

func NewHandler(service *Service, authMiddleware func(http.Handler) http.Handler,
	userFrom UserFromCtx, checkEventAccess EventAccessChecker, secureCookies bool,
	logger zerolog.Logger) *Handler {
	return &Handler{service: service, authMiddleware: authMiddleware, userFrom: userFrom,
		checkEventAccess: checkEventAccess, secureCookies: secureCookies, logger: logger}
}

// OrganizerInvitationRoutes is mounted below
// /api/v1/events/{eventId}/invitations. Keeping this mount narrow prevents the
// invitation subrouter from shadowing the event's own GET/PUT routes.
func (h *Handler) OrganizerInvitationRoutes() chi.Router {
	r := chi.NewRouter()
	r.Use(h.authMiddleware)
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{invitationId}", h.get)
	r.Post("/{invitationId}/deliver", h.deliver)
	r.Post("/{invitationId}/rotate", h.rotate)
	r.Post("/{invitationId}/revoke", h.revoke)
	r.Post("/messages", h.message)
	return r
}

// OrganizerOpenEnrollmentRoutes is mounted below
// /api/v1/events/{eventId}/open-enrollment.
func (h *Handler) OrganizerOpenEnrollmentRoutes() chi.Router {
	r := chi.NewRouter()
	r.Use(h.authMiddleware)
	r.Get("/", h.getOpen)
	r.Put("/", h.configureOpen)
	r.Post("/rotate", h.rotateOpen)
	return r
}

func (h *Handler) message(w http.ResponseWriter, r *http.Request) {
	eventID, userID, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	var req MessageRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	sent, err := h.service.Broadcast(r.Context(), eventID, &userID, req)
	if h.writeServiceError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]int{"sent": sent}})
}

// PublicRoutes is mounted below /api/v1/invitations. Mutation routes that
// bootstrap a session are explicitly exempted from CSRF by the server; session
// response writes remain protected by the double-submit middleware.
func (h *Handler) PublicRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/exchange", h.exchange)
	r.Get("/session", h.session)
	r.Put("/session/response", h.submit)
	r.Post("/recovery/request", h.requestRecovery)
	r.Post("/recovery/exchange", h.exchangeRecovery)
	r.Post("/open/inspect", h.inspectOpen)
	r.Post("/open/enroll", h.enrollOpen)
	return r
}

func (h *Handler) eventActor(r *http.Request) (string, string, bool) {
	userID, ok := h.userFrom(r.Context())
	if !ok {
		return "", "", false
	}
	eventID := chi.URLParam(r, "eventId")
	if eventID == "" || h.checkEventAccess(r.Context(), eventID, userID) != nil {
		return "", "", false
	}
	return eventID, userID, true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	eventID, _, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	items, err := h.service.store.ListByEvent(r.Context(), eventID)
	if err != nil {
		h.internal(w, err, "list invitations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	eventID, userID, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	var req CreateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	result, err := h.service.CreatePrivate(r.Context(), eventID, userID, req)
	if h.writeServiceError(w, err) {
		return
	}
	h.logDeliveryFailure(result.Delivery, eventID, result.Invitation.ID, "private invitation delivery")
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	eventID, _, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	inv, err := h.service.store.FindByIDForEvent(r.Context(), chi.URLParam(r, "invitationId"), eventID)
	if err != nil {
		h.internal(w, err, "get invitation")
		return
	}
	if inv == nil {
		writeError(w, http.StatusNotFound, "not_found", "invitation not found")
		return
	}
	household, err := h.service.store.LoadHousehold(r.Context(), inv.ID)
	if err != nil {
		h.internal(w, err, "load invitation")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": household})
}

func (h *Handler) deliver(w http.ResponseWriter, r *http.Request) {
	eventID, _, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	inv, err := h.service.store.FindByIDForEvent(r.Context(), chi.URLParam(r, "invitationId"), eventID)
	if err != nil || inv == nil {
		writeError(w, http.StatusNotFound, "not_found", "invitation not found")
		return
	}
	if h.writeServiceError(w, h.service.Deliver(r.Context(), inv.ID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) rotate(w http.ResponseWriter, r *http.Request) {
	eventID, _, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	result, err := h.service.Rotate(r.Context(), eventID, chi.URLParam(r, "invitationId"))
	if h.writeServiceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	eventID, _, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if h.writeServiceError(w, h.service.Revoke(r.Context(), eventID,
		chi.URLParam(r, "invitationId"), req.Reason)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) exchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Capability string `json:"capability"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_capability", "invalid or expired invitation")
		return
	}
	rawSession, household, err := h.service.ExchangePrivate(r.Context(), req.Capability)
	if err != nil {
		h.capabilityError(w, err)
		return
	}
	h.setSessionCookie(w, rawSession)
	writeJSON(w, http.StatusOK, map[string]any{"data": household})
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	household, err := h.service.HouseholdForSession(r.Context(), h.sessionCookie(r))
	if err != nil {
		h.capabilityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": household})
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	var req SubmitRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	household, err := h.service.SubmitForSession(r.Context(), h.sessionCookie(r), req)
	if h.writeServiceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": household})
}

func (h *Handler) requestRecovery(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var req RecoveryRequest
	if err := httpx.DecodeJSON(r, &req); err == nil {
		eventID, contact, sourceIdentity := req.EventID, req.Contact, remoteIdentity(r)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := h.service.RequestRecovery(ctx, eventID, contact, sourceIdentity); err != nil {
				// The error is deliberately not reflected in the public response. Do
				// not attach event/contact values to this log entry.
				h.logger.Error().Err(err).Str("error_ref", errcode.Ref()).Msg("invitation recovery request failed")
			}
		}()
	}
	if remaining := recoveryResponseFloor - time.Since(started); remaining > 0 {
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		<-timer.C
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{
		"message": "If a matching invitation exists, recovery instructions will be sent to its stored destination.",
	}})
}

func (h *Handler) exchangeRecovery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Capability string `json:"capability"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_capability", "invalid or expired recovery link")
		return
	}
	rawSession, household, err := h.service.ExchangeRecovery(r.Context(), req.Capability)
	if err != nil {
		h.capabilityError(w, err)
		return
	}
	h.setSessionCookie(w, rawSession)
	writeJSON(w, http.StatusOK, map[string]any{"data": household})
}

func (h *Handler) getOpen(w http.ResponseWriter, r *http.Request) {
	eventID, _, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	config, err := h.service.store.FindOpenByEvent(r.Context(), eventID)
	if err != nil {
		h.internal(w, err, "get open enrollment")
		return
	}
	if config == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"config": config, "accessUrl": h.service.openAccessURL(config),
	}})
}

func (h *Handler) configureOpen(w http.ResponseWriter, r *http.Request) {
	eventID, userID, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	var req ConfigureOpenRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	config, accessURL, err := h.service.ConfigureOpen(r.Context(), eventID, userID, req)
	if h.writeServiceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"config": config, "accessUrl": accessURL,
	}})
}

func (h *Handler) rotateOpen(w http.ResponseWriter, r *http.Request) {
	eventID, _, ok := h.eventActor(r)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "event not found")
		return
	}
	config, accessURL, err := h.service.RotateOpen(r.Context(), eventID)
	if h.writeServiceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"config": config, "accessUrl": accessURL,
	}})
}

func (h *Handler) inspectOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Capability string `json:"capability"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "open invitation unavailable")
		return
	}
	config, event, err := h.service.InspectOpen(r.Context(), req.Capability)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "open invitation unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"event": event, "maxPartySize": config.MaxPartySize,
	}})
}

func (h *Handler) enrollOpen(w http.ResponseWriter, r *http.Request) {
	var req OpenEnrollmentRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	rawSession, household, delivery, err := h.service.EnrollOpen(r.Context(), req)
	if h.writeServiceError(w, err) {
		return
	}
	h.logDeliveryFailure(delivery, household.Invitation.EventID, household.Invitation.ID,
		"open enrollment management delivery")
	h.setSessionCookie(w, rawSession)
	writeJSON(w, http.StatusCreated, map[string]any{"data": household, "delivery": delivery})
}

func (h *Handler) logDeliveryFailure(delivery DeliveryResult, eventID, invitationID, action string) {
	if delivery.err == nil {
		return
	}
	h.logger.Error().Err(delivery.err).Str("error_ref", errcode.Ref()).
		Str("action", action).Str("event_id", eventID).Str("invitation_id", invitationID).
		Msg("invitation persisted but delivery failed")
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, raw string) {
	http.SetCookie(w, &http.Cookie{Name: invitationSessionCookie, Value: raw, Path: "/",
		HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteStrictMode,
		MaxAge: int(h.service.sessionExpiry.Seconds())})
}

func (h *Handler) sessionCookie(r *http.Request) string {
	cookie, err := r.Cookie(invitationSessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (h *Handler) capabilityError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrInvalidCapability) || errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid_capability", "invalid or expired invitation")
		return
	}
	h.internal(w, err, "invitation capability operation")
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errcode.IsValidation(err):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, "version_conflict", "the response changed; reload and try again")
	case errors.Is(err, ErrAllowance):
		writeError(w, http.StatusConflict, "allowance_exceeded", "additional guest allowance exceeded")
	case errors.Is(err, ErrCapacity):
		writeError(w, http.StatusConflict, "capacity_reached", "open invitation capacity reached")
	case errors.Is(err, ErrInvalidCapability):
		writeError(w, http.StatusUnauthorized, "invalid_capability", "invalid or expired invitation")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "invitation not found")
	default:
		h.internal(w, err, "invitation operation")
	}
	return true
}

func (h *Handler) internal(w http.ResponseWriter, err error, action string) {
	ref := errcode.Ref()
	h.logger.Error().Err(err).Str("error_ref", ref).Str("action", action).Msg("invitation request failed")
	writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred (ref: "+ref+")")
}

func remoteIdentity(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}

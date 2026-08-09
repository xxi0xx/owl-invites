package useradmin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/config"
	"github.com/yannkr/openrsvp/internal/errcode"
	"github.com/yannkr/openrsvp/internal/httpx"
)

type Handler struct {
	service         *Service
	cfg             *config.Config
	authMiddleware  func(http.Handler) http.Handler
	adminMiddleware func(http.Handler) http.Handler
	logger          zerolog.Logger
}

func NewHandler(service *Service, cfg *config.Config, authMiddleware func(http.Handler) http.Handler, adminMiddleware func(http.Handler) http.Handler, logger zerolog.Logger) *Handler {
	return &Handler{service: service, cfg: cfg, authMiddleware: authMiddleware, adminMiddleware: adminMiddleware, logger: logger}
}

func (h *Handler) UserRoutes() chi.Router {
	r := chi.NewRouter()
	r.Use(h.authMiddleware)
	r.Use(h.adminMiddleware)
	r.Get("/", h.handleListUsers)
	r.Get("/invites", h.handleListInvites)
	r.Post("/invites", h.handleInviteUser)
	r.Delete("/invites/{inviteID}", h.handleRevokeInvite)
	r.Patch("/{userID}/status", h.handleSetStatus)
	r.Patch("/{userID}/role", h.handleSetRole)
	return r
}

func (h *Handler) handleListInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := h.service.ListPendingInvites(r.Context())
	if err != nil {
		h.internal(w, err, "failed to list account invitations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"invites": invites}})
}

func (h *Handler) AuditRoutes() chi.Router {
	r := chi.NewRouter()
	r.Use(h.authMiddleware)
	r.Use(h.adminMiddleware)
	r.Get("/", h.handleListAudit)
	return r
}

func (h *Handler) PublicRoutes(rateLimit ...func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()
	if len(rateLimit) > 0 && rateLimit[0] != nil {
		r.With(rateLimit[0]).Post("/accept", h.handleAcceptInvite)
	} else {
		r.Post("/accept", h.handleAcceptInvite)
	}
	return r
}

func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.ListUsers(r.Context())
	if err != nil {
		h.internal(w, err, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": UserList{Users: users}})
}

func (h *Handler) handleInviteUser(w http.ResponseWriter, r *http.Request) {
	actor := auth.OrganizerFromContext(r.Context())
	var req CreateInviteRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	invite, err := h.service.InviteUser(r.Context(), actor, req.Email)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidEmail):
			writeError(w, http.StatusBadRequest, "invalid_email", "email must be a valid email address")
		case errors.Is(err, ErrUserExists):
			writeError(w, http.StatusConflict, "user_exists", "an active or disabled user with that email already exists")
		case errors.Is(err, ErrEmailUnavailable):
			writeError(w, http.StatusServiceUnavailable, "email_unavailable", "account invitation email is not configured")
		default:
			h.internal(w, err, "failed to invite user")
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": invite.AccountInvite})
}

func (h *Handler) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req AcceptInviteRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	response, err := h.service.AcceptInvite(r.Context(), req.Token)
	if err != nil {
		if errors.Is(err, ErrInvalidInvite) {
			writeError(w, http.StatusUnauthorized, "invalid_invite", "invalid or expired account invitation")
			return
		}
		h.internal(w, err, "failed to accept account invitation")
		return
	}
	h.setSessionCookies(w, response.Token)
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"user": response.Organizer}})
}

func (h *Handler) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	err := h.service.RevokeInvite(r.Context(), auth.OrganizerFromContext(r.Context()), chi.URLParam(r, "inviteID"))
	if err != nil {
		if errors.Is(err, ErrInvalidInvite) {
			writeError(w, http.StatusNotFound, "invite_not_found", "account invitation not found")
			return
		}
		if h.writeChangeError(w, err) {
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	var req UpdateUserStatusRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	err := h.service.SetUserStatus(r.Context(), auth.OrganizerFromContext(r.Context()), chi.URLParam(r, "userID"), strings.TrimSpace(req.Status))
	if h.writeChangeError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleSetRole(w http.ResponseWriter, r *http.Request) {
	var req UpdateUserRoleRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	err := h.service.SetUserRole(r.Context(), auth.OrganizerFromContext(r.Context()), chi.URLParam(r, "userID"), strings.TrimSpace(req.InstanceRole))
	if h.writeChangeError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := h.service.ListAudit(r.Context(), limit)
	if err != nil {
		h.internal(w, err, "failed to list admin audit")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"entries": entries}})
}

func (h *Handler) writeChangeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrUserNotFound):
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
	case errors.Is(err, ErrLastAdmin):
		writeError(w, http.StatusConflict, "last_admin", "the last active administrator cannot be changed")
	case errors.Is(err, ErrInvalidTransition):
		writeError(w, http.StatusBadRequest, "invalid_transition", "invalid user role or status transition")
	default:
		h.internal(w, err, "failed to change user")
	}
	return true
}

func (h *Handler) setSessionCookies(w http.ResponseWriter, token string) {
	secure := !h.cfg.IsDevelopment()
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: token, Path: "/", MaxAge: int(h.cfg.SessionExpiry / time.Second),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: "csrf_token", Value: "", Path: "/", MaxAge: -1,
		Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

func (h *Handler) internal(w http.ResponseWriter, err error, message string) {
	ref := errcode.Ref()
	h.logger.Error().Err(err).Str("error_code", ref).Msg(message)
	writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred (ref: "+ref+")")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}

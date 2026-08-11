package feedback

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/xxi0xx/owl-invites/internal/errcode"
	"github.com/xxi0xx/owl-invites/internal/httpx"
)

// OrganizerFromCtx extracts the organizer email from the request context.
type OrganizerFromCtx func(ctx context.Context) (email string, ok bool)

// Handler holds HTTP handlers for feedback endpoints.
type Handler struct {
	service        *Service
	authMiddleware func(http.Handler) http.Handler
	organizerFrom  OrganizerFromCtx
	logger         zerolog.Logger
}

// NewHandler creates a new feedback Handler.
func NewHandler(service *Service, authMiddleware func(http.Handler) http.Handler, organizerFrom OrganizerFromCtx, logger zerolog.Logger) *Handler {
	return &Handler{
		service:        service,
		authMiddleware: authMiddleware,
		organizerFrom:  organizerFrom,
		logger:         logger,
	}
}

// Routes returns a chi.Router with all feedback routes mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// Public route (no authentication required). Mounted at /public so the
	// orchestrator can CSRF-exempt just this path. Guests hitting a bug on a
	// public invite page can report it without an account.
	r.Post("/public", h.handleSubmitPublic)

	// Authenticated organizer route.
	r.Group(func(auth chi.Router) {
		auth.Use(h.authMiddleware)
		auth.Post("/", h.handleSubmit)
	})

	return r
}

type submitRequest struct {
	Type          string `json:"type"`
	Message       string `json:"message"`
	AllowFollowUp bool   `json:"allowFollowUp"`
}

type publicSubmitRequest struct {
	Message string `json:"message"`
	Contact string `json:"contact"`
	Source  string `json:"source"`
}

func (h *Handler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	email, ok := h.organizerFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	var req submitRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	// Validate type.
	switch req.Type {
	case "bug", "feature", "general":
		// ok
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "type must be bug, feature, or general")
		return
	}

	// Validate message.
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "message is required")
		return
	}
	if len(req.Message) > 2000 {
		writeError(w, http.StatusBadRequest, "bad_request", "message must be 2000 characters or fewer")
		return
	}

	if err := h.service.Submit(r.Context(), email, req.Type, req.Message, req.AllowFollowUp); err != nil {
		ref := errcode.Ref()
		h.logger.Error().Err(err).Str("error_code", ref).Msg("failed to submit feedback")
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred (ref: "+ref+")")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"status": "submitted"}})
}

// handleSubmitPublic handles unauthenticated guest feedback. It accepts a
// message plus an optional contact email and source/page, and routes through
// the same feedback service (GitHub issue / email fallback) labeled as guest
// feedback. No organizer identity is required.
func (h *Handler) handleSubmitPublic(w http.ResponseWriter, r *http.Request) {
	var req publicSubmitRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "message is required")
		return
	}
	if len(req.Message) > 2000 {
		writeError(w, http.StatusBadRequest, "bad_request", "message must be 2000 characters or fewer")
		return
	}
	if len(req.Contact) > 254 {
		writeError(w, http.StatusBadRequest, "bad_request", "contact must be 254 characters or fewer")
		return
	}
	if len(req.Source) > 512 {
		writeError(w, http.StatusBadRequest, "bad_request", "source must be 512 characters or fewer")
		return
	}

	if err := h.service.SubmitGuest(r.Context(), req.Message, req.Contact, req.Source); err != nil {
		ref := errcode.Ref()
		h.logger.Error().Err(err).Str("error_code", ref).Msg("failed to submit guest feedback")
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred (ref: "+ref+")")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"status": "submitted"}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   errCode,
		"message": message,
	})
}

package suppression

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/yannkr/openrsvp/internal/errcode"
	"github.com/yannkr/openrsvp/internal/httpx"
)

// Handler provides HTTP handlers for the public unsubscribe endpoints.
//
// All routes are public: the recipient clicking an unsubscribe link is not
// authenticated, and the request carries no session cookie. The token itself
// is the bearer of authority, so these routes must bypass both the auth
// middleware and CSRF validation (see the package README / integration notes).
type Handler struct {
	service *Service
	logger  zerolog.Logger
}

// NewHandler creates a new suppression Handler.
func NewHandler(service *Service, logger zerolog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// Routes returns a chi.Router with the public unsubscribe routes mounted.
// No auth middleware is applied; the unsubscribe token authorizes the request.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.handleResolve)
	r.Post("/", h.handleUnsubscribe)
	return r
}

type unsubscribeResponse struct {
	Email   string  `json:"email"`
	EventID *string `json:"eventId"`
}

// handleResolve validates the token from the query string and returns the
// email + event context so the confirmation page can show what is being
// unsubscribed. It does not mutate state.
func (h *Handler) handleResolve(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "token is required")
		return
	}

	email, eventID, ok, err := h.service.VerifyUnsubscribeToken(r.Context(), token)
	if err != nil {
		ref := errcode.Ref()
		h.logger.Error().Err(err).Str("error_code", ref).Msg("failed to verify unsubscribe token")
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred (ref: "+ref+")")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "invalid or expired unsubscribe link")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": unsubscribeResponse{Email: email, EventID: eventID},
	})
}

type unsubscribeRequest struct {
	Token string `json:"token"`
}

// handleUnsubscribe validates the token from the request body and records the
// suppression. It is idempotent and always reports success for a valid token.
func (h *Handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req unsubscribeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "token is required")
		return
	}

	email, eventID, ok, err := h.service.VerifyUnsubscribeToken(r.Context(), req.Token)
	if err != nil {
		ref := errcode.Ref()
		h.logger.Error().Err(err).Str("error_code", ref).Msg("failed to verify unsubscribe token")
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred (ref: "+ref+")")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "invalid or expired unsubscribe link")
		return
	}

	scope := ""
	if eventID != nil {
		scope = *eventID
	}
	if err := h.service.Suppress(r.Context(), email, scope, ReasonUnsubscribe); err != nil {
		ref := errcode.Ref()
		h.logger.Error().Err(err).Str("error_code", ref).Msg("failed to record suppression")
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred (ref: "+ref+")")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "unsubscribed"}})
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

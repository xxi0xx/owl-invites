package eventadmin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yannkr/openrsvp/internal/auth"
	"github.com/yannkr/openrsvp/internal/httpx"
)

type Handler struct {
	service         *Service
	authMiddleware  func(http.Handler) http.Handler
	adminMiddleware func(http.Handler) http.Handler
}

func NewHandler(service *Service, authMiddleware func(http.Handler) http.Handler, adminMiddleware func(http.Handler) http.Handler) *Handler {
	return &Handler{service: service, authMiddleware: authMiddleware, adminMiddleware: adminMiddleware}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(h.authMiddleware)
	r.Use(h.adminMiddleware)
	r.Post("/{eventID}/memberships/self", h.handleAddSelf)
	r.Post("/{eventID}/ownership-transfer", h.handleTransfer)
	return r
}

func (h *Handler) handleAddSelf(w http.ResponseWriter, r *http.Request) {
	err := h.service.AddSelf(r.Context(), auth.OrganizerFromContext(r.Context()), chi.URLParam(r, "eventID"))
	if writeServiceError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type transferRequest struct {
	NewOwnerUserID string `json:"newOwnerUserId"`
}

func (h *Handler) handleTransfer(w http.ResponseWriter, r *http.Request) {
	var req transferRequest
	if err := httpx.DecodeJSON(r, &req); err != nil || req.NewOwnerUserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "newOwnerUserId is required"})
		return
	}
	err := h.service.TransferOwnership(r.Context(), auth.OrganizerFromContext(r.Context()), chi.URLParam(r, "eventID"), req.NewOwnerUserID)
	if writeServiceError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeServiceError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, ErrEventNotFound):
		status, code = http.StatusNotFound, "event_not_found"
	case errors.Is(err, ErrUserNotFound):
		status, code = http.StatusBadRequest, "eligible_user_not_found"
	case errors.Is(err, ErrForbidden):
		status, code = http.StatusForbidden, "forbidden"
	}
	writeJSON(w, status, map[string]string{"error": code})
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

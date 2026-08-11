package instanceconfig

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/xxi0xx/owl-invites/internal/config"
	"github.com/xxi0xx/owl-invites/internal/errcode"
	"github.com/xxi0xx/owl-invites/internal/httpx"
)

// Handler provides HTTP handlers for the first-run setup wizard.
type Handler struct {
	service         *Service
	bootstrap       *BootstrapService
	cfg             *config.Config
	authMiddleware  func(http.Handler) http.Handler
	adminMiddleware func(http.Handler) http.Handler
	logger          zerolog.Logger
}

// NewHandler creates a new setup Handler.
func NewHandler(service *Service, bootstrap *BootstrapService, cfg *config.Config, authMiddleware func(http.Handler) http.Handler, adminMiddleware func(http.Handler) http.Handler, logger zerolog.Logger) *Handler {
	return &Handler{
		service:         service,
		bootstrap:       bootstrap,
		cfg:             cfg,
		authMiddleware:  authMiddleware,
		adminMiddleware: adminMiddleware,
		logger:          logger,
	}
}

// Routes returns a chi.Router with the setup wizard routes.
//
// GET  /status  is public so the SPA can decide whether to show the wizard.
// GET  /config  and POST /config are admin-only (RequireAuth + RequireAdmin).
func (h *Handler) Routes(bootstrapRateLimit ...func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()

	// Public: no auth, no CSRF (GET).
	r.Get("/status", h.handleStatus)
	if len(bootstrapRateLimit) > 0 && bootstrapRateLimit[0] != nil {
		r.With(bootstrapRateLimit[0]).Post("/bootstrap", h.handleBootstrap)
	} else {
		r.Post("/bootstrap", h.handleBootstrap)
	}

	// Admin-only: auth + admin. POST is CSRF-protected by the global middleware.
	r.Group(func(r chi.Router) {
		r.Use(h.authMiddleware)
		r.Use(h.adminMiddleware)
		r.Get("/config", h.handleGetConfig)
		r.Post("/config", h.handleSaveConfig)
	})

	return r
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	configured, err := h.service.IsConfigured(r.Context())
	if err != nil {
		h.writeInternal(w, err, "failed to read setup status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":    configured,
		"setupRequired": !configured,
	})
}

func (h *Handler) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	configured, err := h.service.IsConfigured(r.Context())
	if err != nil {
		h.writeInternal(w, err, "failed to read setup status")
		return
	}
	if configured {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}

	var req BootstrapRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	trimBootstrapRequest(&req)
	if message := validateBootstrapRequest(&req); message != "" {
		writeError(w, http.StatusBadRequest, "bad_request", message)
		return
	}

	result, err := h.bootstrap.Bootstrap(r.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrBootstrapUnauthorized):
			writeError(w, http.StatusUnauthorized, "bootstrap_unauthorized", "bootstrap authorization failed")
		case errors.Is(err, ErrSetupComplete):
			writeError(w, http.StatusNotFound, "not_found", "not found")
		default:
			h.writeInternal(w, err, "failed to bootstrap instance")
		}
		return
	}

	secure := !h.cfg.IsDevelopment()
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: result.SessionToken, Path: "/",
		MaxAge: int(h.cfg.SessionExpiry / time.Second), HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: "csrf_token", Value: "", Path: "/", MaxAge: -1,
		Secure: secure, SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"user":       result.User,
		"configured": true,
	}})
}

func trimBootstrapRequest(req *BootstrapRequest) {
	req.AdminEmail = strings.ToLower(strings.TrimSpace(req.AdminEmail))
	req.AdminName = strings.TrimSpace(req.AdminName)
	req.InstanceName = strings.TrimSpace(req.InstanceName)
	req.DefaultTimezone = strings.TrimSpace(req.DefaultTimezone)
	req.SupportEmail = strings.TrimSpace(req.SupportEmail)
}

func validateBootstrapRequest(req *BootstrapRequest) string {
	if req.BootstrapToken == "" {
		return "bootstrapToken is required"
	}
	parsed, err := mail.ParseAddress(req.AdminEmail)
	if err != nil || parsed.Address != req.AdminEmail || len(req.AdminEmail) > 320 {
		return "adminEmail must be a valid email address"
	}
	if req.AdminName == "" || len(req.AdminName) > 200 {
		return "adminName is required and must be 200 characters or fewer"
	}
	if req.InstanceName == "" || len(req.InstanceName) > 200 {
		return "instanceName is required and must be 200 characters or fewer"
	}
	if req.DefaultTimezone == "" || len(req.DefaultTimezone) > 100 {
		return "defaultTimezone is required and must be 100 characters or fewer"
	}
	if len(req.SupportEmail) > 320 {
		return "supportEmail must be 320 characters or fewer"
	}
	if req.SupportEmail != "" {
		parsed, err := mail.ParseAddress(req.SupportEmail)
		if err != nil || parsed.Address != req.SupportEmail {
			return "supportEmail must be a valid email address"
		}
	}
	return ""
}

func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.GetSettings(r.Context())
	if err != nil {
		h.writeInternal(w, err, "failed to read setup config")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
}

type saveConfigRequest struct {
	InstanceName    string `json:"instanceName"`
	DefaultTimezone string `json:"defaultTimezone"`
	AllowSignups    bool   `json:"allowSignups"`
	SupportEmail    string `json:"supportEmail"`
}

func (h *Handler) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req saveConfigRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	req.InstanceName = strings.TrimSpace(req.InstanceName)
	req.DefaultTimezone = strings.TrimSpace(req.DefaultTimezone)
	req.SupportEmail = strings.TrimSpace(req.SupportEmail)

	if req.InstanceName == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "instanceName is required")
		return
	}
	if len(req.InstanceName) > 200 {
		writeError(w, http.StatusBadRequest, "bad_request", "instanceName must be 200 characters or fewer")
		return
	}
	if req.DefaultTimezone == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "defaultTimezone is required")
		return
	}
	if len(req.DefaultTimezone) > 100 {
		writeError(w, http.StatusBadRequest, "bad_request", "defaultTimezone must be 100 characters or fewer")
		return
	}
	if len(req.SupportEmail) > 320 {
		writeError(w, http.StatusBadRequest, "bad_request", "supportEmail must be 320 characters or fewer")
		return
	}

	settings := &Settings{
		InstanceName:    req.InstanceName,
		DefaultTimezone: req.DefaultTimezone,
		AllowSignups:    req.AllowSignups,
		SupportEmail:    req.SupportEmail,
	}
	if err := h.service.SaveSettings(r.Context(), settings); err != nil {
		h.writeInternal(w, err, "failed to save setup config")
		return
	}

	configured, err := h.service.IsConfigured(r.Context())
	if err != nil {
		h.writeInternal(w, err, "failed to read setup status")
		return
	}
	settings.Configured = configured
	writeJSON(w, http.StatusOK, map[string]any{"data": settings})
}

func (h *Handler) writeInternal(w http.ResponseWriter, err error, msg string) {
	ref := errcode.Ref()
	h.logger.Error().Err(err).Str("error_code", ref).Msg(msg)
	writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred (ref: "+ref+")")
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

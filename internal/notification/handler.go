package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

// 1x1 transparent GIF for open tracking pixel.
var transparentGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00,
	0x80, 0x00, 0x00, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21,
	0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00,
	0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44,
	0x01, 0x00, 0x3b,
}

// OrganizerFromCtx extracts the organizer ID from the request context.
type OrganizerFromCtx func(ctx context.Context) (string, bool)

// EventOwnershipChecker verifies an organizer can manage an event.
type EventOwnershipChecker func(ctx context.Context, eventID, organizerID string) error

// Suppressor is the optional dependency the webhook handlers use to suppress an
// address after a hard bounce or complaint. Satisfied by *suppression.Service.
// A nil Suppressor disables suppression on inbound delivery events (the event
// is still recorded in the notification log).
type Suppressor interface {
	Suppress(ctx context.Context, email, eventID, reason string) error
}

// Handler handles notification tracking HTTP endpoints.
type Handler struct {
	tracking       *TrackingService
	service        *Service
	suppressor     Suppressor
	authMiddleware func(http.Handler) http.Handler
	organizerFrom  OrganizerFromCtx
	checkOwner     EventOwnershipChecker
	logger         zerolog.Logger
}

// NewHandler creates a new notification Handler. suppressor may be nil to
// disable suppression on inbound bounce/complaint webhooks.
func NewHandler(
	tracking *TrackingService,
	service *Service,
	suppressor Suppressor,
	authMiddleware func(http.Handler) http.Handler,
	organizerFrom OrganizerFromCtx,
	checkOwner EventOwnershipChecker,
	logger zerolog.Logger,
) *Handler {
	return &Handler{
		tracking:       tracking,
		service:        service,
		suppressor:     suppressor,
		authMiddleware: authMiddleware,
		organizerFrom:  organizerFrom,
		checkOwner:     checkOwner,
		logger:         logger,
	}
}

// Routes returns the chi router for notification endpoints.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// Public tracking endpoints (no auth).
	r.Get("/track/open/{logId}", h.handleTrackOpen)

	// Provider delivery webhooks intentionally remain unmounted until their
	// provider-specific signatures, timestamps, replay protection, and message
	// correlation are verified. Keeping the parser code private preserves the
	// implementation work without exposing an unsigned suppression sink.

	// Authenticated endpoints.
	r.Group(func(auth chi.Router) {
		auth.Use(h.authMiddleware)
		auth.Get("/event/{eventId}/stats", h.handleGetStats)
		auth.Get("/event/{eventId}", h.handleGetLog)
	})

	return r
}

func (h *Handler) handleTrackOpen(w http.ResponseWriter, r *http.Request) {
	logID := chi.URLParam(r, "logId")

	// Record open asynchronously with a detached context so the write
	// survives after the HTTP response is sent.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		h.tracking.RecordOpen(ctx, logID)
	}()

	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_, _ = w.Write(transparentGIF)
}

// maxWebhookBody caps inbound webhook payload size to defend against abuse.
const maxWebhookBody = 1 << 20 // 1 MiB

// sendGridEvent is one element of a SendGrid event webhook array. Only the
// fields we act on are parsed; unknown fields are ignored.
type sendGridEvent struct {
	Email       string `json:"email"`
	Event       string `json:"event"` // delivered, open, bounce, dropped, spamreport, ...
	Type        string `json:"type"`  // for bounce: "bounce" (hard) or "blocked"
	SGMessageID string `json:"sg_message_id"`
	Timestamp   int64  `json:"timestamp"`
}

// handleSendGridWebhook processes a SendGrid event webhook (a JSON array of
// events). It records each delivery event and suppresses addresses on hard
// bounces and spam complaints.
//
// TODO: verify the SendGrid "Signed Event Webhook" signature
// (X-Twilio-Email-Event-Webhook-Signature / -Timestamp) once the verification
// public key is plumbed through config. Until then this endpoint trusts the
// payload; deploy it behind a hard-to-guess path or a reverse-proxy allowlist.
func (h *Handler) handleSendGridWebhook(w http.ResponseWriter, r *http.Request) {
	var events []sendGridEvent
	if err := json.NewDecoder(io.LimitReader(r.Body, maxWebhookBody)).Decode(&events); err != nil {
		h.logger.Warn().Err(err).Msg("sendgrid webhook: invalid payload")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, e := range events {
		eventType, bounceType := mapSendGridEvent(e)
		if eventType == "" {
			continue // Unknown/ignored event type.
		}

		ts := time.Now().UTC()
		if e.Timestamp > 0 {
			ts = time.Unix(e.Timestamp, 0).UTC()
		}

		if e.SGMessageID != "" {
			if err := h.tracking.ProcessDeliveryEvent(r.Context(), DeliveryEvent{
				MessageID:  e.SGMessageID,
				EventType:  eventType,
				Timestamp:  ts,
				BounceType: bounceType,
			}); err != nil {
				h.logger.Debug().Err(err).Str("message_id", e.SGMessageID).Msg("sendgrid webhook: process delivery event")
			}
		}

		h.suppressOnFailure(r.Context(), eventType, bounceType, e.Email)
	}

	w.WriteHeader(http.StatusOK)
}

// mapSendGridEvent maps a SendGrid event to an internal delivery status and
// bounce type. Returns ("", "") for events we do not track.
func mapSendGridEvent(e sendGridEvent) (eventType, bounceType string) {
	switch e.Event {
	case "delivered":
		return "delivered", ""
	case "open":
		return "opened", ""
	case "click":
		return "clicked", ""
	case "bounce":
		// SendGrid "bounce" is a hard bounce; "blocked" arrives as a separate
		// event we treat as soft.
		return "bounced", "hard"
	case "blocked":
		return "bounced", "soft"
	case "dropped":
		return "bounced", "hard"
	case "spamreport":
		return "complained", ""
	default:
		return "", ""
	}
}

// snsNotification is the SNS envelope SES delivery notifications arrive in.
type snsNotification struct {
	Type    string `json:"Type"`
	Message string `json:"Message"` // JSON-encoded SES event (stringified).
}

// sesEvent is the SES delivery notification carried inside the SNS Message.
type sesEvent struct {
	NotificationType string `json:"notificationType"` // Bounce, Complaint, Delivery
	Mail             struct {
		MessageID string `json:"messageId"`
	} `json:"mail"`
	Bounce struct {
		BounceType        string `json:"bounceType"` // Permanent, Transient, Undetermined
		BouncedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"bouncedRecipients"`
	} `json:"bounce"`
	Complaint struct {
		ComplainedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"complainedRecipients"`
	} `json:"complaint"`
}

// handleSESWebhook processes an Amazon SES delivery notification delivered via
// SNS. It records the delivery event and suppresses recipients on permanent
// bounces and complaints.
//
// TODO: verify the SNS message signature (SigningCertURL / Signature) and
// handle SubscriptionConfirmation messages once SNS is configured. Until then
// this endpoint trusts the payload; deploy behind an allowlist.
func (h *Handler) handleSESWebhook(w http.ResponseWriter, r *http.Request) {
	var envelope snsNotification
	if err := json.NewDecoder(io.LimitReader(r.Body, maxWebhookBody)).Decode(&envelope); err != nil {
		h.logger.Warn().Err(err).Msg("ses webhook: invalid payload")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var ses sesEvent
	if err := json.Unmarshal([]byte(envelope.Message), &ses); err != nil {
		h.logger.Warn().Err(err).Msg("ses webhook: invalid SES message")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventType, bounceType, recipients := mapSESEvent(ses)
	if eventType == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if ses.Mail.MessageID != "" {
		if err := h.tracking.ProcessDeliveryEvent(r.Context(), DeliveryEvent{
			MessageID:  ses.Mail.MessageID,
			EventType:  eventType,
			Timestamp:  time.Now().UTC(),
			BounceType: bounceType,
		}); err != nil {
			h.logger.Debug().Err(err).Str("message_id", ses.Mail.MessageID).Msg("ses webhook: process delivery event")
		}
	}

	for _, email := range recipients {
		h.suppressOnFailure(r.Context(), eventType, bounceType, email)
	}

	w.WriteHeader(http.StatusOK)
}

// mapSESEvent maps an SES notification to an internal delivery status, bounce
// type, and the affected recipient addresses. Returns "" for untracked types.
func mapSESEvent(ses sesEvent) (eventType, bounceType string, recipients []string) {
	switch ses.NotificationType {
	case "Delivery":
		return "delivered", "", nil
	case "Bounce":
		bt := "soft"
		if ses.Bounce.BounceType == "Permanent" {
			bt = "hard"
		}
		for _, r := range ses.Bounce.BouncedRecipients {
			if r.EmailAddress != "" {
				recipients = append(recipients, r.EmailAddress)
			}
		}
		return "bounced", bt, recipients
	case "Complaint":
		for _, r := range ses.Complaint.ComplainedRecipients {
			if r.EmailAddress != "" {
				recipients = append(recipients, r.EmailAddress)
			}
		}
		return "complained", "", recipients
	default:
		return "", "", nil
	}
}

// suppressOnFailure suppresses an address (globally) after a hard bounce or a
// complaint so it stops receiving mail. No-op without a suppressor, an empty
// email, or for non-terminal events.
func (h *Handler) suppressOnFailure(ctx context.Context, eventType, bounceType, email string) {
	if h.suppressor == nil || email == "" {
		return
	}
	var reason string
	switch {
	case eventType == "bounced" && bounceType == "hard":
		reason = "bounce"
	case eventType == "complained":
		reason = "complaint"
	default:
		return
	}
	if err := h.suppressor.Suppress(ctx, email, "", reason); err != nil {
		h.logger.Error().Err(err).Str("email", email).Str("reason", reason).Msg("failed to suppress address")
	}
}

func (h *Handler) handleGetStats(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventId")
	organizerID, ok := h.organizerFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if err := h.checkOwner(r.Context(), eventID, organizerID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
		return
	}

	stats, err := h.tracking.GetEmailStats(r.Context(), eventID)
	if err != nil {
		h.logger.Error().Err(err).Str("event_id", eventID).Msg("failed to get email stats")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": stats})
}

func (h *Handler) handleGetLog(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventId")
	organizerID, ok := h.organizerFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if err := h.checkOwner(r.Context(), eventID, organizerID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
		return
	}

	entries, err := h.service.GetLogsByEvent(r.Context(), eventID)
	if err != nil {
		h.logger.Error().Err(err).Str("event_id", eventID).Msg("failed to get notification log")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if entries == nil {
		entries = []*LogEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": entries})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

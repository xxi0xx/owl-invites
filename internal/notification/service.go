package notification

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/yannkr/openrsvp/internal/database"
)

// LogEntry represents a row in the notification_log table.
type LogEntry struct {
	ID             string
	EventID        string
	InvitationID   string
	Channel        string
	Provider       string
	Status         string // "pending", "sent", "failed"
	DeliveryStatus string // "unknown", "delivered", "opened", "clicked", "bounced", "complained"
	Error          string
	Recipient      string
	Subject        string
	MessageID      string
	SentAt         *time.Time
	DeliveredAt    *time.Time
	OpenedAt       *time.Time
	ClickedAt      *time.Time
	BouncedAt      *time.Time
	BounceType     string
	ComplaintAt    *time.Time
	CreatedAt      time.Time
}

// SuppressionChecker is the optional dependency the Service uses to skip
// suppressed recipients and to build unsubscribe links. It is satisfied by
// *suppression.Service. A nil checker disables the suppression gate and the
// unsubscribe footer (graceful degradation).
type SuppressionChecker interface {
	// IsSuppressed reports whether the address is suppressed globally or for
	// the given event (eventID "" means global only).
	IsSuppressed(ctx context.Context, email, eventID string) bool
	// GenerateUnsubscribeToken mints an unsubscribe token for the address,
	// scoped to the event (eventID "" means global).
	GenerateUnsubscribeToken(ctx context.Context, email, eventID string) (string, error)
}

// Options configures optional Service behavior. The zero value disables open
// tracking and the unsubscribe footer, preserving legacy behavior.
type Options struct {
	// BaseURL is the public base URL used to build tracking-pixel and
	// unsubscribe links (e.g. "https://rsvp.example.com").
	BaseURL string
	// OpenTrackingEnabled controls whether an open-tracking pixel is embedded
	// into outbound HTML email bodies (EMAIL_OPEN_TRACKING_ENABLED).
	OpenTrackingEnabled bool
	// Suppression is the optional suppression dependency. When nil, the
	// suppression gate and unsubscribe footer are disabled.
	Suppression SuppressionChecker
}

// Service dispatches notifications via registered providers and logs results.
type Service struct {
	registry *Registry
	db       database.DB
	logger   zerolog.Logger
	opts     Options
}

// NewService creates a new notification Service with default (disabled)
// options: no open-tracking pixel, no suppression gate, no unsubscribe footer.
func NewService(registry *Registry, db database.DB, logger zerolog.Logger) *Service {
	return NewServiceWithOptions(registry, db, logger, Options{})
}

// NewServiceWithOptions creates a new notification Service with the given
// options for open tracking, base URL, and the optional suppression checker.
func NewServiceWithOptions(registry *Registry, db database.DB, logger zerolog.Logger, opts Options) *Service {
	return &Service{
		registry: registry,
		db:       db,
		logger:   logger,
		opts:     opts,
	}
}

// Send delivers a single notification and logs the result.
func (s *Service) Send(ctx context.Context, eventID, invitationID string, ch Channel, msg *Message) error {
	provider, err := s.registry.Get(ch)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}

	// Suppression gate: skip email recipients that have unsubscribed, bounced,
	// or complained. SMS is not gated by email suppression.
	if s.isSuppressedEmail(ctx, ch, msg.To, eventID) {
		s.logger.Info().Str("recipient", msg.To).Str("event_id", eventID).Msg("recipient suppressed, skipping send")
		return nil
	}

	logID := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()

	// For email, append an unsubscribe footer and embed the open-tracking
	// pixel keyed on this log row's id before logging/sending.
	if ch == ChannelEmail {
		s.applyUnsubscribeFooter(ctx, msg, eventID)
		s.embedOpenPixel(msg, logID)
	}

	// Insert pending log entry with recipient and subject for tracking.
	if err := s.insertLog(ctx, logID, eventID, invitationID, string(ch), provider.Name(), "pending", "", msg.To, msg.Subject, nil, now); err != nil {
		s.logger.Error().Err(err).Msg("failed to insert notification log")
	}

	// Attempt delivery with retry on transient errors.
	const maxAttempts = 3
	var sendErr error
	var result *SendResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, sendErr = provider.Send(ctx, msg)
		if sendErr == nil {
			break
		}

		if attempt < maxAttempts {
			// Check if context is already cancelled before retrying.
			if ctx.Err() != nil {
				s.logger.Warn().Err(ctx.Err()).Int("attempt", attempt).Msg("context cancelled, skipping retry")
				break
			}

			backoff := time.Duration(1<<(attempt-1)) * time.Second // 1s, 2s, 4s
			s.logger.Warn().Err(sendErr).Int("attempt", attempt).Dur("backoff", backoff).Msg("notification send failed, retrying")
			time.Sleep(backoff)
		}
	}

	if sendErr != nil {
		// Update log to failed.
		if err := s.updateLog(ctx, logID, "failed", sendErr.Error(), "", nil); err != nil {
			s.logger.Error().Err(err).Msg("failed to update notification log")
		}
		return fmt.Errorf("send notification: %w", sendErr)
	}

	// Update log to sent with message ID for delivery tracking.
	sentAt := time.Now().UTC()
	var messageID string
	if result != nil {
		messageID = result.MessageID
	}
	if err := s.updateLog(ctx, logID, "sent", "", messageID, &sentAt); err != nil {
		s.logger.Error().Err(err).Msg("failed to update notification log")
	}

	return nil
}

// isSuppressedEmail reports whether an email recipient is suppressed. It
// returns false when the channel is not email or when no suppression checker
// is configured.
func (s *Service) isSuppressedEmail(ctx context.Context, ch Channel, email, eventID string) bool {
	if ch != ChannelEmail || s.opts.Suppression == nil || email == "" {
		return false
	}
	return s.opts.Suppression.IsSuppressed(ctx, email, eventID)
}

// embedOpenPixel injects a 1x1 open-tracking pixel into the HTML body of an
// email, keyed on the notification_log row id. It is a no-op when open
// tracking is disabled, no base URL is set, or the message has no HTML body.
// Plain-text parts never receive a pixel.
func (s *Service) embedOpenPixel(msg *Message, logID string) {
	if !s.opts.OpenTrackingEnabled || s.opts.BaseURL == "" || msg.Body == "" {
		return
	}
	pixel := fmt.Sprintf(
		`<img src="%s/api/v1/notifications/track/open/%s" width="1" height="1" alt="" style="display:none" />`,
		strings.TrimRight(s.opts.BaseURL, "/"), logID,
	)
	// Append before </body> when present so the pixel stays inside the document.
	if idx := strings.LastIndex(strings.ToLower(msg.Body), "</body>"); idx != -1 {
		msg.Body = msg.Body[:idx] + pixel + msg.Body[idx:]
	} else {
		msg.Body += pixel
	}
}

// applyUnsubscribeFooter appends an unsubscribe footer to the HTML and plain
// text parts of an email. It is a no-op when no suppression checker is
// configured or no base URL is set. The link is scoped to the event so the
// recipient can opt out of just this event's mail.
func (s *Service) applyUnsubscribeFooter(ctx context.Context, msg *Message, eventID string) {
	if s.opts.Suppression == nil || s.opts.BaseURL == "" || msg.To == "" {
		return
	}
	token, err := s.opts.Suppression.GenerateUnsubscribeToken(ctx, msg.To, eventID)
	if err != nil {
		s.logger.Error().Err(err).Str("recipient", msg.To).Msg("failed to generate unsubscribe token")
		return
	}
	base := strings.TrimRight(s.opts.BaseURL, "/")
	link := fmt.Sprintf("%s/unsubscribe?token=%s", base, token)

	if msg.Body != "" {
		footerHTML := fmt.Sprintf(
			`<div style="margin-top:24px;font-size:12px;color:#A8A29E;text-align:center">`+
				`<a href="%s" style="color:#A8A29E">Unsubscribe from these emails</a></div>`,
			link,
		)
		if idx := strings.LastIndex(strings.ToLower(msg.Body), "</body>"); idx != -1 {
			msg.Body = msg.Body[:idx] + footerHTML + msg.Body[idx:]
		} else {
			msg.Body += footerHTML
		}
	}

	// Only extend an existing plain-text part. When Plain is empty the
	// providers fall back to Body (which already carries the footer), so we
	// avoid clobbering the body content with a footer-only plain part.
	if msg.Plain != "" {
		msg.Plain += "\n\n---\nUnsubscribe from these emails: " + link + "\n"
	}
}

// SendBatch delivers multiple notifications and logs each result.
func (s *Service) SendBatch(ctx context.Context, eventID, invitationID string, ch Channel, msgs []*Message) []error {
	provider, err := s.registry.Get(ch)
	if err != nil {
		errs := make([]error, len(msgs))
		for i := range errs {
			errs[i] = fmt.Errorf("get provider: %w", err)
		}
		return errs
	}

	now := time.Now().UTC()
	logIDs := make([]string, len(msgs))

	// Insert pending log entries for each message, applying the unsubscribe
	// footer and open-tracking pixel for email messages.
	for i, msg := range msgs {
		logIDs[i] = uuid.Must(uuid.NewV7()).String()
		if ch == ChannelEmail {
			s.applyUnsubscribeFooter(ctx, msg, eventID)
			s.embedOpenPixel(msg, logIDs[i])
		}
		if err := s.insertLog(ctx, logIDs[i], eventID, invitationID, string(ch), provider.Name(), "pending", "", msg.To, msg.Subject, nil, now); err != nil {
			s.logger.Error().Err(err).Int("index", i).Msg("failed to insert notification log")
		}
	}

	// Attempt batch delivery.
	results, errs := provider.SendBatch(ctx, msgs)

	// Update log entries with results.
	for i, sendErr := range errs {
		if sendErr != nil {
			if err := s.updateLog(ctx, logIDs[i], "failed", sendErr.Error(), "", nil); err != nil {
				s.logger.Error().Err(err).Int("index", i).Msg("failed to update notification log")
			}
		} else {
			sentAt := time.Now().UTC()
			var messageID string
			if results[i] != nil {
				messageID = results[i].MessageID
			}
			if err := s.updateLog(ctx, logIDs[i], "sent", "", messageID, &sentAt); err != nil {
				s.logger.Error().Err(err).Int("index", i).Msg("failed to update notification log")
			}
		}
	}

	return errs
}

// insertLog creates a new notification_log row.
func (s *Service) insertLog(ctx context.Context, id, eventID, invitationID, channel, provider, status, errText, recipient, subject string, sentAt *time.Time, createdAt time.Time) error {
	var sentAtStr sql.NullString
	if sentAt != nil {
		sentAtStr = sql.NullString{String: sentAt.UTC().Format(time.RFC3339), Valid: true}
	}

	var invitationRef any
	if invitationID != "" {
		invitationRef = invitationID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notification_log (id, event_id, invitation_id, channel, provider, status, error, recipient, subject, sent_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, eventID, invitationRef, channel, provider, status, errText, recipient, subject, sentAtStr, createdAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert notification log: %w", err)
	}

	return nil
}

// updateLog updates the status, error, message_id, and sent_at of a notification_log row.
func (s *Service) updateLog(ctx context.Context, id, status, errText, messageID string, sentAt *time.Time) error {
	var sentAtStr sql.NullString
	if sentAt != nil {
		sentAtStr = sql.NullString{String: sentAt.UTC().Format(time.RFC3339), Valid: true}
	}

	var msgIDStr sql.NullString
	if messageID != "" {
		msgIDStr = sql.NullString{String: messageID, Valid: true}
	}

	_, err := s.db.ExecContext(ctx,
		`UPDATE notification_log SET status = ?, error = ?, message_id = ?, sent_at = ? WHERE id = ?`,
		status, errText, msgIDStr, sentAtStr, id,
	)
	if err != nil {
		return fmt.Errorf("update notification log: %w", err)
	}

	return nil
}

// GetLogByID returns a single notification log entry.
func (s *Service) GetLogByID(ctx context.Context, id string) (*LogEntry, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, event_id, invitation_id, channel, provider, status, delivery_status, error,
		        recipient, subject, message_id, sent_at, delivered_at, opened_at, clicked_at,
		        bounced_at, bounce_type, complaint_at, created_at
		 FROM notification_log WHERE id = ?`, id)
	return scanLogEntry(row)
}

// GetLogsByEvent returns all notification log entries for an event.
func (s *Service) GetLogsByEvent(ctx context.Context, eventID string) ([]*LogEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event_id, invitation_id, channel, provider, status, delivery_status, error,
		        recipient, subject, message_id, sent_at, delivered_at, opened_at, clicked_at,
		        bounced_at, bounce_type, complaint_at, created_at
		 FROM notification_log WHERE event_id = ? ORDER BY created_at DESC`, eventID)
	if err != nil {
		return nil, fmt.Errorf("query notification logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []*LogEntry
	for rows.Next() {
		entry, err := scanLogEntryRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func scanLogEntry(row *sql.Row) (*LogEntry, error) {
	var e LogEntry
	var sentAt, deliveredAt, openedAt, clickedAt, bouncedAt, complaintAt sql.NullString
	var messageID, bounceType, errText, invitationID sql.NullString
	var createdAtStr string

	// created_at is RFC3339 TEXT: scan into a string then parse. lib/pq cannot
	// convert a TEXT column straight into time.Time (go-sqlite3 silently does).
	// invitation_id is nullable (ON DELETE SET NULL), so scan via NullString.
	err := row.Scan(&e.ID, &e.EventID, &invitationID, &e.Channel, &e.Provider,
		&e.Status, &e.DeliveryStatus, &errText,
		&e.Recipient, &e.Subject, &messageID, &sentAt, &deliveredAt, &openedAt, &clickedAt,
		&bouncedAt, &bounceType, &complaintAt, &createdAtStr)
	if err != nil {
		return nil, err
	}
	e.InvitationID = invitationID.String
	e.Error = errText.String
	e.MessageID = messageID.String
	e.BounceType = bounceType.String
	e.SentAt = parseNullTime(sentAt)
	e.DeliveredAt = parseNullTime(deliveredAt)
	e.OpenedAt = parseNullTime(openedAt)
	e.ClickedAt = parseNullTime(clickedAt)
	e.BouncedAt = parseNullTime(bouncedAt)
	e.ComplaintAt = parseNullTime(complaintAt)
	if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		e.CreatedAt = t
	}
	return &e, nil
}

func scanLogEntryRow(rows *sql.Rows) (*LogEntry, error) {
	var e LogEntry
	var sentAt, deliveredAt, openedAt, clickedAt, bouncedAt, complaintAt sql.NullString
	var messageID, bounceType, errText, invitationID sql.NullString
	var createdAtStr string

	err := rows.Scan(&e.ID, &e.EventID, &invitationID, &e.Channel, &e.Provider,
		&e.Status, &e.DeliveryStatus, &errText,
		&e.Recipient, &e.Subject, &messageID, &sentAt, &deliveredAt, &openedAt, &clickedAt,
		&bouncedAt, &bounceType, &complaintAt, &createdAtStr)
	if err != nil {
		return nil, err
	}
	e.InvitationID = invitationID.String
	e.Error = errText.String
	e.MessageID = messageID.String
	e.BounceType = bounceType.String
	e.SentAt = parseNullTime(sentAt)
	e.DeliveredAt = parseNullTime(deliveredAt)
	e.OpenedAt = parseNullTime(openedAt)
	e.ClickedAt = parseNullTime(clickedAt)
	e.BouncedAt = parseNullTime(bouncedAt)
	e.ComplaintAt = parseNullTime(complaintAt)
	if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		e.CreatedAt = t
	}
	return &e, nil
}

func parseNullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, ns.String)
	if err != nil {
		return nil
	}
	return &t
}

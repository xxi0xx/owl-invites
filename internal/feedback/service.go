package feedback

import (
	"context"
	"fmt"
	"html"

	"github.com/rs/zerolog/log"

	"github.com/xxi0xx/owl-invites/internal/notification/templates"
)

// Service orchestrates feedback submission via GitHub Issues or email fallback.
type Service struct {
	githubToken   string
	githubRepo    string
	feedbackEmail string
	sendEmail     func(ctx context.Context, to, subject, body, plain string) error
}

// NewService creates a new feedback Service.
func NewService(githubToken, githubRepo, feedbackEmail string) *Service {
	return &Service{
		githubToken:   githubToken,
		githubRepo:    githubRepo,
		feedbackEmail: feedbackEmail,
	}
}

// SetEmailSender injects an email-sending function (breaks circular dependency).
func (s *Service) SetEmailSender(fn func(ctx context.Context, to, subject, body, plain string) error) {
	s.sendEmail = fn
}

// Submit sends feedback via GitHub Issues (preferred) or email (fallback).
// If allowFollowUp is true and sendEmail is configured, a confirmation is sent
// to organizerEmail.
func (s *Service) Submit(ctx context.Context, organizerEmail, feedbackType, message string, allowFollowUp bool) error {
	var submitErr error
	if s.githubToken != "" && s.githubRepo != "" {
		submitErr = s.submitGitHub(ctx, organizerEmail, feedbackType, message)
	} else if s.sendEmail != nil && s.feedbackEmail != "" {
		submitErr = s.submitEmail(ctx, organizerEmail, feedbackType, message)
	} else {
		log.Info().
			Str("from", organizerEmail).
			Str("type", feedbackType).
			Str("message", message).
			Msg("feedback received (no external channel configured)")
	}

	if submitErr != nil {
		return submitErr
	}

	// Send confirmation to the submitter if they opted in and email is available.
	if allowFollowUp && s.sendEmail != nil && organizerEmail != "" {
		htmlBody, plain, err := templates.RenderFeedbackConfirmation(feedbackType, true)
		if err != nil {
			log.Error().Err(err).Msg("failed to render feedback confirmation template")
			return nil
		}
		if err := s.sendEmail(ctx, organizerEmail, "We received your feedback — Owl Invites", htmlBody, plain); err != nil {
			log.Error().Err(err).Str("email", organizerEmail).Msg("failed to send feedback confirmation email")
		}
	}

	return nil
}

func (s *Service) submitGitHub(ctx context.Context, organizerEmail, feedbackType, message string) error {
	title := fmt.Sprintf("[Feedback - %s] %s", feedbackType, truncate(message, 80))
	body := fmt.Sprintf("**Type:** %s\n**From:** %s\n\n---\n\n%s", feedbackType, organizerEmail, message)
	labels := []string{"feedback", feedbackType}

	return createGitHubIssue(ctx, s.githubToken, s.githubRepo, title, body, labels)
}

// SubmitGuest sends feedback from an unauthenticated guest (e.g. someone
// hitting a bug on a public invite page). The submitter has no organizer
// identity; contactEmail and source are optional. Issues are labeled
// "guest feedback" so they are distinguishable from organizer feedback.
func (s *Service) SubmitGuest(ctx context.Context, message, contactEmail, source string) error {
	if s.githubToken != "" && s.githubRepo != "" {
		return s.submitGuestGitHub(ctx, message, contactEmail, source)
	}
	if s.sendEmail != nil && s.feedbackEmail != "" {
		return s.submitGuestEmail(ctx, message, contactEmail, source)
	}
	log.Info().
		Str("contact", contactEmail).
		Str("source", source).
		Str("message", message).
		Msg("guest feedback received (no external channel configured)")
	return nil
}

func (s *Service) submitGuestGitHub(ctx context.Context, message, contactEmail, source string) error {
	contact := contactEmail
	if contact == "" {
		contact = "(anonymous)"
	}
	src := source
	if src == "" {
		src = "(unknown)"
	}
	title := fmt.Sprintf("[Guest Feedback] %s", truncate(message, 80))
	body := fmt.Sprintf("**From:** guest\n**Contact:** %s\n**Source:** %s\n\n---\n\n%s", contact, src, message)
	labels := []string{"feedback", "guest feedback"}

	return createGitHubIssue(ctx, s.githubToken, s.githubRepo, title, body, labels)
}

func (s *Service) submitGuestEmail(ctx context.Context, message, contactEmail, source string) error {
	subject := fmt.Sprintf("[Owl Invites Guest Feedback] %s", truncate(message, 60))
	contact := contactEmail
	if contact == "" {
		contact = "(anonymous)"
	}
	src := source
	if src == "" {
		src = "(unknown)"
	}
	plain := fmt.Sprintf("From: guest\nContact: %s\nSource: %s\n\n%s", contact, src, message)
	// HTML-escape every guest-controlled field before interpolating into the
	// email body, since it is rendered in the operator's mail client.
	htmlBody := fmt.Sprintf(
		"<p><strong>From:</strong> guest</p><p><strong>Contact:</strong> %s</p><p><strong>Source:</strong> %s</p><hr><pre>%s</pre>",
		html.EscapeString(contact),
		html.EscapeString(src),
		html.EscapeString(message),
	)

	return s.sendEmail(ctx, s.feedbackEmail, subject, htmlBody, plain)
}

func (s *Service) submitEmail(ctx context.Context, organizerEmail, feedbackType, message string) error {
	subject := fmt.Sprintf("[Owl Invites Feedback - %s] %s", feedbackType, truncate(message, 60))
	plain := fmt.Sprintf("Type: %s\nFrom: %s\n\n%s", feedbackType, organizerEmail, message)
	// HTML-escape every user/organizer-controlled field before interpolating
	// into the email body. The message originates from an authenticated
	// organizer but is rendered in the operator's mail client, so an
	// unescaped <script> or <img onerror=...> would execute there.
	htmlBody := fmt.Sprintf(
		"<p><strong>Type:</strong> %s</p><p><strong>From:</strong> %s</p><hr><pre>%s</pre>",
		html.EscapeString(feedbackType),
		html.EscapeString(organizerEmail),
		html.EscapeString(message),
	)

	return s.sendEmail(ctx, s.feedbackEmail, subject, htmlBody, plain)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

package templates

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

// Gate 2 embeds only templates backed by active application behavior. Legacy
// attendee RSVP, lookup, waitlist, and attendee-message templates were removed
// with their authorization model.
//
//go:embed magic_link.html retention_warning.html feedback_confirmation.html cohost_invitation.html household_invitation.html
var templateFS embed.FS

var (
	magicLinkTmpl            = template.Must(template.ParseFS(templateFS, "magic_link.html"))
	retentionWarningTmpl     = template.Must(template.ParseFS(templateFS, "retention_warning.html"))
	feedbackConfirmationTmpl = template.Must(template.ParseFS(templateFS, "feedback_confirmation.html"))
	cohostInvitationTmpl     = template.Must(template.ParseFS(templateFS, "cohost_invitation.html"))
	householdInvitationTmpl  = template.Must(
		template.ParseFS(templateFS, "household_invitation.html"),
	)
)

type magicLinkData struct {
	URL           string
	ExpiryMinutes int
	Colors        EmailColors
}

func RenderMagicLink(baseURL, token string, expiryMinutes int) (html, plain string, err error) {
	url := fmt.Sprintf("%s/auth/verify?token=%s", baseURL, token)
	data := magicLinkData{URL: url, ExpiryMinutes: expiryMinutes, Colors: DefaultEmailColors()}

	var buf bytes.Buffer
	if err := magicLinkTmpl.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("render magic link template: %w", err)
	}
	plain = fmt.Sprintf("Sign in to Owl Invites\n\nClick the link below to sign in:\n%s\n\nThis link expires in %d minutes.\n\nIf you did not request this link, you can safely ignore this email.", url, expiryMinutes)
	return buf.String(), plain, nil
}

type householdInvitationData struct {
	EventTitle    string
	EventDate     string
	Location      string
	InvitationURL string
	Colors        EmailColors
}

func RenderHouseholdInvitation(
	eventTitle,
	eventDate,
	location,
	invitationURL string,
) (htmlBody, plain string, err error) {
	data := householdInvitationData{
		EventTitle:    eventTitle,
		EventDate:     eventDate,
		Location:      location,
		InvitationURL: invitationURL,
		Colors:        DefaultEmailColors(),
	}

	var buf bytes.Buffer
	if err := householdInvitationTmpl.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf(
			"render household invitation template: %w",
			err,
		)
	}

	var text strings.Builder

	text.WriteString("You're invited\n\n")
	text.WriteString(fmt.Sprintf(
		"You're invited to %s.\n\n",
		eventTitle,
	))

	text.WriteString(
		"View your invitation for the event details and RSVP for your household.\n\n",
	)

	if eventDate != "" {
		text.WriteString(fmt.Sprintf("Date: %s\n", eventDate))
	}

	if location != "" {
		text.WriteString(fmt.Sprintf("Location: %s\n", location))
	}

	if eventDate != "" || location != "" {
		text.WriteString("\n")
	}

	text.WriteString("View invitation & RSVP:\n")
	text.WriteString(invitationURL)
	text.WriteString("\n\n")

	text.WriteString(
		"This invitation link is unique to your household. Please don't share it.\n",
	)

	text.WriteString(
		"If you weren't expecting this invitation, you can safely ignore this email.\n",
	)

	return buf.String(), text.String(), nil
}

type retentionWarningData struct {
	EventTitle   string
	ExpiresAt    string
	DashboardURL string
	Colors       EmailColors
}

func RenderRetentionWarning(eventTitle, expiresAt, dashboardURL string) (html, plain string, err error) {
	data := retentionWarningData{EventTitle: eventTitle, ExpiresAt: expiresAt, DashboardURL: dashboardURL, Colors: DefaultEmailColors()}
	var buf bytes.Buffer
	if err := retentionWarningTmpl.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("render retention warning template: %w", err)
	}

	var text strings.Builder
	text.WriteString("Data Retention Notice\n\n")
	text.WriteString(fmt.Sprintf("Your event %q is scheduled for automatic deletion on %s.\n\n", eventTitle, expiresAt))
	text.WriteString("After this date, all event data including invitations, guest responses, and invitation messages will be permanently deleted.\n\n")
	if dashboardURL != "" {
		text.WriteString(fmt.Sprintf("To extend the retention period, visit:\n%s\n", dashboardURL))
	}
	text.WriteString("\nThis is an automated notice from Owl Invites.\n")
	return buf.String(), text.String(), nil
}

type feedbackConfirmationData struct {
	FeedbackType  string
	AllowFollowUp bool
	Colors        EmailColors
}

func RenderFeedbackConfirmation(feedbackType string, allowFollowUp bool) (htmlBody, plain string, err error) {
	data := feedbackConfirmationData{FeedbackType: feedbackType, AllowFollowUp: allowFollowUp, Colors: DefaultEmailColors()}
	var buf bytes.Buffer
	if err := feedbackConfirmationTmpl.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("render feedback confirmation template: %w", err)
	}

	var text strings.Builder
	text.WriteString("Thanks for your feedback!\n\n")
	text.WriteString(fmt.Sprintf("We received your %s submission and appreciate you taking the time to share it with us.\n\n", feedbackType))
	if allowFollowUp {
		text.WriteString("Since you opted in to follow-up contact, we may reach out to you if we have questions or updates related to your feedback.\n\n")
	}
	text.WriteString("Your feedback helps make Owl Invites better for everyone.\n")
	return buf.String(), text.String(), nil
}

type cohostInvitationData struct {
	EventTitle   string
	EventDate    string
	Location     string
	AddedByName  string
	DashboardURL string
	Colors       EmailColors
}

func RenderCoHostInvitation(eventTitle, eventDate, location, addedByName, dashboardURL string) (html, plain string, err error) {
	data := cohostInvitationData{EventTitle: eventTitle, EventDate: eventDate, Location: location, AddedByName: addedByName, DashboardURL: dashboardURL, Colors: DefaultEmailColors()}
	var buf bytes.Buffer
	if err := cohostInvitationTmpl.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("render cohost invitation template: %w", err)
	}

	var text strings.Builder
	text.WriteString("You've Been Added as a Co-Host\n\n")
	text.WriteString(fmt.Sprintf("%s has added you as a co-host for %s.\n\n", addedByName, eventTitle))
	text.WriteString(fmt.Sprintf("Event: %s\nDate: %s\nLocation: %s\n\n", eventTitle, eventDate, location))
	text.WriteString(fmt.Sprintf("View the event dashboard:\n%s\n", dashboardURL))
	text.WriteString("\nSent by Owl Invites.\n")
	return buf.String(), text.String(), nil
}

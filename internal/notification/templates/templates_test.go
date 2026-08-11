package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepresentativeEmailsUseOnlyOwlInvitesBranding(t *testing.T) {
	rendered := make([]string, 0, 8)
	for _, render := range []func() (string, string, error){
		func() (string, string, error) { return RenderMagicLink("https://invites.example", "token", 15) },
		func() (string, string, error) {
			return RenderRetentionWarning("Party", "tomorrow", "https://invites.example/events")
		},
		func() (string, string, error) { return RenderFeedbackConfirmation("feature", true) },
		func() (string, string, error) {
			return RenderCoHostInvitation("Party", "tomorrow", "Town Hall", "Alex", "https://invites.example/events/1")
		},
	} {
		htmlBody, plainBody, err := render()
		require.NoError(t, err)
		rendered = append(rendered, htmlBody, plainBody)
	}
	for _, body := range rendered {
		assert.Contains(t, body, "Owl Invites")
		assert.NotContains(t, body, "OpenRSVP")
		assert.NotContains(t, strings.ToLower(body), "openrsvp")
	}
}

func TestRenderRetentionWarning(t *testing.T) {
	html, plain, err := RenderRetentionWarning("Birthday Party", "March 15, 2026", "http://localhost:8080/events")
	require.NoError(t, err)

	assert.Contains(t, html, "Birthday Party")
	assert.Contains(t, html, "March 15, 2026")
	assert.Contains(t, html, "http://localhost:8080/events")
	assert.Contains(t, html, "Data Retention Notice")

	assert.Contains(t, plain, "Birthday Party")
	assert.Contains(t, plain, "March 15, 2026")
	assert.Contains(t, plain, "http://localhost:8080/events")
	assert.Contains(t, plain, "permanently deleted")
}

func TestRenderRetentionWarningNoDashboardURL(t *testing.T) {
	html, plain, err := RenderRetentionWarning("Garden Party", "April 20, 2026", "")
	require.NoError(t, err)

	assert.Contains(t, html, "Garden Party")
	assert.Contains(t, html, "April 20, 2026")
	assert.NotContains(t, html, "View Event")

	assert.Contains(t, plain, "Garden Party")
	assert.NotContains(t, plain, "visit:")
}

func TestRenderMagicLink(t *testing.T) {
	html, plain, err := RenderMagicLink("http://localhost:8080", "abc123token", 15)
	require.NoError(t, err)

	assert.Contains(t, html, "abc123token")
	assert.Contains(t, html, "http://localhost:8080")
	assert.Contains(t, plain, "15 minutes")
}

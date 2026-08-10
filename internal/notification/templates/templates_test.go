package templates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

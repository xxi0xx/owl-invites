package invitation

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilityDomainsAndVersionsAreIsolated(t *testing.T) {
	signer, err := NewCapabilitySigner(strings.Repeat("k", 32))
	require.NoError(t, err)

	private := signer.Private("opaque-selector", 3)
	accessID, version, err := signer.ParsePrivate(private)
	require.NoError(t, err)
	assert.Equal(t, "opaque-selector", accessID)
	assert.Equal(t, 3, version)

	_, _, err = signer.ParseOpen(private)
	assert.ErrorIs(t, err, ErrInvalidCapability)
	_, _, err = signer.ParsePrivate(signer.Private("opaque-selector", 4))
	require.NoError(t, err)

	tampered := private[:len(private)-1] + "A"
	_, _, err = signer.ParsePrivate(tampered)
	assert.ErrorIs(t, err, ErrInvalidCapability)
}

func TestCapabilitySignerRequiresRestoreGradeSecret(t *testing.T) {
	_, err := NewCapabilitySigner("too-short")
	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrInvalidCapability))
}

func TestRandomTokenAndHashDoNotExposeRawMaterial(t *testing.T) {
	raw, err := randomToken(32)
	require.NoError(t, err)
	assert.NotEmpty(t, raw)
	assert.NotContains(t, hashToken(raw), raw)
}

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadNotificationProviderEnv(t *testing.T) {
	setInvitationSecret(t)
	t.Setenv("PORT", "9090")
	t.Setenv("ENV", "development")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", "openrsvp.db")
	t.Setenv("NOTIFICATION_EMAIL_PROVIDER", "sendgrid")
	t.Setenv("SENDGRID_API_KEY", "SG.test")
	t.Setenv("SENDGRID_FROM", "sendgrid@example.com")
	t.Setenv("NOTIFICATION_SMS_PROVIDER", "twilio")
	t.Setenv("TWILIO_ACCOUNT_SID", "AC123")
	t.Setenv("TWILIO_AUTH_TOKEN", "token")
	t.Setenv("TWILIO_FROM_NUMBER", "+15551234567")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "sendgrid", cfg.NotificationEmailProvider)
	assert.Equal(t, "SG.test", cfg.SendGridAPIKey)
	assert.Equal(t, "sendgrid@example.com", cfg.SendGridFrom)
	assert.Equal(t, "twilio", cfg.NotificationSMSProvider)
	assert.Equal(t, "AC123", cfg.TwilioAccountSID)
	assert.Equal(t, "token", cfg.TwilioAuthToken)
	assert.Equal(t, "+15551234567", cfg.TwilioFromNumber)
}

func TestLoadSESEnv(t *testing.T) {
	setInvitationSecret(t)
	t.Setenv("PORT", "8080")
	t.Setenv("ENV", "production")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", "openrsvp.db")
	t.Setenv("NOTIFICATION_EMAIL_PROVIDER", "ses")
	t.Setenv("SES_REGION", "us-east-1")
	t.Setenv("SES_USERNAME", "ses-user")
	t.Setenv("SES_PASSWORD", "ses-pass")
	t.Setenv("SES_FROM", "ses@example.com")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "ses", cfg.NotificationEmailProvider)
	assert.Equal(t, "us-east-1", cfg.SESRegion)
	assert.Equal(t, "ses-user", cfg.SESUsername)
	assert.Equal(t, "ses-pass", cfg.SESPassword)
	assert.Equal(t, "ses@example.com", cfg.SESFrom)
}

func TestAllowSignupsDefaultsOff(t *testing.T) {
	setInvitationSecret(t)
	t.Setenv("ALLOW_SIGNUPS", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.AllowSignups)
}

func TestAllowSignupsCanBeExplicitlyEnabled(t *testing.T) {
	setInvitationSecret(t)
	t.Setenv("ALLOW_SIGNUPS", "true")

	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.AllowSignups)
}

func TestBootstrapTokenUsesOwlInvitesEnvironmentKey(t *testing.T) {
	setInvitationSecret(t)
	t.Setenv("OWL_INVITES_BOOTSTRAP_TOKEN", "operator-secret")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "operator-secret", cfg.BootstrapToken)
}

func TestAccountInviteExpiryUsesOwlInvitesEnvironmentKey(t *testing.T) {
	setInvitationSecret(t)
	t.Setenv("OWL_INVITES_ACCOUNT_INVITE_EXPIRY", "48h")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 48*time.Hour, cfg.AccountInviteExpiry)
}

func TestTrustedProxiesRejectsInvalidEntries(t *testing.T) {
	setInvitationSecret(t)
	t.Setenv("TRUSTED_PROXIES", "not-a-network")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid TRUSTED_PROXIES")
}

func TestTrustedProxiesAcceptsIPsAndCIDRs(t *testing.T) {
	setInvitationSecret(t)
	t.Setenv("TRUSTED_PROXIES", "127.0.0.1, 10.0.0.0/8, 2001:db8::/32")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"127.0.0.1", "10.0.0.0/8", "2001:db8::/32"}, cfg.TrustedProxies)
}

func TestInvitationSecretIsRequiredRestoreMaterial(t *testing.T) {
	t.Setenv("OWL_INVITES_SECRET_KEY", "short")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OWL_INVITES_SECRET_KEY")
}

func TestInvitationSecretLoadsFromFileAndTrimsOneLineEnding(t *testing.T) {
	unsetEnv(t, "OWL_INVITES_SECRET_KEY")
	secret := "file-backed-restore-secret-key-with-32-bytes"
	path := filepath.Join(t.TempDir(), "owl-invites-secret")
	require.NoError(t, os.WriteFile(path, []byte(secret+"\r\n"), 0o600))
	t.Setenv("OWL_INVITES_SECRET_KEY_FILE", path)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, secret, cfg.InvitationSecretKey)
}

func TestInvitationSecretRejectsBothSources(t *testing.T) {
	t.Setenv("OWL_INVITES_SECRET_KEY", "direct-secret-key-material-at-least-32-bytes")
	t.Setenv("OWL_INVITES_SECRET_KEY_FILE", filepath.Join(t.TempDir(), "secret"))

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")
	assert.NotContains(t, err.Error(), "direct-secret-key-material")
}

func TestInvitationSecretRejectsMissingFileWithoutLeakingMaterial(t *testing.T) {
	unsetEnv(t, "OWL_INVITES_SECRET_KEY")
	path := filepath.Join(t.TempDir(), "missing-secret")
	t.Setenv("OWL_INVITES_SECRET_KEY_FILE", path)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read OWL_INVITES_SECRET_KEY_FILE")
}

func TestInvitationSecretRejectsProductionPlaceholder(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("OWL_INVITES_SECRET_KEY", "change-me-to-a-real-production-secret-key")
	unsetEnv(t, "OWL_INVITES_SECRET_KEY_FILE")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "placeholder")
	assert.NotContains(t, err.Error(), "change-me")
}

func TestInvitationSecretFilePreservesNonNewlineWhitespace(t *testing.T) {
	got := trimOneTrailingLineEnding("  secret material with deliberate spaces  \n")
	assert.Equal(t, "  secret material with deliberate spaces  ", got)
	assert.Equal(t, "secret\n", trimOneTrailingLineEnding("secret\n\n"))
}

func setInvitationSecret(t *testing.T) {
	t.Helper()
	t.Setenv("OWL_INVITES_SECRET_KEY", "d2c7b318a9454f11a08be6392f64d38f7de95568")
	unsetEnv(t, "OWL_INVITES_SECRET_KEY_FILE")
}

func unsetEnv(t *testing.T, name string) {
	t.Helper()
	value, existed := os.LookupEnv(name)
	require.NoError(t, os.Unsetenv(name))
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateCommandsSQLite(t *testing.T) {
	unsetTestEnv(t, "OWL_INVITES_SECRET_KEY_FILE")
	t.Setenv("ENV", "development")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", filepath.Join(t.TempDir(), "operator-migrate.db"))
	t.Setenv("OWL_INVITES_SECRET_KEY", "migration-test-secret-key-at-least-32-bytes")

	runCommand := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err != nil {
			t.Fatalf("run %v: %v (stderr=%q)", args, err, stderr.String())
		}
		return stdout.String()
	}

	if got := runCommand("migrate", "version"); got != "0\n" {
		t.Fatalf("fresh version output = %q, want 0", got)
	}
	if got := runCommand("migrate", "up"); !strings.Contains(got, "migrated schema from 0 to 36") {
		t.Fatalf("migrate up output = %q", got)
	}
	if got := runCommand("migrate", "version"); got != "36\n" {
		t.Fatalf("migrated version output = %q, want 36", got)
	}
	status := runCommand("migrate", "status")
	for _, want := range []string{"current=36", "latest=36", "dirty=false", "pending=false"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status output %q does not contain %q", status, want)
		}
	}
	if got := runCommand("migrate", "up"); got != "schema already at version 36\n" {
		t.Fatalf("no-op migrate output = %q", got)
	}
}

func TestMigrateCommandRejectsUnsupportedActionBeforeConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"migrate", "down"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "status|version|up") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSecretFingerprintCommandDoesNotPrintSecret(t *testing.T) {
	const secret = "operator-fingerprint-secret-material-32-bytes"
	unsetTestEnv(t, "OWL_INVITES_SECRET_KEY_FILE")
	t.Setenv("ENV", "development")
	t.Setenv("OWL_INVITES_SECRET_KEY", secret)

	var stdout, stderr bytes.Buffer
	if err := run([]string{"secret", "fingerprint"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "oi-secret-v1:") {
		t.Fatalf("unexpected fingerprint output: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), secret) {
		t.Fatal("fingerprint output leaked the secret")
	}

	stdout.Reset()
	stderr.Reset()
	t.Setenv("OWL_INVITES_SECRET_KEY_FILE", filepath.Join(t.TempDir(), "unused"))
	if err := run([]string{"secret", "fingerprint"}, &stdout, &stderr); err == nil {
		t.Fatal("expected both-set configuration to fail")
	}
	if strings.Contains(stdout.String()+stderr.String(), secret) {
		t.Fatal("failed configuration leaked the secret")
	}
}

func unsetTestEnv(t *testing.T, name string) {
	t.Helper()
	value, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

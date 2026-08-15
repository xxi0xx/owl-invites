## Codex Windows environment

The primary local Codex development environment for Owl Invites is
Windows-native Codex.

The user's integrated terminal may use WSL, but Codex agent commands should
be treated as native Windows/PowerShell commands.

### Environment setup

Repository dependencies are prepared with:

    powershell -NoProfile -ExecutionPolicy Bypass `
      -File scripts\codex-environment-setup.ps1

The expected environment includes:

- Go 1.26.5
- Node.js 22
- npm
- CGO with MSYS2 UCRT64 GCC
- Playwright Chromium
- govulncheck 1.1.4
- golangci-lint 2.12.2

### Required preflight

Before substantive repository work, run:

    powershell -NoProfile -ExecutionPolicy Bypass `
      -File scripts\codex-preflight.ps1

For work requiring an initially clean tree:

    powershell -NoProfile -ExecutionPolicy Bypass `
      -File scripts\codex-preflight.ps1 `
      -RequireCleanWorktree

If preflight passes:

- treat the local development environment as ready;
- do not reinstall dependencies;
- do not change toolchain versions;
- do not run winget, npm install, go install, pacman, or other package
  management commands speculatively;
- do not investigate the host environment without new concrete evidence that
  it is broken.

If preflight fails:

- identify the exact failing prerequisite;
- do not perform broad environment troubleshooting;
- repair only the failed prerequisite when environment repair is explicitly
  within task scope;
- otherwise report the limitation and stop only the affected validation.

### Running commands

For commands that depend on the canonical Codex environment, prefer:

    powershell -NoProfile -ExecutionPolicy Bypass `
      -File scripts\codex-run.ps1 `
      <command> <arguments>

### Optional infrastructure

Docker and a local PostgreSQL server are optional for ordinary source,
implementation, and security-review work.

If Docker, PostgreSQL, or another optional integration service is unavailable:

- do not attempt Docker-in-Docker or nested virtualization;
- do not spend task time repairing optional infrastructure;
- continue with validations that do not require it;
- record unavailable dynamic validation accurately.

GitHub Actions remains authoritative for complete release verification,
including:

- SQLite full, race, and migration testing;
- PostgreSQL 16 full, race, and migration testing;
- PostgreSQL destructive backup/restore;
- Chromium + Mailpit acceptance;
- production-container hardening;
- amd64 and arm64 container builds;
- release-policy verification.

Do not recreate full GitHub Actions infrastructure merely to prove the local
Codex environment works.

### Codex Security

The repository preflight does not replace the mandatory Codex Security
plugin preflight.

For Deep Security Scan:

1. run the repository preflight;
2. require it to pass;
3. run the Codex Security capability preflight;
4. require threat-model, validation, and attack-path capabilities;
5. only then begin Deep discovery.

Do not substitute ad hoc searching or a shallow security scan for a failed
Deep Security Scan preflight.

### Network and package management

Dependency downloads belong primarily in environment setup.

Once preflight passes:

- avoid speculative package downloads;
- avoid unnecessary network access;
- install or update dependencies only when the task genuinely requires it.

### Secrets

Never place production secrets in the Codex development environment.

Do not store:

- Resend credentials;
- Cloudflare credentials;
- production Owl Invites capability keys;
- bootstrap tokens;
- live invitation capabilities;
- production databases containing real guest data.

Use synthetic test credentials and local test services instead.
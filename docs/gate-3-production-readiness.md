# Gate 3 production-readiness architecture

Gate 3 establishes Owl Invites' operational trust boundary. It does not make a
stable product release, begin the broad naming migration, or change the Gate 2
Invitation/Guest/RSVP model.

The required chain is:

```text
source commit -> authoritative verification -> identified immutable artifact
              -> SBOM/provenance -> hardened deployment -> verified backup
              -> tested restore -> old household capability still works
```

Operator procedures are split by task:

- [Production deployment](operations/production-deployment.md)
- [Backup, restore, and disaster recovery](operations/backup-restore.md)
- [Release integrity](operations/releases.md)

## Operational boundaries

- `.github/workflows/verify.yml` is the one product gate used by normal CI and
  release CI. Verification has read-only repository permissions.
- Release tags must be exact stable `vX.Y.Z` tags whose commit is contained in
  `origin/main`. No `latest`, major-only, or major/minor aliases are emitted.
- An explicitly requested review publication may emit only
  `sha-<full-commit>` after the same gate; it is never a stable release.
- The canonical deployment identity is
  `ghcr.io/xxi0xx/owl-invites@sha256:<digest>`, not a tag.
- Both shipped binaries carry version, full commit, and build state from the
  same `scripts/build-go.sh` injection path. `/health` exposes that non-secret
  identity and `owl-invites version --json` is available inside the image.
- SQLite recovery bundles contain a SQLite-native consistent database
  snapshot, upload snapshot, checksums, schema/build metadata, and only a
  domain-separated fingerprint of `OWL_INVITES_SECRET_KEY`.
- PostgreSQL recovery uses `pg_dump`/`pg_restore`; uploads and key custody stay
  separate.
- Production runtime is UID/GID 10001, capability-free, no-new-privileges,
  read-only except `/data` and bounded `/tmp`, with no published host port in
  the production Compose example.
- The architecture-independent frontend is compiled once on BuildKit's
  `$BUILDPLATFORM`; target-platform stages retain native CGO compilation. CI
  loads both amd64 and arm64 images and proves each serves the embedded
  frontend and reports the exact expected build identity.

## Schema and migration policy

Gate 3 adds no schema migration. Server startup continues to apply pending
migrations for compatibility. Operators also have an explicit pre-deployment
path:

```bash
owl-invites migrate status
owl-invites migrate version
owl-invites migrate up
```

`status` reports installed/latest/dirty/pending state, `version` prints only
the installed integer, and `up` reports the actual transition or a no-op.
There is deliberately no down or force command.

Migration 36 remains a one-way Gate 2 cutover. Its down scripts and application
migrator fail before mutation. Rollback across it means stopping the new
application and restoring the complete verified pre-upgrade database, uploads,
and matching secret material before running the pre-Gate-2 application. Empty
legacy tables are not a rollback.

## Delivery and background jobs

Gate 2's persistence/delivery split remains intact. A committed invitation or
open enrollment is successful even when SMTP subsequently fails. Notification
delivery makes at most three in-process attempts (1s and 2s waits), records the
terminal result, and stops retrying on cancellation. Reminder claims use one
atomic conditional update so ordinary concurrent workers do not double-send.
Manual email delivery remains available; SMS-preferred households never fall
through to email.

No durable queue or generalized outbox was added. A process crash after a
reminder is claimed can still require operator inspection/manual action, and
in-process notification retries do not survive restart. Reliable asynchronous
delivery remains a separately reviewed future feature because storing pending
one-time recovery material would change the capability threat model.

## Known limitations

- No stable Owl Invites GHCR image exists during Gate 3.
- The SQLite database snapshot is transactional, but its following upload-tree
  copy is not transactionally atomic. Quiesce upload changes for the strongest
  cross-store recovery point.
- PostgreSQL backup orchestration intentionally remains standard operator
  tooling rather than an application-specific wrapper.
- Base image digests and Action SHAs require ongoing maintenance; Dependabot is
  configured for Docker, Actions, Go, and npm.
- Existing inherited OpenRSVP module, binary, service, database, and filesystem
  names remain until Gate 4.

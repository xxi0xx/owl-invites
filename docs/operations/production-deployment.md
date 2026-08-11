# Production deployment

> [!WARNING]
> Gate 4 is a redevelopment milestone, not a stable production release. No
> stable Owl Invites image has been published. Never substitute upstream
> OpenRSVP, `:latest`, a major-only tag, or a major/minor tag.

## Install inputs

A complete installation needs:

1. an image digest produced by the verified Owl Invites release workflow;
2. persistent `/data` storage (SQLite database and uploads), or an external
   PostgreSQL database plus persistent uploads;
3. a high-entropy `OWL_INVITES_SECRET_KEY` stored separately from ordinary
   backups;
4. external SMTP and a public HTTPS `BASE_URL`; and
5. a reverse proxy on the same private network as the application.

The prepared [production Compose example](../../docker-compose.production.yml)
accepts only the digest portion of the canonical image reference:

```bash
export OWL_INVITES_IMAGE_DIGEST='<64-character digest from release CI>'
export OWL_INVITES_SECRET_KEY_HOST_FILE=/secure/owl-invites-capability-key
export BASE_URL=https://invites.example.com
export TRUSTED_PROXIES=172.20.0.0/16
export SMTP_HOST=smtp.example.com
export SMTP_FROM=noreply@example.com
docker compose -f docker-compose.production.yml config --quiet
docker compose -f docker-compose.production.yml up -d
```

Generate the capability key once, protect it independently, and do not commit
it or place it in a database/upload archive:

```bash
umask 077
openssl rand -base64 48 > /secure/owl-invites-capability-key
```

The application removes one terminal LF or CRLF when loading a secret file.
Set exactly one of `OWL_INVITES_SECRET_KEY` and
`OWL_INVITES_SECRET_KEY_FILE`; setting both, even if one is empty, is an error.
Production rejects obvious example/placeholder values and values shorter than
32 bytes.

The Compose service publishes no host port. Attach the reverse proxy to its
network and proxy to `owl-invites:8080`. Set `TRUSTED_PROXIES` only to the
actual proxy network(s).

## Runtime controls

The production image and Compose example use:

- UID/GID 10001 and no root fallback;
- a read-only root filesystem;
- `cap_drop: ALL` and `no-new-privileges`;
- writable persistent `/data` only;
- a 16 MiB, noexec/nosuid/nodev `/tmp` tmpfs;
- a read-only secret-file mount under `/run/secrets`;
- no Docker socket and no host port publication;
- a readiness healthcheck and 35-second SIGTERM grace period; and
- bounded JSON log rotation.

The image's Node, Go, and Alpine bases are pinned by multi-architecture
manifest-list digest. Dependabot proposes reviewed digest updates.

## Prove what is running

```bash
docker compose -f docker-compose.production.yml exec owl-invites \
  owl-invites version --json
curl --fail http://owl-invites:8080/health
curl --fail http://owl-invites:8080/health/ready
```

`/health` is liveness and includes non-secret version/commit/build state.
`/health/ready` checks database reachability. During shutdown, readiness and
new work return 503 while liveness remains available until the process exits.
Database error details are logged privately, not returned by health endpoints.

Compare the reported commit with the image's OCI revision label and the digest
recorded by release CI. A tag alone is not proof of the running artifact.

## Upgrade sequence

Do not deploy first and hope automatic migrations succeed. The recommended
sequence is:

```text
verify a restorable backup
  -> verify separately protected secret material and fingerprint
  -> inspect migration status
  -> apply migrations explicitly
  -> start the candidate and check readiness
  -> cut traffic to the candidate
```

Example operator checks using the candidate binary/image and production
configuration:

```bash
owl-invites backup verify /backups/owl-invites-2026-08-10
owl-invites secret fingerprint
owl-invites migrate status
owl-invites migrate up
owl-invites migrate version
```

Server auto-migration remains enabled for compatibility, but it is not a
replacement for the explicit pre-deployment sequence.

Migration 36 cannot be rolled down. If an upgrade crossing it must be undone,
restore the verified pre-upgrade database, uploads, and matching key, then run
the prior application. See [backup and disaster recovery](backup-restore.md).

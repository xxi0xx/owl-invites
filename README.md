# Owl Invites

![Owl Invites owl and envelope mark](web/static/owl-invites-mark.svg)

Owl Invites is a self-hosted invitation and RSVP application for organizers and
their guests. Organizers create events and private households, invite co-hosts,
send email invitations, and manage responses. Guests use isolated household
capabilities or open enrollment without creating an account.

> [!WARNING]
> Owl Invites is in staged, pre-stable redevelopment. Gates 1–5 are engineering
> milestones, not a stable production release. Gate 5 is release-candidate
> evidence, not a production release. No stable image or semantic
> version is published. Review images, when available, are identified only by
> an immutable `sha-<full-commit>` tag and digest—never deploy `latest`.

## Current guarantees

- users and event memberships are the authoritative organizer identity model;
- invitations, assigned guests, RSVP responses, questions, and open enrollment
  use the isolated Gate 2 capability model;
- invitation delivery and recovery are email-only;
- household CSV import previews and revalidates explicit grouping before one
  atomic create-only commit; equal contacts never merge households;
- organizer reporting export reflects household, guest, attendance, response,
  and scoped-answer state while defanging spreadsheet formulas;
- the configured invitation card appears in the authorized guest household
  experience without exposing organizer-only data;
- organizers can search, filter, safely edit, explicitly deliver, rotate, and
  revoke household invitations, with useful delivery visibility;
- SQLite and PostgreSQL are tested, including race and migration suites;
- SQLite and PostgreSQL backup/restore drills preserve household capabilities;
- production containers run as UID/GID 10001 with a read-only root filesystem,
  dropped capabilities, bounded temporary storage, and readiness checks; and
- the release workflow builds amd64/arm64 images with SBOM, BuildKit provenance,
  and GitHub OIDC attestation.

## Repository identity

| Item | Canonical value |
| --- | --- |
| Product | Owl Invites |
| Repository | `xxi0xx/owl-invites` |
| Go module | `github.com/xxi0xx/owl-invites` |
| Binary and CLI | `owl-invites` |
| Container image | `ghcr.io/xxi0xx/owl-invites` |
| Fresh SQLite database | `/data/owl-invites.db` |

`RSVP`, `RSVPResponse`, `Invitation`, `Guest`, `Event`, and `User` remain normal
domain terminology. They are not legacy branding.

## Build and test

Required toolchains are Go 1.26.5+, Node.js 22, npm, a C compiler for SQLite,
and Docker/BuildKit for the production image.

```bash
cd web
npm ci
npm run check
npm run build
cd ..
make build
./bin/owl-invites version --json
```

The complete local product checks are:

```bash
bash scripts/lint-brand-residue.sh
bash scripts/lint-api-routes.sh
go vet ./...
go test ./...
go test -race ./...
cd web && npm run check && npm run build
```

PostgreSQL tests use `TEST_DATABASE_URL`. The authoritative GitHub Actions gate
also runs migration tests, Chromium/Mailpit flows, golangci-lint,
govulncheck, PostgreSQL recovery, and both production architectures.

## Run for development

Copy `.env.example` to `.env`, set a high-entropy
`OWL_INVITES_SECRET_KEY`, and supply the one-time bootstrap token before first
setup. Then run:

```bash
docker compose up --build
```

For the native binary:

```bash
make build
./bin/owl-invites
```

The same executable provides operator commands:

```bash
./bin/owl-invites version --json
./bin/owl-invites migrate status
./bin/owl-invites secret fingerprint
./bin/owl-invites backup verify /path/to/backup
./bin/owl-invites admin promote --email operator@example.com
```

## Supported product workflow

After first-run setup, an organizer can create and publish an event, configure
RSVP questions and its invitation card, then build households manually or
preview and commit an explicit-key CSV import. Import never sends email. The
organizer reviews the Invitations screen and deliberately delivers selected
households.

Guests follow the emailed private household capability, see the configured
card and only their household, record assigned/additional-guest attendance and
scoped answers, and update the response later through the household session or
email recovery. Organizers can search/filter the resulting state, send a
one-way targeted message or schedule a reminder, inspect honest provider
delivery status, and export a current-domain reporting CSV.

See [Gate 5 product maturation](docs/gate-5-product-maturation.md) for the exact
CSV columns and limits, create-only semantics, presentation security boundary,
delivery limitations, recovery guarantees, and deferred features.

Forward-looking priorities are maintained in the [product roadmap](ROADMAP.md).
The next integrated readiness exercise is defined by the
[release-candidate review plan](docs/release-candidate-review.md); that review
has not yet been performed and remains separate from any stable-release
decision.

## SQLite path compatibility

When `DB_DSN` is explicitly set, Owl Invites uses it exactly. With SQLite and
no explicit `DB_DSN`, startup selects:

| Files present | Result |
| --- | --- |
| canonical file only | use `/data/owl-invites.db` |
| inherited default only | use the inherited file and log an operator warning |
| neither | create `/data/owl-invites.db` |
| both | fail safely and require an explicit `DB_DSN` |

Startup never renames, copies, or mutates a database merely because its path is
inherited. See [backup and restore](docs/operations/backup-restore.md) for the
offline path-migration procedure. PostgreSQL DSNs are always operator-owned;
existing database and role names do not need to change.

## Production review deployments

There is no stable Owl Invites release yet. Build locally or use only an
exact-SHA review artifact whose authoritative verification, digest, SBOM,
provenance, and GitHub attestation you have checked. The production Compose
example requires the digest explicitly and cannot silently select a tag.

Read [production deployment](docs/operations/production-deployment.md) and
[backup/restore](docs/operations/backup-restore.md) before upgrading. The
application capability key is independent recovery material; changing or
losing it invalidates existing household links even when the database is
healthy.

## Engineering milestones

- [Gate 1: foundation](docs/gate-1-foundation.md)
- [Gate 2: invitation domain](docs/gate-2-invitation-domain.md)
- [Gate 3: production readiness](docs/gate-3-production-readiness.md)
- [Gate 4: controlled Owl rebrand](docs/gate-4-owl-rebrand.md)
- [Gate 5: product maturation and release candidate](docs/gate-5-product-maturation.md)

Owl Invites derives from an MIT-licensed upstream project. See
[provenance](docs/provenance.md), [NOTICE](NOTICE),
[third-party notices](THIRD_PARTY_NOTICES.md), and the
[archived upstream history](docs/upstream/openrsvp-history.md). Historical
upstream notes are preserved as history and are not Owl Invites release claims.

## License

MIT. See [LICENSE](LICENSE). The upstream copyright notice remains preserved.

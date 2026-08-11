# Gate 4: controlled Owl Invites rebrand

Gate 4 makes Owl Invites the canonical product, repository, module, binary,
container, UI, email, and documentation identity. It is deliberately
schema-neutral and does not change the Gate 2 invitation domain.

## Canonical identity

| Surface | Gate 4 identity |
| --- | --- |
| Product | Owl Invites |
| Repository/module | `xxi0xx/owl-invites`; `github.com/xxi0xx/owl-invites` |
| Binary/entrypoint | `owl-invites`; `/usr/local/bin/owl-invites` |
| Image | `ghcr.io/xxi0xx/owl-invites` |
| Compose service/volume | `owl-invites`; `owl-invites-data` |
| Container account | `owl-invites`, with preserved UID/GID 10001 |
| Fresh SQLite default | `/data/owl-invites.db` |

Bare RSVP terminology was not renamed. `RSVPResponse`, invitation responses,
and RSVP questions remain accurate domain terms.

## Source, runtime, and client surfaces

The Go module and every native import use the Owl Invites repository. A single
`cmd/owl-invites` target serves both the application and operator commands; the
old executable is no longer built or shipped. Docker, Make, Compose, CI, E2E,
PostgreSQL fixtures, temporary artifacts, OCI labels, OpenAPI metadata, npm
metadata, titles, navigation, error states, emails, and export filenames use
the canonical identity.

The numeric container account remains 10001:10001 so existing `/data`
ownership is compatible even though the textual account changed. Renaming a
Compose named volume can otherwise present an empty volume, so operators must
attach the existing volume explicitly or perform the verified offline
migration described in the operations guide.

## Database and persisted-state compatibility

An explicit `DB_DSN` always wins byte-for-byte. PostgreSQL role and database
names are operator-controlled and never rewritten. New examples use
`owl_invites`, but an existing explicit DSN needs no rename.

With SQLite and no explicit DSN, the canonical file wins when it is the only
default; the inherited file is selected with a warning when it is the only
default; neither file selects the new path; both files cause an actionable
startup failure. Selection uses metadata checks only. Startup never moves,
copies, opens, or mutates both files while deciding.

The Gate 3→4 recovery drill restores real invitation state at the inherited
path, selects it through Gate 4 discovery, and proves the existing household
capability, guests, response, answers, upload, secret fingerprint, and backup
format remain valid. Fresh and ambiguous-file cases are covered separately.
The PostgreSQL recovery drill creates a noncanonical operator role/database,
reuses its explicit DSN through migration and restore, and proves the same
capability without requiring an Owl-themed database identity.

## Deliberately preserved identifiers

Cosmetic purity cannot break existing consumers or duplicate calendar events.
The following values are isolated as legacy compatibility constants:

- webhook headers `X-OpenRSVP-Signature`, `X-OpenRSVP-Event`, and
  `X-OpenRSVP-Delivery`, plus user agent `OpenRSVP-Webhook/1.0`;
- calendar UID suffix `@openrsvp` and product ID `-//OpenRSVP//EN`;
- inherited SQLite default `/data/openrsvp.db` and the exact persisted setup
  default `OpenRSVP`; and
- tests and a negative container assertion that prove those compatibility
  contracts or the absence of the retired executable.

Invitation/open-enrollment/recovery HMAC domains and the Gate 3 secret
fingerprint domain were already Owl-specific and remain byte-for-byte
unchanged. Cookie and client storage scans found no live inherited key needing
a dual-read migration.

## Visual identity and licensing

`web/static/owl-invites-mark.svg` is the replaceable source mark: a geometric
owl whose envelope is formed by the body, with no gradient or raster source.
`web/static/favicon.svg` is its favicon-scale companion. Navigation presents
the wordmark with an accessible home label; decorative use hides duplicate
alternative text.

Satoshi binaries and declarations were removed because the repository did not
carry enough redistribution provenance. Plus Jakarta Sans now serves display
and body text, with Geist Mono for code/data. Both are locally served OFL 1.1
fonts. Exact copyrights, upstream sources, hashes, and complete per-font
licenses are in `THIRD_PARTY_NOTICES.md` and `web/static/fonts/`. No runtime
font CDN is used. The inherited Svelte starter favicon was removed.

## Provenance and residue policy

Owl Invites derives from OpenRSVP 1.8.1 at commit
`82b8b34ce42a8d0266a8aef56bd8d071bd5df542`. `LICENSE`, `NOTICE`, and
`docs/provenance.md` preserve the upstream MIT notice and authorship. The
pre-Gate-4 README and release notes remain unmodified history under
`docs/upstream/`; they are not presented as Owl Invites releases.

`scripts/lint-brand-residue.sh` scans product names, repository paths, the
former working name, and upstream image coordinates. It fails CI outside a
narrow path-and-purpose allowlist covering licenses/provenance, archived
history, database selection, frozen webhook/calendar wire values, compatibility
tests, and negative absence assertions. It prints exact final counts on every
run.

## Upgrade implications and limitations

- Stop and verify a backup before moving an inherited SQLite path; move any
  applicable sidecars only while no process has the database open.
- A renamed Compose volume is not an automatic data migration. Attach or move
  the prior volume deliberately.
- Existing operator-selected instance names, DSNs, SMTP sender identities,
  database names, and roles remain authoritative.
- Historical/protocol occurrences of the upstream name remain intentionally;
  visible active-product residue is a defect.
- Gate 4 remains pre-stable. It publishes no stable semantic version or moving
  tag; review evidence is exact-SHA only.
- Dead `/r/[token]` and `/i/[token]` routes remain small, tested compatibility
  explainers for retired Gate 1 links. Removing them would turn an actionable
  recovery message into a generic 404, so they are not dead assets.

Gate 4 adds no Gate 5 feature, schema/domain redesign, or generalized delivery
infrastructure.
